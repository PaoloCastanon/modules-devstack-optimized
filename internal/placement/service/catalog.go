package service

import (
	"context"
	"errors"
	"strings"

	"github.com/PaoloCastanon/modules-devstack-optimized/internal/auth"
	"github.com/PaoloCastanon/modules-devstack-optimized/internal/placement/model"
	"github.com/PaoloCastanon/modules-devstack-optimized/internal/placement/repository"
)

func (s *Service) catalog(who auth.Identity) (repository.Catalog, error) {
	if err := authorize(who); err != nil {
		return nil, err
	}
	c, ok := s.repo.(repository.Catalog)
	if !ok {
		return nil, model.Fail(503, "Catalogue repository unavailable.")
	}
	return c, nil
}
func classError(err error, name string) error {
	if errors.Is(err, repository.ErrNotFound) {
		return model.Fail(404, "No such resource class "+name+".")
	}
	return mapped(err, "")
}
func (s *Service) Classes(ctx context.Context, who auth.Identity) ([]model.CatalogEntry, error) {
	c, err := s.catalog(who)
	if err != nil {
		return nil, err
	}
	return c.Classes(ctx)
}
func (s *Service) Class(ctx context.Context, who auth.Identity, name string) (model.CatalogEntry, error) {
	c, err := s.catalog(who)
	if err != nil {
		return model.CatalogEntry{}, err
	}
	item, err := c.Class(ctx, name)
	return item, classError(err, name)
}
func (s *Service) CreateClass(ctx context.Context, who auth.Identity, name string, idempotent bool) (bool, error) {
	c, err := s.catalog(who)
	if err != nil {
		return false, err
	}
	if err = model.ValidateCustomName(name); err != nil {
		return false, err
	}
	created, err := c.CreateClass(ctx, name, idempotent)
	if errors.Is(err, repository.ErrDuplicate) {
		err = model.Fail(409, "Conflicting resource class already exists: "+name)
	}
	return created, err
}
func (s *Service) RenameClass(ctx context.Context, who auth.Identity, name, to string) error {
	c, err := s.catalog(who)
	if err != nil {
		return err
	}
	if err = model.ValidateCustomName(to); err != nil {
		return err
	}
	err = c.RenameClass(ctx, name, to)
	if errors.Is(err, repository.ErrStandard) {
		return model.Fail(400, "Cannot update standard resource class "+name)
	}
	if errors.Is(err, repository.ErrDuplicate) {
		return model.Fail(409, "Resource class already exists: "+to)
	}
	return classError(err, name)
}
func (s *Service) DeleteClass(ctx context.Context, who auth.Identity, name string) error {
	c, err := s.catalog(who)
	if err != nil {
		return err
	}
	err = c.DeleteClass(ctx, name)
	if errors.Is(err, repository.ErrStandard) {
		return model.Fail(400, "Error in delete resource class: Cannot delete standard resource class "+name+".")
	}
	if errors.Is(err, repository.ErrInUse) {
		return model.Fail(409, "Error in delete resource class: Cannot delete resource class "+name+". Class is in use in inventory.")
	}
	return classError(err, name)
}
func (s *Service) Traits(ctx context.Context, who auth.Identity, f model.TraitFilter) ([]string, error) {
	c, err := s.catalog(who)
	if err != nil {
		return nil, err
	}
	return c.Traits(ctx, f)
}
func (s *Service) Trait(ctx context.Context, who auth.Identity, name string) error {
	c, err := s.catalog(who)
	if err != nil {
		return err
	}
	_, err = c.Trait(ctx, name)
	if errors.Is(err, repository.ErrNotFound) {
		return model.Fail(404, "No such trait(s): "+name+".")
	}
	return err
}
func (s *Service) EnsureTrait(ctx context.Context, who auth.Identity, name string) (bool, error) {
	c, err := s.catalog(who)
	if err != nil {
		return false, err
	}
	if model.ValidateCustomName(name) != nil {
		return false, model.Fail(400, `The trait is invalid. A valid trait must be no longer than 255 characters, start with the prefix "CUSTOM_" and use following characters: "A"-"Z", "0"-"9" and "_"`)
	}
	return c.EnsureTrait(ctx, name)
}
func (s *Service) DeleteTrait(ctx context.Context, who auth.Identity, name string) error {
	c, err := s.catalog(who)
	if err != nil {
		return err
	}
	err = c.DeleteTrait(ctx, name)
	if errors.Is(err, repository.ErrNotFound) {
		return model.Fail(404, "No such trait(s): "+name+".")
	}
	if errors.Is(err, repository.ErrStandard) {
		return model.Fail(400, "Cannot delete standard trait "+name+".")
	}
	if errors.Is(err, repository.ErrInUse) {
		return model.Fail(409, "The trait "+name+" is in use by a resource provider.")
	}
	return err
}
func (s *Service) ProviderTraits(ctx context.Context, who auth.Identity, id string) (model.TraitSet, error) {
	c, err := s.catalog(who)
	if err != nil {
		return model.TraitSet{}, err
	}
	id, err = pathID(id)
	if err != nil {
		return model.TraitSet{}, err
	}
	out, err := c.ProviderTraits(ctx, id)
	if errors.Is(err, repository.ErrNotFound) {
		detail := "No resource provider with uuid " + id + " found"
		err = model.Fail(404, detail+": "+detail)
	}
	return out, err
}
func (s *Service) ReplaceTraits(ctx context.Context, who auth.Identity, id string, generation int64, names []string) (model.TraitSet, error) {
	c, err := s.catalog(who)
	if err != nil {
		return model.TraitSet{}, err
	}
	id, err = pathID(id)
	if err != nil {
		return model.TraitSet{}, err
	}
	out, err := c.ReplaceTraits(ctx, id, generation, names)
	if errors.Is(err, repository.ErrConcurrent) {
		return out, model.Fail(409, "Resource provider's generation already changed. Please update the generation and try again.")
	}
	var missing *repository.MissingTraits
	if errors.As(err, &missing) {
		return out, model.Fail(400, "No such trait "+strings.Join(missing.Names, ", "))
	}
	return out, mapped(err, id)
}
func (s *Service) DeleteTraits(ctx context.Context, who auth.Identity, id string) error {
	c, err := s.catalog(who)
	if err != nil {
		return err
	}
	p, err := s.Get(ctx, who, id)
	if err != nil {
		return err
	}
	_, err = c.ReplaceTraits(ctx, p.UUID, p.Generation, []string{})
	if errors.Is(err, repository.ErrConcurrent) {
		return model.Fail(409, "Another thread concurrently updated the resource provider data. Please retry your update")
	}
	return mapped(err, p.UUID)
}
func (s *Service) Aggregates(ctx context.Context, who auth.Identity, id string) ([]string, error) {
	c, err := s.catalog(who)
	if err != nil {
		return nil, err
	}
	id, err = pathID(id)
	if err != nil {
		return nil, err
	}
	out, err := c.Aggregates(ctx, id)
	return out, mapped(err, id)
}
func (s *Service) ReplaceAggregates(ctx context.Context, who auth.Identity, id string, names []string) ([]string, error) {
	c, err := s.catalog(who)
	if err != nil {
		return nil, err
	}
	id, err = pathID(id)
	if err != nil {
		return nil, err
	}
	for _, name := range names {
		if _, err = model.NormalizeUUID(name); err != nil {
			return nil, err
		}
	}
	out, err := c.ReplaceAggregates(ctx, id, names)
	return out, mapped(err, id)
}
func (s *Service) DeleteInventories(ctx context.Context, who auth.Identity, id string) error {
	c, err := s.catalog(who)
	if err != nil {
		return err
	}
	id, err = pathID(id)
	if err != nil {
		return err
	}
	err = c.DeleteInventories(ctx, id)
	var inUse *repository.InventoryInUse
	if errors.As(err, &inUse) {
		return model.FailCode(409, inUse.Error(), "placement.inventory.inuse")
	}
	if errors.Is(err, repository.ErrConcurrent) {
		return model.Fail(409, "Unable to delete inventory for resource provider "+id+" because the inventory was updated by another process. Please retry your request.")
	}
	return mapped(err, id)
}

func (s *Service) AggregateState(ctx context.Context, who auth.Identity, id string) (model.AggregateSet, error) {
	c, err := s.catalog(who)
	if err != nil {
		return model.AggregateSet{}, err
	}
	id, err = pathID(id)
	if err != nil {
		return model.AggregateSet{}, err
	}
	out, err := c.AggregateState(ctx, id)
	return out, mapped(err, id)
}
func (s *Service) ReplaceAggregatesGeneration(ctx context.Context, who auth.Identity, id string, generation int64, names []string) (model.AggregateSet, error) {
	c, err := s.catalog(who)
	if err != nil {
		return model.AggregateSet{}, err
	}
	id, err = pathID(id)
	if err != nil {
		return model.AggregateSet{}, err
	}
	out, err := c.ReplaceAggregatesGeneration(ctx, id, generation, names)
	if errors.Is(err, repository.ErrConcurrent) {
		return out, model.FailCode(409, "Resource provider's generation already changed. Please update the generation and try again.", "placement.concurrent_update")
	}
	return out, mapped(err, id)
}
