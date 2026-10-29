package api

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/PaoloCastanon/modules-devstack-optimized/internal/auth"
	"github.com/PaoloCastanon/modules-devstack-optimized/internal/placement/model"
	"github.com/PaoloCastanon/modules-devstack-optimized/internal/placement/service"
)

func TestUnscopedPolicyErrorPreservesVersion(t *testing.T) {
	h, a, _ := harness(t)
	a.err = auth.ErrForbidden
	for _, tc := range []struct{ method, path, version, policy string }{
		{"GET", "/resource_providers", "1.14", "placement:resource_providers:list"},
		{"PUT", "/allocations/00000000-0000-4000-8000-000000000001", "1.36", "placement:allocations:update"},
		{"DELETE", "/resource_providers/00000000-0000-4000-8000-000000000001/inventories", "1.26", "placement:resource_providers:inventories:delete"},
	} {
		t.Run(tc.method+tc.version, func(t *testing.T) {
			r := httptest.NewRequest(tc.method, tc.path, nil)
			r.Header.Set("X-Auth-Token", "test-token")
			r.Header.Set("OpenStack-API-Version", "placement "+tc.version)
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if w.Code != 403 || w.Header().Get("OpenStack-API-Version") != "placement "+tc.version || !strings.Contains(w.Body.String(), "Policy does not allow "+tc.policy+" to be performed.") {
				t.Fatalf("%d %v %s", w.Code, w.Header(), w.Body.String())
			}
			if tc.version == "1.36" && !strings.Contains(w.Body.String(), `"code":"placement.undefined_code"`) {
				t.Fatal(w.Body.String())
			}
		})
	}
}

type inventorySnapshotStub struct {
	*memory
	modified time.Time
}

func (s *inventorySnapshotStub) InventorySnapshot(context.Context, string, string) (model.Inventories, error) {
	return model.Inventories{Generation: 2, LastModified: s.modified, Inventories: map[string]model.Inventory{"VCPU": {Total: 8, MaxUnit: 8, MinUnit: 1, StepSize: 1, AllocationRatio: 1}}}, nil
}
func TestIndividualInventoryUsesItsPersistedTimestamp(t *testing.T) {
	stamp := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	repo := &inventorySnapshotStub{memory: &memory{inventories: map[string]model.Inventories{
		"00000000-0000-4000-8000-000000000001": {LastModified: stamp.Add(time.Hour), Inventories: map[string]model.Inventory{"VCPU": {Total: 8}}},
	}}, modified: stamp}
	a := &identityStub{who: auth.Identity{UserID: "nova", ProjectID: "service", Roles: []string{"service"}, ExpiresAt: time.Now().Add(time.Hour)}}
	h, err := New(service.New(repo), a, "http://localhost/placement", "http://localhost/identity/v3", 1024, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest("GET", "/resource_providers/00000000-0000-4000-8000-000000000001/inventories/VCPU", nil)
	r.Header.Set("X-Auth-Token", "test-token")
	r.Header.Set("OpenStack-API-Version", "placement 1.26")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 200 || w.Header().Get("Cache-Control") != "no-cache" || w.Header().Get("Last-Modified") != stamp.Format(http.TimeFormat) {
		t.Fatalf("%d %v %s", w.Code, w.Header(), w.Body.String())
	}
}

// These requests cannot gain an identity through caller-supplied headers.
func TestExpiredAndDuplicateCredentialsFailClosed(t *testing.T) {
	for _, duplicate := range []bool{false, true} {
		h, a, _ := harness(t)
		a.who.ExpiresAt = time.Now().Add(-time.Minute)
		r := httptest.NewRequest("PUT", "/allocations/00000000-0000-4000-8000-000000000001", strings.NewReader(`{}`))
		r.Header.Set("X-Auth-Token", "expired-token")
		if duplicate {
			r.Header.Add("X-Auth-Token", "another-token")
		}
		r.Header.Set("X-Roles", "admin,service")
		r.Header.Set("X-Project-Id", "service")
		r.Header.Set("OpenStack-API-Version", "placement 1.36")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != 401 || w.Header().Get("WWW-Authenticate") == "" || w.Header().Get("OpenStack-API-Version") != "" {
			t.Fatalf("%d %v %s", w.Code, w.Header(), w.Body.String())
		}
	}
}
