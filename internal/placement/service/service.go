package service

import (
	"context"
	"errors"
	"fmt"
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
		return model.Fail(409, "resource provider generation conflict")
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
	return p, mapped(err, p.UUID)
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
func (s *Service) ReplaceInventories(ctx context.Context, who auth.Identity, id string, generation int64, input map[string]model.InventoryInput) (model.Inventories, error) {
	if err := authorize(who); err != nil {
		return model.Inventories{}, err
	}
	id, err := pathID(id)
	if err != nil {
		return model.Inventories{}, err
	}
	invs := make(map[string]model.Inventory, len(input))
	for class, raw := range input {
		if !model.KnownClass(class) {
			return model.Inventories{}, model.Fail(400, fmt.Sprintf("Unknown resource class in inventory for resource provider %s: No such resource class %s.", id, class))
		}
		inv, err := raw.Resolve()
		if errors.Is(err, model.ErrInventoryCapacity) {
			return model.Inventories{}, model.Fail(400, fmt.Sprintf("Unable to update inventory for resource provider %s: Invalid inventory for '%s' on resource provider '%s'. The reserved value is greater than or equal to total.", id, class, id))
		}
		if err != nil {
			return model.Inventories{}, err
		}
		invs[class] = inv
	}
	result, err := s.repo.ReplaceInventories(ctx, id, generation, invs)
	return result, mapped(err, id)
}
