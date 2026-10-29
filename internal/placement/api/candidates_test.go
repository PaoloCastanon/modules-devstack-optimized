package api

import (
	"context"
	"github.com/PaoloCastanon/modules-devstack-optimized/internal/placement/model"
	"github.com/PaoloCastanon/modules-devstack-optimized/internal/placement/service"
	"io"
	"log/slog"
	"net/http/httptest"
	"testing"
)

type candidateStub struct{ *memory }

func (s *candidateStub) CandidateState(context.Context) (model.CandidateState, error) {
	return model.CandidateState{Classes: map[string]bool{"VCPU": true}, Traits: map[string]bool{}}, nil
}
func TestCandidateRouteUsesRepository(t *testing.T) {
	_, identity, _ := harness(t)
	repo := &candidateStub{&memory{providers: map[string]model.Provider{}, inventories: map[string]model.Inventories{}}}
	h, err := New(service.New(repo), identity, "http://localhost/placement", "http://localhost/identity/v3", 8192, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest("GET", "/allocation_candidates?resources=VCPU:1", nil)
	r.Header.Set("X-Auth-Token", "fixture-token")
	r.Header.Set("OpenStack-API-Version", "placement 1.36")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	sameJSON(t, w.Body.Bytes(), []byte(`{"allocation_requests":[],"provider_summaries":{}}`))
}
