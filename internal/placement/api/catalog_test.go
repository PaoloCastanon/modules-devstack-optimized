package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/PaoloCastanon/modules-devstack-optimized/internal/placement/model"
	"github.com/PaoloCastanon/modules-devstack-optimized/internal/placement/repository"
	"github.com/PaoloCastanon/modules-devstack-optimized/internal/placement/service"
)

// Protocol stub only: persistence and mutation invariants are tested against
// actual MySQL. Unimplemented calls panic instead of silently succeeding.
type catalogStub struct {
	*memory
	repository.Catalog
	calls      int
	idempotent bool
	name       string
	generation int64
	filter     model.TraitFilter
}

func (r *catalogStub) Classes(context.Context) ([]model.CatalogEntry, error) {
	r.calls++
	return []model.CatalogEntry{{Name: "VCPU", Standard: true}}, nil
}
func (r *catalogStub) Class(_ context.Context, name string) (model.CatalogEntry, error) {
	r.calls++
	return model.CatalogEntry{Name: name}, nil
}
func (r *catalogStub) CreateClass(_ context.Context, name string, idempotent bool) (bool, error) {
	r.calls++
	r.name = name
	r.idempotent = idempotent
	return true, nil
}
func (r *catalogStub) RenameClass(_ context.Context, from, to string) error {
	r.calls++
	r.name = from + "->" + to
	return nil
}
func (r *catalogStub) EnsureTrait(_ context.Context, name string) (bool, error) {
	r.calls++
	r.name = name
	return true, nil
}
func (r *catalogStub) Trait(_ context.Context, name string) (model.CatalogEntry, error) {
	r.calls++
	return model.CatalogEntry{Name: name}, nil
}
func (r *catalogStub) Traits(_ context.Context, f model.TraitFilter) ([]string, error) {
	r.calls++
	r.filter = f
	return []string{"OWNER_NOVA"}, nil
}
func (r *catalogStub) ReplaceTraits(_ context.Context, _ string, generation int64, names []string) (model.TraitSet, error) {
	r.calls++
	r.generation = generation
	return model.TraitSet{Generation: generation + 1, Traits: append([]string{}, names...)}, nil
}
func (r *catalogStub) ReplaceAggregates(_ context.Context, _ string, names []string) ([]string, error) {
	r.calls++
	return names, nil
}

