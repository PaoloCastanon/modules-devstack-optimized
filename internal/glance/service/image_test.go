package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/PaoloCastanon/modules-devstack-optimized/internal/auth"
	"github.com/PaoloCastanon/modules-devstack-optimized/internal/glance/model"
	"github.com/PaoloCastanon/modules-devstack-optimized/internal/glance/repository"
)

type repositoryStub struct {
	image   model.Image
	err     error
	creates int
	deleted bool
	options model.ListOptions
}

func (r *repositoryStub) Create(ctx context.Context, i model.Image) error {
	r.creates++
	r.image = i
	return r.err
}
func (r *repositoryStub) Get(context.Context, string) (model.Image, error) { return r.image, r.err }
func (r *repositoryStub) List(_ context.Context, o model.ListOptions) ([]model.Image, error) {
	r.options = o
	return []model.Image{}, r.err
}
func (r *repositoryStub) Update(_ context.Context, _ string, f func(model.Image) (model.Image, error)) (model.Image, error) {
	if r.err != nil {
		return model.Image{}, r.err
	}
	if r.deleted {
		return model.Image{}, repository.ErrNotFound
	}
	i, err := f(r.image)
	if err == nil {
		r.image = i
	}
	return i, err
}
func (r *repositoryStub) Delete(_ context.Context, _ string, _ time.Time, f func(model.Image) error) error {
	if r.err != nil {
		return r.err
	}
	if r.deleted {
		return repository.ErrNotFound
	}
	if err := f(r.image); err != nil {
		return err
	}
	r.deleted = true
	return nil
}

func identity(roles ...string) auth.Identity {
	return auth.Identity{UserID: "u", ProjectID: "p", Roles: roles, ExpiresAt: time.Now().Add(time.Hour)}
}

func TestCreatePolicyAndPropagation(t *testing.T) {
	ctx := context.Background()
	repo := &repositoryStub{}
	s := New(repo, time.Now)
	if _, err := s.Create(ctx, identity("reader"), model.CreateInput{}); err == nil || repo.creates != 0 {
		t.Fatal("reader wrote data")
	}
	public := "public"
	if _, err := s.Create(ctx, identity("member"), model.CreateInput{Visibility: &public}); err == nil || repo.creates != 0 {
		t.Fatal("member published data")
	}
	if _, err := s.Create(ctx, identity("admin"), model.CreateInput{Visibility: &public}); err != nil {
		t.Fatal(err)
	}
	if repo.image.Owner != "p" || repo.image.Status != "queued" {
		t.Fatal("wrong identity or state")
	}
	repo.err = repository.ErrDuplicate
	_, err := s.Create(ctx, identity("member"), model.CreateInput{})
	var domain *model.Error
	if !errors.As(err, &domain) || domain.Kind != model.Conflict {
		t.Fatalf("duplicate: %v", err)
	}
	upstream := errors.New("database failure")
	repo.err = upstream
	if _, err = s.Create(ctx, identity("member"), model.CreateInput{}); !errors.Is(err, upstream) {
		t.Fatal("creation error swallowed")
	}
	if _, err = s.Get(ctx, identity("reader"), "00000000-0000-4000-8000-000000000000"); !errors.Is(err, upstream) {
		t.Fatal("read error swallowed")
	}
}

func TestReadVisibility(t *testing.T) {
	id := "00000000-0000-4000-8000-000000000000"
	for _, visibility := range []string{"private", "shared", "public", "community"} {
		for _, persona := range []string{"owner", "other", "admin", "member-only"} {
			t.Run(visibility+persona, func(t *testing.T) {
				repo := &repositoryStub{image: model.Image{ID: id, Owner: "p", Visibility: visibility}}
				s := New(repo, time.Now)
				who := identity("reader")
				if persona == "other" {
					who.ProjectID = "other"
				}
				if persona == "admin" {
					who = identity("admin")
					who.ProjectID = "other"
				}
				if persona == "member-only" {
					who = identity("member")
				}
				_, err := s.Get(context.Background(), who, id)
				allowed := persona != "member-only" && (persona != "other" || visibility == "public" || visibility == "community")
				if (err == nil) != allowed {
					t.Fatalf("allowed=%v error=%v", allowed, err)
				}
				if persona == "other" && !allowed {
					var d *model.Error
					if !errors.As(err, &d) || d.Kind != model.NotFound {
						t.Fatal("private image existence disclosed")
					}
				}
			})
		}
	}
	repo := &repositoryStub{err: repository.ErrNotFound}
	s := New(repo, time.Now)
	_, err := s.Get(context.Background(), identity("reader"), id)
	var d *model.Error
	if !errors.As(err, &d) || d.Kind != model.NotFound {
		t.Fatal(err)
	}
	if _, err = s.Get(context.Background(), identity("admin"), "invalid"); err == nil {
		t.Fatal("invalid id accepted")
	}
	expired := identity("admin")
	expired.ExpiresAt = time.Now().Add(-time.Second)
	if _, err = s.Create(context.Background(), expired, model.CreateInput{}); !errors.Is(err, auth.ErrInvalidToken) {
		t.Fatal("expired identity used")
	}
}
