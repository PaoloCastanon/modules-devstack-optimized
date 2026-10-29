package api

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/PaoloCastanon/modules-devstack-optimized/internal/placement/model"
	"github.com/PaoloCastanon/modules-devstack-optimized/internal/placement/repository"
	"github.com/PaoloCastanon/modules-devstack-optimized/internal/placement/service"
	"io"
	"log/slog"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

type allocationStub struct {
	*memory
	repository.Allocations
	calls    int
	consumer string
	values   map[string]map[string]int64
}

func (s *allocationStub) ConsumerAllocations(_ context.Context, id string) (model.ConsumerAllocations, error) {
	s.calls++
	s.consumer = id
	return model.ConsumerAllocations{Allocations: map[string]model.ResourceAllocation{}}, nil
}
func (s *allocationStub) ReplaceAllocations(_ context.Context, id string, update model.AllocationUpdate) error {
	values := update.Allocations
	s.calls++
	s.consumer = id
	s.values = values
	return nil
}
func TestB09AllocationProtocols(t *testing.T) {
	_, identity, _ := harness(t)
	repo := &allocationStub{memory: &memory{providers: map[string]model.Provider{}, inventories: map[string]model.Inventories{}}}
	h, err := New(service.New(repo), identity, "http://localhost/placement", "http://localhost/identity/v3", 4096, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	call := func(method, path, body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		r.Header.Set("X-Auth-Token", "fixture-token")
		r.Header.Set("OpenStack-API-Version", "placement 1.7")
		if body != "" {
			r.Header.Set("Content-Type", "application/json")
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	w := call("GET", "/allocations/unknown-consumer", "")
	if w.Code != 200 {
		t.Fatalf("empty consumer: %d %s", w.Code, w.Body.String())
	}
	sameJSON(t, w.Body.Bytes(), []byte(`{"allocations":{}}`))
	id := "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
	rp := "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"
	body := `{"allocations":[{"resource_provider":{"uuid":"` + rp + `"},"resources":{"VCPU":2}},{"resource_provider":{"uuid":"` + rp + `"},"resources":{"VCPU":4}}]}`
	w = call("PUT", "/allocations/"+strings.ReplaceAll(id, "-", ""), body)
	if w.Code != 204 || w.Body.Len() != 0 || repo.consumer != id || repo.values[rp]["VCPU"] != 4 {
		t.Fatal(w.Code, w.Body.String(), repo.values)
	}
	for _, bad := range []string{`{"allocations":[]}`, `{"allocations":[{"resource_provider":{"uuid":"bad"},"resources":{"VCPU":1}}]}`, `{"allocations":[{"resource_provider":{"uuid":"` + rp + `"},"resources":{}}]}`, `{"allocations":[{"resource_provider":{"uuid":"` + rp + `"},"resources":{"VCPU":0}}]}`, `{"allocations":{},"consumer_generation":null}`} {
		before := repo.calls
		w = call("PUT", "/allocations/"+id, bad)
		if w.Code != 400 || repo.calls != before {
			t.Fatal("invalid request reached repository", w.Code, w.Body.String())
		}
	}
	before := repo.calls
	w = call("PUT", "/allocations/not-uuid", body)
	if w.Code != 400 || repo.calls != before {
		t.Fatal("malformed consumer reached repository", w.Code)
	}
	identity.who.Roles = []string{"reader"}
	before = repo.calls
	w = call("PUT", "/allocations/"+id, body)
	if w.Code != 403 || repo.calls != before || !strings.Contains(w.Body.String(), "placement:allocations:update") {
		t.Fatal(w.Code, w.Body.String())
	}
}

func TestCapturedB09SchemaErrors(t *testing.T) {
	data, err := os.ReadFile("../../../tests/compatibility/fixtures/placement-b09-errors.json")
	if err != nil {
		t.Fatal(err)
	}
	var capture struct {
		Cases []struct {
			Name    string
			Request struct {
				Method, Path string
				Body         any
				Headers      map[string]string
			}
			Response struct {
				Status  int
				Headers map[string]string
				Body    string `json:"body_text"`
			}
		}
	}
	if err = json.Unmarshal(data, &capture); err != nil {
		t.Fatal(err)
	}
	for _, tc := range capture.Cases {
		t.Run(tc.Name, func(t *testing.T) {
			_, identity, _ := harness(t)
			repo := &allocationStub{memory: &memory{providers: map[string]model.Provider{}, inventories: map[string]model.Inventories{}}}
			h, err := New(service.New(repo), identity, "http://localhost/placement", "http://localhost/identity/v3", 4096, slog.New(slog.NewTextHandler(io.Discard, nil)))
			if err != nil {
				t.Fatal(err)
			}
			body, _ := json.Marshal(tc.Request.Body)
			req := httptest.NewRequest(tc.Request.Method, tc.Request.Path, bytes.NewReader(body))
			req.Header.Set("X-Auth-Token", "fixture-token")
			for k, v := range tc.Request.Headers {
				req.Header.Set(k, v)
			}
			w := httptest.NewRecorder()
			h.ServeHTTP(w, req)
			if w.Code != tc.Response.Status || repo.calls != 0 {
				t.Fatal(w.Code, w.Body.String(), repo.calls)
			}
			compareErrorOrState(t, w.Body.String(), tc.Response.Body)
			for _, key := range []string{"content-type", "openstack-api-version", "vary", "allow"} {
				if strings.ToLower(w.Header().Get(key)) != strings.ToLower(tc.Response.Headers[key]) {
					t.Errorf("%s differs", key)
				}
			}
		})
	}
}
