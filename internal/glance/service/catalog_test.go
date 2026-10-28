package service

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/PaoloCastanon/modules-devstack-optimized/internal/glance/model"
	"github.com/PaoloCastanon/modules-devstack-optimized/internal/glance/repository"
)

func operation(op, key, value string) model.PatchOperation {
	return model.PatchOperation{Op: op, Key: key, Value: json.RawMessage(value)}
}
func domainKind(err error) model.Kind {
	var d *model.Error
	if errors.As(err, &d) {
		return d.Kind
	}
	return 0
}

func TestCatalogPoliciesAndRollback(t *testing.T) {
	ctx := context.Background()
	image, _ := model.New(model.CreateInput{Properties: map[string]string{"x": "original"}}, "p", time.Now().UTC())
	r := &repositoryStub{image: image}
	s := New(r, time.Now)
	if _, err := s.List(ctx, identity("member"), model.ListOptions{Limit: 25}); domainKind(err) != model.Forbidden {
		t.Fatal(err)
	}
	if _, err := s.List(ctx, identity("reader"), model.ListOptions{Limit: 25, ProjectID: "victim", Admin: true}); err != nil || r.options.ProjectID != "p" || r.options.Admin {
		t.Fatal("untrusted scope survived", err)
	}
	for _, action := range []string{"patch", "delete"} {
		t.Run("reader "+action, func(t *testing.T) {
			var err error
			if action == "patch" {
				_, err = s.Patch(ctx, identity("reader"), image.ID, nil)
			} else {
				err = s.Delete(ctx, identity("reader"), image.ID)
			}
			if domainKind(err) != model.Forbidden {
				t.Fatal(err)
			}
		})
	}
	for _, visibility := range []string{"private", "shared", "public", "community"} {
		t.Run("foreign "+visibility, func(t *testing.T) {
			r.image.Visibility = visibility
			who := identity("member")
			who.ProjectID = "other"
			_, err := s.Patch(ctx, who, image.ID, []model.PatchOperation{operation("add", "y", `"x"`)})
			want := model.NotFound
			if visibility == "public" || visibility == "community" {
				want = model.Forbidden
			}
			if domainKind(err) != want {
				t.Fatal(err)
			}
			if err = s.Delete(ctx, who, image.ID); domainKind(err) != want {
				t.Fatal(err)
			}
		})
	}
	r.image = image
	if _, err := s.Patch(ctx, identity("member"), image.ID, []model.PatchOperation{operation("replace", "x", `"changed"`), operation("remove", "absent", ``)}); domainKind(err) != model.Conflict || r.image.Properties["x"] != "original" {
		t.Fatal("partial patch persisted", err)
	}
	for _, value := range []string{`"public"`, `"\u0070ublic"`} {
		if _, err := s.Patch(ctx, identity("member"), image.ID, []model.PatchOperation{operation("replace", "visibility", value), operation("replace", "visibility", `"private"`)}); domainKind(err) != model.Forbidden {
			t.Fatal("publication bypass", err)
		}
	}
	if _, err := s.Patch(ctx, identity("admin"), image.ID, []model.PatchOperation{operation("replace", "visibility", `"public"`)}); err != nil {
		t.Fatal(err)
	}
	r.image = image
	r.image.Protected = true
	if err := s.Delete(ctx, identity("member"), image.ID); domainKind(err) != model.Forbidden {
		t.Fatal(err)
	}
	r.image.Protected = false
	r.image.Status = "active"
	if err := s.Delete(ctx, identity("member"), image.ID); domainKind(err) != model.Conflict {
		t.Fatal(err)
	}
	r.image = image
	if err := s.Delete(ctx, identity("member"), image.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.Delete(ctx, identity("member"), image.ID); domainKind(err) != model.NotFound {
		t.Fatal(err)
	}
	for _, upstream := range []error{repository.ErrConflict, repository.ErrNotFound, errors.New("database failure")} {
		r.err = upstream
		_, err := s.Patch(ctx, identity("member"), image.ID, nil)
		if upstream == repository.ErrConflict && domainKind(err) != model.Conflict {
			t.Fatal(err)
		}
		if upstream == repository.ErrNotFound && domainKind(err) != model.NotFound {
			t.Fatal(err)
		}
		if upstream.Error() == "database failure" && !errors.Is(err, upstream) {
			t.Fatal(err)
		}
	}
}

// This double checks the callback contract and service concurrency only. Real
// MySQL lock serialization is covered by the opt-in integration test.
type lockedRepository struct {
	repositoryStub
	mu sync.Mutex
}

func (r *lockedRepository) Update(c context.Context, id string, f func(model.Image) (model.Image, error)) (model.Image, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.repositoryStub.Update(c, id, f)
}
func TestConcurrentPatchCallbacksRetainBothChanges(t *testing.T) {
	i, _ := model.New(model.CreateInput{}, "p", time.Now())
	r := &lockedRepository{repositoryStub: repositoryStub{image: i}}
	s := New(r, time.Now)
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for _, key := range []string{"one", "two"} {
		wg.Add(1)
		go func(key string) {
			defer wg.Done()
			_, err := s.Patch(context.Background(), identity("member"), i.ID, []model.PatchOperation{operation("add", key, `"value"`)})
			errs <- err
		}(key)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if len(r.image.Properties) != 2 {
		t.Fatal("lost an update")
	}
}
