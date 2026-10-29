package service

import (
	"context"
	"errors"
	"github.com/PaoloCastanon/modules-devstack-optimized/internal/auth"
	"github.com/PaoloCastanon/modules-devstack-optimized/internal/placement/model"
	"github.com/PaoloCastanon/modules-devstack-optimized/internal/placement/repository"
	"strings"
)

func (s *Service) allocations(who auth.Identity) (repository.Allocations, error) {
	if err := authorize(who); err != nil {
		return nil, err
	}
	r, ok := s.repo.(repository.Allocations)
	if !ok {
		return nil, model.Fail(503, "Allocation repository unavailable.")
	}
	return r, nil
}
func (s *Service) ConsumerAllocations(ctx context.Context, who auth.Identity, id string) (model.ConsumerAllocations, error) {
	r, err := s.allocations(who)
	if err != nil {
		return model.ConsumerAllocations{}, err
	}
	return r.ConsumerAllocations(ctx, id)
}
func (s *Service) ReplaceAllocations(ctx context.Context, who auth.Identity, id string, request model.AllocationRequest) error {
	r, err := s.allocations(who)
	if err != nil {
		return err
	}
	originalID := id
	id, err = model.NormalizeUUID(id)
	if err != nil {
		return model.Fail(400, "Malformed consumer_uuid: "+originalID)
	}
	values := map[string]map[string]int64{}
	for _, item := range request.Allocations {
		values[strings.ToLower(item.Provider.UUID)] = item.Resources
	}
	update := model.AllocationUpdate{Allocations: values, ProjectID: "00000000-0000-0000-0000-000000000000", UserID: "00000000-0000-0000-0000-000000000000"}
	if request.ProjectID != nil {
		update.ProjectID = *request.ProjectID
	}
	if request.UserID != nil {
		update.UserID = *request.UserID
	}
	err = r.ReplaceAllocations(ctx, id, update)
	if errors.Is(err, repository.ErrConcurrent) {
		return model.Fail(409, "Inventory and/or allocations changed while attempting to allocate: Another thread concurrently updated the resource provider data. Please retry your update")
	}
	return err
}
func (s *Service) DeleteAllocations(ctx context.Context, who auth.Identity, id string) error {
	r, err := s.allocations(who)
	if err != nil {
		return err
	}
	err = r.DeleteAllocations(ctx, id)
	if errors.Is(err, repository.ErrNotFound) {
		return model.Fail(404, "No allocations for consumer '"+id+"'")
	}
	return err
}
func (s *Service) ProviderAllocations(ctx context.Context, who auth.Identity, id string) (model.ProviderAllocations, error) {
	r, err := s.allocations(who)
	if err != nil {
		return model.ProviderAllocations{}, err
	}
	out, err := r.ProviderAllocations(ctx, strings.ToLower(id))
	if errors.Is(err, repository.ErrNotFound) {
		err = model.Fail(404, "Resource provider '"+id+"' not found: No resource provider with uuid "+id+" found")
	}
	return out, err
}
func (s *Service) Usages(ctx context.Context, who auth.Identity, id string) (model.Usages, error) {
	r, err := s.allocations(who)
	if err != nil {
		return model.Usages{}, err
	}
	out, err := r.Usages(ctx, strings.ToLower(id))
	if errors.Is(err, repository.ErrNotFound) {
		err = model.Fail(404, "No resource provider with uuid "+id+" found: No resource provider with uuid "+id+" found")
	}
	return out, err
}

func (s *Service) ReplaceModernAllocations(ctx context.Context, who auth.Identity, id string, request model.ModernAllocationRequest, check bool) error {
	r, err := s.allocations(who)
	if err != nil {
		return err
	}
	values := map[string]map[string]int64{}
	for provider, item := range request.Allocations {
		values[strings.ToLower(provider)] = item.Resources
	}
	id, err = model.NormalizeUUID(id)
	if err != nil {
		return err
	}
	update := model.AllocationUpdate{Allocations: values, ProjectID: request.ProjectID, UserID: request.UserID, Generation: request.ConsumerGeneration, CheckGeneration: check}
	err = r.ReplaceAllocations(ctx, id, update)
	if errors.Is(err, repository.ErrConcurrent) {
		return model.FailCode(409, "Inventory and/or allocations changed while attempting to allocate: Another thread concurrently updated the resource provider data. Please retry your update", "placement.concurrent_update")
	}
	return err
}
