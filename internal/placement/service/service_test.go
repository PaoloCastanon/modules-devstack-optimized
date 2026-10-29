package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/PaoloCastanon/modules-devstack-optimized/internal/auth"
	"github.com/PaoloCastanon/modules-devstack-optimized/internal/placement/model"
	"github.com/PaoloCastanon/modules-devstack-optimized/internal/placement/repository"
)

type stub struct {
	repository.Repository
	calls int
	err   error
	p     model.Provider
}

func (r *stub) Create(_ context.Context, p model.Provider) error    { r.calls++; r.p = p; return r.err }
func (r *stub) Get(context.Context, string) (model.Provider, error) { r.calls++; return r.p, r.err }
func who(role string) auth.Identity {
	return auth.Identity{UserID: "nova", ProjectID: "service", Roles: []string{role}, ExpiresAt: time.Now().Add(time.Hour)}
}
func TestPolicyAndErrors(t *testing.T) {
	r := &stub{}
	s := New(r)
	ctx := context.Background()
	for _, role := range []string{"reader", "member", ""} {
		if _, err := s.Create(ctx, who(role), "host", ""); err == nil || r.calls != 0 {
			t.Fatal("unauthorized persistence", role, err)
		}
	}
	for _, role := range []string{"service", "admin"} {
		if _, err := s.Create(ctx, who(role), "host", ""); err != nil {
			t.Fatal(err)
		}
	}
	expired := who("service")
	expired.ExpiresAt = time.Now().Add(-time.Second)
	if _, err := s.Create(ctx, expired, "host", ""); !errors.Is(err, auth.ErrInvalidToken) {
		t.Fatal(err)
	}
	r.err = repository.ErrDuplicate
	if _, err := s.Create(ctx, who("service"), "host", ""); model.Status(err) != 409 {
		t.Fatal(err)
	}
	r.err = repository.ErrNotFound
	if _, err := s.Get(ctx, who("service"), "4446c55f-ee69-4f16-b6c0-b562d5f200ab"); model.Status(err) != 404 {
		t.Fatal(err)
	}
}