func TestB08VersionedProtocolsAndPolicies(t *testing.T) {
	_, identity, _ := harness(t)
	r := &catalogStub{memory: &memory{providers: map[string]model.Provider{}, inventories: map[string]model.Inventories{}}}
	h, err := New(service.New(r), identity, "http://localhost/placement", "http://localhost/identity/v3", 1024, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	call := func(method, path, body, version string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("X-Auth-Token", "fixture-token")
		req.Header.Set("OpenStack-API-Version", "placement "+version)
		if body != "" {
			req.Header.Set("Content-Type", "application/json")
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		return w
	}
	for _, tc := range []struct {
		path, version string
		status        int
	}{{"/resource_classes", "1.1", 404}, {"/resource_classes", "1.2", 200}, {"/traits", "1.5", 404}, {"/traits", "1.6", 200}} {
		before := r.calls
		w := call("GET", tc.path, "", tc.version)
		if w.Code != tc.status {
			t.Fatal(tc, w.Code, w.Body.String())
		}
		if tc.status == 404 && before != r.calls {
			t.Fatal("version gate reached persistence")
		}
	}
	w := call("POST", "/resource_classes", `{"name":"CUSTOM_B08"}`, "1.2")
	if w.Code != 201 || w.Header().Get("Location") != "http://localhost/placement/resource_classes/CUSTOM_B08" || r.idempotent {
		t.Fatal(w.Code, w.Header())
	}
	w = call("PUT", "/resource_classes/CUSTOM_B08", `{"name":"CUSTOM_NEW"}`, "1.6")
	if w.Code != 200 || r.name != "CUSTOM_B08->CUSTOM_NEW" {
		t.Fatal(w.Code, w.Body.String())
	}
	w = call("PUT", "/resource_classes/CUSTOM_B08", "", "1.7")
	if w.Code != 201 || !r.idempotent || w.Header().Get("Location") == "" {
		t.Fatal(w.Code, w.Header())
	}
	w = call("PUT", "/traits/CUSTOM_B08", "", "1.6")
	if w.Code != 201 || w.Header().Get("Location") != "http://localhost/placement/traits/CUSTOM_B08" {
		t.Fatal(w.Code, w.Header())
	}
	w = call("GET", "/traits/OWNER_NOVA", "", "1.6")
	if w.Code != 204 || w.Body.Len() != 0 {
		t.Fatal(w.Code, w.Body.String())
	}
	identity.who.Roles = []string{"reader"}
	before := r.calls
	w = call("PUT", "/traits/CUSTOM_NO_WRITE", "", "1.6")
	if w.Code != 403 || r.calls != before {
		t.Fatal("reader mutated catalogue", w.Code)
	}
	identity.who.Roles = []string{"service"}
	before = r.calls
	for _, body := range []string{`{"name":"VCPU"}`, `{"name":"CUSTOM_"}`, `{"name":null}`, `{"name":"CUSTOM_OK","extra":1}`} {
		w = call("POST", "/resource_classes", body, "1.2")
		if w.Code != 400 || r.calls != before {
			t.Fatal("schema failure reached repository", body, w.Code, w.Body.String())
		}
	}
	w = call("GET", "/traits?name=startswith:HW_&associated=FALSE", "", "1.6")
	if w.Code != 200 || r.filter.Prefix == nil || *r.filter.Prefix != "HW_" || r.filter.Associated == nil || *r.filter.Associated {
		t.Fatal(w.Code, r.filter)
	}
}

func TestB08ArraySchemaPaths(t *testing.T) {
	_, identity, _ := harness(t)
	r := &catalogStub{memory: &memory{providers: map[string]model.Provider{}, inventories: map[string]model.Inventories{}}}
	h, err := New(service.New(r), identity, "http://localhost/placement", "http://localhost/identity/v3", 1024, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest("PUT", "/resource_providers/4446c55f-ee69-4f16-b6c0-b562d5f200ab/traits", strings.NewReader(`{"resource_provider_generation":3,"traits":[null]}`))
	req.Header.Set("X-Auth-Token", "fixture-token")
	req.Header.Set("OpenStack-API-Version", "placement 1.6")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != 400 || r.calls != 0 || !strings.Contains(w.Body.String(), "instance['traits'][0]") {
		t.Fatal(w.Code, w.Body.String())
	}
	for version := 0; version <= 8; version++ {
		req := httptest.NewRequest("GET", "/", nil)
		req.Header.Set("OpenStack-API-Version", fmt.Sprintf("placement 1.%d", version))
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		if w.Code != 200 || w.Header().Get("OpenStack-API-Version") != fmt.Sprintf("placement 1.%d", version) {
			t.Fatal(version, w.Code, w.Header())
		}
	}
}

type missingCatalog struct{ *catalogStub }

func (*missingCatalog) Class(context.Context, string) (model.CatalogEntry, error) {
	return model.CatalogEntry{}, repository.ErrNotFound
}
func (*missingCatalog) DeleteClass(context.Context, string) error { return repository.ErrNotFound }
func (*missingCatalog) Aggregates(context.Context, string) ([]string, error) {
	return nil, repository.ErrNotFound
}

func TestCapturedB08Errors(t *testing.T) {
	data, err := os.ReadFile("../../../tests/compatibility/fixtures/placement-b08-errors.json")
	if err != nil {
		t.Fatal(err)
	}
	var capture struct {
		Cases []struct {
			Name    string
			Request struct {
				Method, Path string
				Headers      map[string]string
				Body         any
			}
			Response struct {
				Status  int
				Headers map[string]string
				Body    string `json:"body_text"`
			}
		}
	}
	if err := json.Unmarshal(data, &capture); err != nil {
		t.Fatal(err)
	}
	for _, tc := range capture.Cases {
		t.Run(tc.Name, func(t *testing.T) {
			_, identity, _ := harness(t)
			repo := &missingCatalog{&catalogStub{memory: &memory{providers: map[string]model.Provider{}, inventories: map[string]model.Inventories{}}}}
			h, err := New(service.New(repo), identity, "http://localhost/placement", "http://localhost/identity/v3", 4096, slog.New(slog.NewTextHandler(io.Discard, nil)))
			if err != nil {
				t.Fatal(err)
			}
			body := ""
			if tc.Request.Body != nil {
				encoded, _ := json.Marshal(tc.Request.Body)
				body = string(encoded)
			}
			req := httptest.NewRequest(tc.Request.Method, tc.Request.Path, strings.NewReader(body))
			req.Header.Set("X-Auth-Token", "fixture-token")
			for k, v := range tc.Request.Headers {
				req.Header.Set(k, v)
			}
			w := httptest.NewRecorder()
			h.ServeHTTP(w, req)
			if w.Code != tc.Response.Status {
				t.Fatalf("status=%d want=%d", w.Code, tc.Response.Status)
			}
			compareErrorOrState(t, w.Body.String(), tc.Response.Body)
			for _, key := range []string{"content-type", "openstack-api-version", "vary", "allow"} {
				if strings.ToLower(w.Header().Get(key)) != strings.ToLower(tc.Response.Headers[key]) {
					t.Errorf("header %s=%q want=%q", key, w.Header().Get(key), tc.Response.Headers[key])
				}
			}
		})
	}
}

type concurrentTraitCatalog struct{ *catalogStub }

func (*concurrentTraitCatalog) ReplaceTraits(context.Context, string, int64, []string) (model.TraitSet, error) {
	return model.TraitSet{}, repository.ErrConcurrent
}

func TestDeleteTraitsConcurrentConflict(t *testing.T) {
	_, identity, _ := harness(t)
	id := "4446c55f-ee69-4f16-b6c0-b562d5f200ab"
	r := &concurrentTraitCatalog{&catalogStub{memory: &memory{
		providers:   map[string]model.Provider{id: {UUID: id, Name: "concurrent", Generation: 1}},
		inventories: map[string]model.Inventories{},
	}}}
	h, err := New(service.New(r), identity, "http://localhost/placement", "http://localhost/identity/v3", 1024, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest("DELETE", "/resource_providers/"+id+"/traits", nil)
	req.Header.Set("X-Auth-Token", "fixture-token")
	req.Header.Set("OpenStack-API-Version", "placement 1.6")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != 409 {
		t.Fatal(w.Code, w.Body.String())
	}
	compareErrorOrState(t, w.Body.String(), `{"errors":[{"status":409,"title":"Conflict","detail":"There was a conflict when trying to complete your request.\n\n Another thread concurrently updated the resource provider data. Please retry your update  "}]}`)
}
