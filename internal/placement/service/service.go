package service

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/PaoloCastanon/modules-devstack-optimized/internal/auth"
	"github.com/PaoloCastanon/modules-devstack-optimized/internal/placement/model"
	"github.com/PaoloCastanon/modules-devstack-optimized/internal/placement/repository"
)

type Service struct{ repo repository.Repository }

func New(repo repository.Repository) *Service { return &Service{repo: repo} }

// Placement resources are global infrastructure, not owned by the caller's project.
func authorize(who auth.Identity) error {
	if err := who.Check(time.Now()); err != nil {
		return err
	}
	if !who.HasRole("admin") && !who.HasRole("service") {
		return model.Fail(403, "Placement requires the admin or service role.")
	}
	return nil
}
func mapped(err error, id string) error {
	switch {
	case errors.Is(err, repository.ErrNotFound):
		return model.Fail(404, "No resource provider with uuid "+id+" found")
	case errors.Is(err, repository.ErrDuplicate):
		return model.Fail(409, "Conflicting resource provider name or UUID already exists.")
	case errors.Is(err, repository.ErrConcurrent):
		return model.FailCode(409, "resource provider generation conflict", "placement.concurrent_update")
	case errors.Is(err, repository.ErrProviderInUse):
		return model.FailCode(409, "Unable to delete resource provider "+id+": Resource provider has allocations.", "placement.resource_provider.inuse")
	case errors.Is(err, repository.ErrUnknownClass):
		return model.Fail(400, "Unknown resource class in inventory.")
	}
	return err
}
func pathID(id string) (string, error) {
	_, err := model.NormalizeUUID(id)
	if err != nil {
		return "", model.Fail(404, "No resource provider with uuid "+id+" found")
	}
	return strings.ToLower(id), nil
}
func (s *Service) Create(ctx context.Context, who auth.Identity, name, id string) (model.Provider, error) {
	if err := authorize(who); err != nil {
		return model.Provider{}, err
	}
	p, err := model.NewProvider(name, id)
	if err != nil {
		return p, err
	}
	err = s.repo.Create(ctx, p)
	if errors.Is(err, repository.ErrDuplicateUUID) {
		return p, model.Fail(409, "Conflicting resource provider uuid: "+p.UUID+" already exists.")
	}
	if errors.Is(err, repository.ErrDuplicateName) {
		return p, model.Fail(409, "Conflicting resource provider name: "+p.Name+" already exists.")
	}
	if err != nil {
		return p, mapped(err, p.UUID)
	}
	return s.repo.Get(ctx, p.UUID)
}
func (s *Service) Get(ctx context.Context, who auth.Identity, id string) (model.Provider, error) {
	if err := authorize(who); err != nil {
		return model.Provider{}, err
	}
	id, err := pathID(id)
	if err != nil {
		return model.Provider{}, err
	}
	p, err := s.repo.Get(ctx, id)
	return p, mapped(err, id)
}
func (s *Service) List(ctx context.Context, who auth.Identity, f model.Filter) ([]model.Provider, error) {
	if err := authorize(who); err != nil {
		return nil, err
	}
	if f.UUID != nil {
		_, err := model.NormalizeUUID(*f.UUID)
		if err != nil || len(*f.UUID) != 36 {
			return nil, model.Fail(400, "Invalid UUID query value.")
		}
		value := strings.ToLower(*f.UUID)
		f.UUID = &value
	}
	if f.Name != nil && *f.Name == "" {
		f.Name = nil
	}
	if f.InTree != nil {
		v := strings.ToLower(*f.InTree)
		f.InTree = &v
	}
	if len(f.Required)+len(f.Forbidden) > 0 {
		catalog, err := s.catalog(who)
		if err != nil {
			return nil, err
		}
		for _, name := range append(append([]string{}, f.Required...), f.Forbidden...) {
			if _, err := catalog.Trait(ctx, name); errors.Is(err, repository.ErrNotFound) {
				return nil, model.Fail(400, "No such trait(s): "+name+".")
			} else if err != nil {
				return nil, err
			}
		}
	}

	for name := range f.Resources {
		known, err := s.repo.ResourceClassExists(ctx, name)
		if err != nil {
			return nil, err
		}
		if !known {
			return nil, model.Fail(400, "Invalid resource class in resources parameter: No such resource class "+name+".")
		}
	}
	return s.repo.List(ctx, f)
}
func (s *Service) Rename(ctx context.Context, who auth.Identity, id, name string) (model.Provider, error) {
	if err := authorize(who); err != nil {
		return model.Provider{}, err
	}
	id, err := pathID(id)
	if err != nil {
		return model.Provider{}, err
	}
	if err = model.ValidateName(name); err != nil {
		return model.Provider{}, err
	}
	p, err := s.repo.Rename(ctx, id, name)
	if errors.Is(err, repository.ErrDuplicate) {
		return p, model.Fail(409, "Conflicting resource provider "+name+" already exists.")
	}
	return p, mapped(err, id)
}
func (s *Service) Delete(ctx context.Context, who auth.Identity, id string) error {
	if err := authorize(who); err != nil {
		return err
	}
	id, err := pathID(id)
	if err != nil {
		return err
	}
	return mapped(s.repo.Delete(ctx, id), id)
}
func (s *Service) Inventories(ctx context.Context, who auth.Identity, id string) (model.Inventories, error) {
	if err := authorize(who); err != nil {
		return model.Inventories{}, err
	}
	id, err := pathID(id)
	if err != nil {
		return model.Inventories{}, err
	}
	invs, err := s.repo.Inventories(ctx, id)
	if errors.Is(err, repository.ErrNotFound) {
		detail := "No resource provider with uuid " + id + " found"
		return invs, model.Fail(404, detail+" : "+detail)
	}
	return invs, mapped(err, id)
}
func (s *Service) ReplaceInventories(ctx context.Context, who auth.Identity, id string, generation int64, input map[string]model.InventoryInput, classOrder ...string) (model.Inventories, error) {
	return s.ReplaceInventoriesVersion(ctx, who, id, generation, input, false, classOrder...)
}
func (s *Service) ReplaceInventoriesVersion(ctx context.Context, who auth.Identity, id string, generation int64, input map[string]model.InventoryInput, allowZero bool, classOrder ...string) (model.Inventories, error) {
	if err := authorize(who); err != nil {
		return model.Inventories{}, err
	}
	id, err := pathID(id)
	if err != nil {
		return model.Inventories{}, err
	}
	provider, err := s.repo.Get(ctx, id)
	if err != nil {
		return model.Inventories{}, mapped(err, id)
	}
	if provider.Generation != generation {
		return model.Inventories{}, mapped(repository.ErrConcurrent, id)
	}
	classes := make([]string, 0, len(input))
	positions := make(map[string]int, len(classOrder))
	for index, class := range classOrder {
		positions[class] = index
	}
	for class := range input {
		classes = append(classes, class)
	}
	sort.Slice(classes, func(i, j int) bool {
		left, ok := positions[classes[i]]
		if !ok {
			left = len(classOrder)
		}
		right, ok := positions[classes[j]]
		if !ok {
			right = len(classOrder)
		}
		if left != right {
			return left < right
		}
		return classes[i] < classes[j]
	})
	invs := make(map[string]model.Inventory, len(input))
	for _, class := range classes {
		raw := input[class]
		inv, err := raw.ResolveFor(allowZero)
		if errors.Is(err, model.ErrInventoryCapacity) {
			comparison := "greater than or equal to"
			if allowZero {
				comparison = "greater than"
			}
			return model.Inventories{}, model.Fail(400, fmt.Sprintf("Unable to update inventory for resource provider %s: Invalid inventory for '%s' on resource provider '%s'. The reserved value is %s total.", id, class, id, comparison))
		}
		if err != nil {
			return model.Inventories{}, err
		}
		invs[class] = inv
	}
	// The original validates capacity for every proposed inventory before its
	// repository resolves class names. Keep the final CAS inside that repository.
	for _, class := range classes {
		known, err := s.repo.ResourceClassExists(ctx, class)
		if err != nil {
			return model.Inventories{}, err
		}
		if !known {
			return model.Inventories{}, model.Fail(400, fmt.Sprintf("Unknown resource class in inventory for resource provider %s: No such resource class %s.", id, class))
		}
	}
	result, err := s.repo.ReplaceInventories(ctx, id, generation, invs)
	var inUse *repository.InventoryInUse
	if errors.As(err, &inUse) {
		err = model.FailCode(409, "update conflict: "+inUse.Error(), "placement.inventory.inuse")
	}
	return result, mapped(err, id)
}
