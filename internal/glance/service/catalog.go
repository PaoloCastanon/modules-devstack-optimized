package service

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/PaoloCastanon/modules-devstack-optimized/internal/auth"
	"github.com/PaoloCastanon/modules-devstack-optimized/internal/glance/model"
	"github.com/PaoloCastanon/modules-devstack-optimized/internal/glance/repository"
)

func (s *Service) List(ctx context.Context, who auth.Identity, options model.ListOptions) ([]model.Image, error) {
	if err := who.Check(s.now()); err != nil {
		return nil, err
	}
	if !who.HasRole("reader") && !who.HasRole("admin") {
		return nil, model.Fail(model.Forbidden, "Listing images requires reader or admin role.")
	}
	options.ProjectID, options.Admin = who.ProjectID, who.HasRole("admin")
	options, err := options.Validate()
	if err != nil {
		return nil, err
	}
	return s.repo.List(ctx, options)
}

func writePolicy(who auth.Identity, image model.Image) error {
	if !who.HasRole("member") && !who.HasRole("admin") {
		return model.Fail(model.Forbidden, "Modifying images requires member or admin role.")
	}
	if !who.HasRole("admin") && image.Owner != who.ProjectID {
		if image.Visibility != "public" && image.Visibility != "community" {
			return notFound(image.ID)
		}
		return model.Fail(model.Forbidden, "Only the owner or an administrator may modify this image.")
	}
	return nil
}

func mutationError(err error, id string) error {
	if errors.Is(err, repository.ErrNotFound) {
		return notFound(id)
	}
	if errors.Is(err, repository.ErrConflict) {
		return model.Fail(model.Conflict, "Concurrent metadata update; retry the request.")
	}
	return err
}

func (s *Service) Patch(ctx context.Context, who auth.Identity, id string, ops []model.PatchOperation) (model.Image, error) {
	if err := who.Check(s.now()); err != nil {
		return model.Image{}, err
	}
	if !who.HasRole("member") && !who.HasRole("admin") {
		return model.Image{}, model.Fail(model.Forbidden, "Modifying images requires member or admin role.")
	}
	id, err := model.CanonicalID(id)
	if err != nil {
		return model.Image{}, err
	}
	image, err := s.repo.Update(ctx, id, func(current model.Image) (model.Image, error) {
		if err := writePolicy(who, current); err != nil {
			return model.Image{}, err
		}
		for _, op := range ops {
			var visibility string
			if op.Key == "visibility" && op.Op != "remove" && !who.HasRole("admin") && json.Unmarshal(op.Value, &visibility) == nil && visibility == "public" {
				return model.Image{}, model.Fail(model.Forbidden, "Publishing images requires admin role.")
			}
		}
		changed, err := model.ApplyPatch(current, ops, s.now())
		if err != nil {
			return model.Image{}, err
		}
		// Check the decoded value, not just its possible JSON spellings/escapes.
		if changed.Visibility == "public" && current.Visibility != "public" && !who.HasRole("admin") {
			return model.Image{}, model.Fail(model.Forbidden, "Publishing images requires admin role.")
		}
		return changed, nil
	})
	return image, mutationError(err, id)
}

func (s *Service) Delete(ctx context.Context, who auth.Identity, id string) error {
	if err := who.Check(s.now()); err != nil {
		return err
	}
	if !who.HasRole("member") && !who.HasRole("admin") {
		return model.Fail(model.Forbidden, "Deleting images requires member or admin role.")
	}
	id, err := model.CanonicalID(id)
	if err != nil {
		return err
	}
	err = s.repo.Delete(ctx, id, s.now(), func(current model.Image) error {
		if err := writePolicy(who, current); err != nil {
			return err
		}
		if current.Protected {
			return model.Fail(model.Forbidden, "Protected images cannot be deleted.")
		}
		if current.Status != "queued" || current.Size != nil || current.VirtualSize != nil || current.Checksum != nil || current.HashAlgorithm != nil || current.HashValue != nil {
			return model.Fail(model.Conflict, "Only metadata without binary content can be deleted in B20.")
		}
		return nil
	})
	return mutationError(err, id)
}
