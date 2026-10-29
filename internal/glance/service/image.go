package service

import (
	"context"
	"errors"
	"time"

	"github.com/PaoloCastanon/modules-devstack-optimized/internal/auth"
	"github.com/PaoloCastanon/modules-devstack-optimized/internal/glance/model"
	"github.com/PaoloCastanon/modules-devstack-optimized/internal/glance/repository"
)

type Service struct {
	repo repository.Repository
	now  func() time.Time
}

func New(repo repository.Repository, now func() time.Time) *Service {
	return &Service{repo: repo, now: now}
}

func (s *Service) Create(ctx context.Context, identity auth.Identity, input model.CreateInput) (model.Image, error) {
	if err := identity.Check(s.now()); err != nil {
		return model.Image{}, err
	}
	if !identity.HasRole("admin") && !identity.HasRole("member") {
		return model.Image{}, model.Fail(model.Forbidden, "Creating images requires member or admin role.")
	}
	image, err := model.New(input, identity.ProjectID, s.now())
	if err != nil {
		return model.Image{}, err
	}
	if image.Visibility == "public" && !identity.HasRole("admin") {
		return model.Image{}, model.Fail(model.Forbidden, "Publishing images requires admin role.")
	}
	if err = s.repo.Create(ctx, image); errors.Is(err, repository.ErrDuplicate) {
		return model.Image{}, model.Fail(model.Conflict, "An image with this ID already exists.")
	}
	if err != nil {
		return model.Image{}, err
	}
	return image, nil
}

func (s *Service) Get(ctx context.Context, identity auth.Identity, value string) (model.Image, error) {
	if err := identity.Check(s.now()); err != nil {
		return model.Image{}, err
	}
	id, err := model.CanonicalID(value)
	if err != nil {
		return model.Image{}, err
	}
	if !identity.HasRole("admin") && !identity.HasRole("reader") {
		return model.Image{}, model.Fail(model.Forbidden, "Reading images requires reader or admin role.")
	}
	image, err := s.repo.Get(ctx, id)
	if errors.Is(err, repository.ErrNotFound) {
		return model.Image{}, notFound(id)
	}
	if err != nil {
		return model.Image{}, err
	}
	if !identity.HasRole("admin") && image.Owner != identity.ProjectID && image.Visibility != "public" && image.Visibility != "community" {
		return model.Image{}, notFound(id)
	}
	return image, nil
}

func notFound(id string) error { return model.Fail(model.NotFound, "No image found with ID "+id) }
