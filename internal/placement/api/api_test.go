package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/PaoloCastanon/modules-devstack-optimized/internal/auth"
	"github.com/PaoloCastanon/modules-devstack-optimized/internal/placement/model"
	"github.com/PaoloCastanon/modules-devstack-optimized/internal/placement/repository"
	"github.com/PaoloCastanon/modules-devstack-optimized/internal/placement/service"
)

// All in-memory persistence and fake identities are restricted to tests.
type memory struct {
	mu          sync.Mutex
	providers   map[string]model.Provider
	inventories map[string]model.Inventories
}

func (r *memory) Create(_ context.Context, p model.Provider) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, other := range r.providers {
		if other.UUID == p.UUID {
			return repository.ErrDuplicateUUID
		}
		if other.Name == p.Name {
			return repository.ErrDuplicateName
		}
	}
	r.providers[p.UUID] = p
	r.inventories[p.UUID] = model.Inventories{Inventories: map[string]model.Inventory{}}
	return nil
}
func (r *memory) Get(_ context.Context, id string) (model.Provider, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	p, ok := r.providers[id]
	if !ok {
		return p, repository.ErrNotFound
	}
	return p, nil
}
func (r *memory) List(_ context.Context, f model.Filter) ([]model.Provider, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := []model.Provider{}
	for _, p := range r.providers {
		if (f.Name == nil || *f.Name == p.Name) && (f.UUID == nil || *f.UUID == p.UUID) {
			out = append(out, p)
		}
	}
	return out, nil
}
func (r *memory) Rename(_ context.Context, id, name string) (model.Provider, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	p, ok := r.providers[id]
	if !ok {
		return p, repository.ErrNotFound
	}
	for _, q := range r.providers {
		if q.UUID != id && q.Name == name {
			return p, repository.ErrDuplicate
		}
	}
	p.Name = name
	r.providers[id] = p
	return p, nil
}
func (r *memory) Delete(_ context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.providers[id]; !ok {
		return repository.ErrNotFound
	}
	delete(r.providers, id)
	delete(r.inventories, id)
	return nil
}
func (r *memory) Inventories(_ context.Context, id string) (model.Inventories, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	i, ok := r.inventories[id]
	if !ok {
		return i, repository.ErrNotFound
	}
	return i, nil
}
func (r *memory) ReplaceInventories(_ context.Context, id string, g int64, v map[string]model.Inventory) (model.Inventories, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	p, ok := r.providers[id]
	if !ok {
		return model.Inventories{}, repository.ErrNotFound
	}
	if p.Generation != g {
		return model.Inventories{}, repository.ErrConcurrent
	}
	p.Generation++
	r.providers[id] = p
	i := model.Inventories{Generation: p.Generation, Inventories: v}
	r.inventories[id] = i
	return i, nil
}

type identityStub struct {
	who auth.Identity
	err error
}

func (a *identityStub) Authenticate(context.Context, string) (auth.Identity, error) {
	return a.who, a.err
}
func harness(t *testing.T) (http.Handler, *identityStub, *bytes.Buffer) {
	t.Helper()
	r := &memory{providers: map[string]model.Provider{}, inventories: map[string]model.Inventories{}}
	a := &identityStub{who: auth.Identity{UserID: "nova", ProjectID: "service", Roles: []string{"service"}, ExpiresAt: time.Now().Add(time.Hour)}}
	log := &bytes.Buffer{}
	h, err := New(service.New(r), a, "http://localhost/placement", "http://localhost/identity/v3", 1024, slog.New(slog.NewJSONHandler(log, nil)))
	if err != nil {
		t.Fatal(err)
	}
	return h, a, log
}
func request(h http.Handler, method, path, body string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	r.Header.Set("X-Auth-Token", "test-token")
	if body != "" {
		r.Header.Set("Content-Type", "application/json")
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}
func readFixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile("../../../evidence/sprint2/placement/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
func sameJSON(t *testing.T, a, b []byte) {
	t.Helper()
	var one, two any
	if json.Unmarshal(a, &one) != nil || json.Unmarshal(b, &two) != nil || !reflect.DeepEqual(one, two) {
		t.Fatalf("JSON differs\ngot: %s\nwant: %s", a, b)
	}
}
func TestHistoricalProviderInventorySequence(t *testing.T) {
	h, _, _ := harness(t)
	create := readFixture(t, "provider-create.request.body")
	var p model.Provider
	_ = json.Unmarshal(create, &p)
	path := "/resource_providers/" + p.UUID
	w := request(h, "POST", "/resource_providers", string(create))
	if w.Code != 201 || w.Body.Len() != 0 || w.Header().Get("Location") != "http://localhost/placement"+path {
		t.Fatal(w.Code, w.Body.String(), w.Header())
	}
	w = request(h, "GET", path, "")
	if w.Code != 200 {
		t.Fatal(w.Code)
	}
	sameJSON(t, w.Body.Bytes(), readFixture(t, "provider-created.response.json"))
	w = request(h, "PUT", path+"/inventories", string(readFixture(t, "inventory-put.request.body")))
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	sameJSON(t, w.Body.Bytes(), readFixture(t, "inventory-put.response.json"))
	w = request(h, "GET", path+"/inventories/VCPU", "")
	sameJSON(t, w.Body.Bytes(), readFixture(t, "inventory-created.response.json"))
	// Generation belongs to inventory changes. Renaming neither accepts nor increments it.
	w = request(h, "PUT", path, `{"name":"renamed"}`)
	var renamed model.Provider
	_ = json.Unmarshal(w.Body.Bytes(), &renamed)
	if w.Code != 200 || renamed.Generation != 1 || renamed.Name != "renamed" {
		t.Fatal(w.Code, w.Body.String())
	}
	w = request(h, "PUT", path+"/inventories", string(readFixture(t, "inventory-put.request.body")))
	if w.Code != 409 {
		t.Fatal("stale generation accepted", w.Code)
	}
	w = request(h, "GET", path+"/inventories", "")
	sameJSON(t, w.Body.Bytes(), readFixture(t, "inventory-put.response.json"))
	if w = request(h, "DELETE", path, ""); w.Code != 204 || w.Body.Len() != 0 {
		t.Fatal(w.Code)
	}
	if w = request(h, "GET", path, ""); w.Code != 404 {
		t.Fatal(w.Code)
	}
}
func TestDiscoveryAuthAndVersions(t *testing.T) {
	h, a, log := harness(t)
	r := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"max_version":"1.0"`) {
		t.Fatal(w.Code, w.Body.String())
	}
	for _, version := range []string{"placement 1.1", "placement 1.36", "placement 2.0", "placement 0.9"} {
		r = httptest.NewRequest("GET", "/", nil)
		r.Header.Set("OpenStack-API-Version", version)
		w = httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != 406 {
			t.Fatal(version, w.Code)
		}
	}
	r = httptest.NewRequest("GET", "/resource_providers", nil)
	r.Header.Set("X-Roles", "admin")
	r.Header.Set("X-Project-Id", "service")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 401 || w.Header().Get("WWW-Authenticate") == "" {
		t.Fatal(w.Code, w.Header())
	}
	a.who.Roles = []string{"member", "reader"}
	if w = request(h, "GET", "/resource_providers", ""); w.Code != 403 {
		t.Fatal(w.Code)
	}
	a.who.Roles = []string{"service"}
	a.err = auth.ErrUnavailable
	if w = request(h, "GET", "/resource_providers", ""); w.Code != 503 {
		t.Fatal(w.Code)
	}
	if strings.Contains(log.String(), "test-token") {
		t.Fatal("token leaked")
	}
}
func TestMalformedRequestsAndInventoryRollback(t *testing.T) {
	h, _, _ := harness(t)
	id := "4446c55f-ee69-4f16-b6c0-b562d5f200ab"
	path := "/resource_providers/" + id
	for _, body := range []string{`null`, `[]`, `{}`, `{"name":null}`, `{"Name":"wrong-case"}`, `{"name":"x","extra":true}`, `{"name":"x"} {}`, `{"name":"x","uuid":"bad"}`} {
		if w := request(h, "POST", "/resource_providers", body); w.Code != 400 {
			t.Fatal(body, w.Code)
		}
	}
	if w := request(h, "POST", "/resource_providers", `{"uuid":"`+id+`","name":"host"}`); w.Code != 201 {
		t.Fatal(w.Code)
	}
	if w := request(h, "POST", "/resource_providers", `{"name":"host"}`); w.Code != 409 {
		t.Fatal(w.Code)
	}
	if w := request(h, "PUT", path, `{"name":"renamed","generation":0}`); w.Code != 400 {
		t.Fatal(w.Code)
	}
	for _, body := range []string{`{"inventories":{}}`, `{"resource_provider_generation":0,"inventories":null}`, `{"resource_provider_generation":0,"inventories":{"VCPU":{"total":null}}}`, `{"resource_provider_generation":0,"inventories":{"VCPU":{"total":4},"BAD":{"total":1}}}`, `{"resource_provider_generation":0,"inventories":{"VCPU":{"total":4,"reserved":4}}}`, `{"resource_provider_generation":0,"inventories":{"VCPU":{"total":4,"extra":1}}}`, `{"resource_provider_generation":0,"inventories":{"VCPU":{"Total":4}}}`} {
		if w := request(h, "PUT", path+"/inventories", body); w.Code != 400 {
			t.Fatal(body, w.Code, w.Body.String())
		}
	}
	w := request(h, "GET", path+"/inventories", "")
	sameJSON(t, w.Body.Bytes(), []byte(`{"resource_provider_generation":0,"inventories":{}}`))
	for _, query := range []string{"resources=VCPU:1", "uuid=bad", "name=%FF"} {
		if w := request(h, "GET", "/resource_providers?"+query, ""); w.Code != 400 {
			t.Fatal(query, w.Code)
		}
	}
	if w := request(h, "GET", path+"/inventories/VCPU", ""); w.Code != 404 {
		t.Fatal(w.Code)
	}
	if w := request(h, "GET", "/resource_providers?name=missing", ""); w.Code != 200 || !strings.Contains(w.Body.String(), `"resource_providers":[]`) {
		t.Fatal(w.Code, w.Body.String())
	}
}
func TestHTTPNegotiationBoundsAndRedaction(t *testing.T) {
	h, _, log := harness(t)
	cases := []struct {
		method, path, body, header, value string
		status                            int
	}{
		{"POST", "/resource_providers", `{"name":"x"}`, "Content-Type", "text/plain", 415},
		{"POST", "/resource_providers", strings.Repeat("x", 1025), "Content-Type", "application/json", 413},
		{"GET", "/resource_providers", "", "Accept", "text/plain", 406},
		{"GET", "/resource_providers", "", "OpenStack-API-Version", "placement bad", 400},
		{"GET", "/unknown/secret-token", "", "X-Service-Token", "composite-token", 403},
		{"PATCH", "/resource_providers", "", "Accept", "application/json", 405},
	}
	for _, tc := range cases {
		r := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
		r.Header.Set("X-Auth-Token", "test-token")
		r.Header.Set(tc.header, tc.value)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != tc.status {
			t.Fatal(tc, w.Code, w.Body.String())
		}
	}
	if strings.Contains(log.String(), "secret-token") || strings.Contains(log.String(), "composite-token") {
		t.Fatal("arbitrary request data leaked")
	}
}
func TestConcurrentInventoryGeneration(t *testing.T) {
	h, _, _ := harness(t)
	create := readFixture(t, "provider-create.request.body")
	request(h, "POST", "/resource_providers", string(create))
	var p model.Provider
	_ = json.Unmarshal(create, &p)
	body := string(readFixture(t, "inventory-put.request.body"))
	ch := make(chan int, 2)
	for n := 0; n < 2; n++ {
		go func() { ch <- request(h, "PUT", "/resource_providers/"+p.UUID+"/inventories", body).Code }()
	}
	a, b := <-ch, <-ch
	if !(a == 200 && b == 409 || a == 409 && b == 200) {
		t.Fatal(a, b)
	}
}

func TestHistoricalNotFound(t *testing.T) {
	h, _, _ := harness(t)
	w := request(h, "GET", "/resource_providers/00000000-0000-4000-8000-000000000000", "")
	if w.Code != 404 {
		t.Fatal(w.Code)
	}
	var actual, original struct {
		Errors []map[string]any `json:"errors"`
	}
	if json.Unmarshal(w.Body.Bytes(), &actual) != nil || json.Unmarshal(readFixture(t, "provider-not-found.response.json"), &original) != nil {
		t.Fatal("invalid error JSON")
	}
	// Request IDs are volatile. Status, title and detail remain exact.
	for _, item := range actual.Errors {
		delete(item, "request_id")
	}
	for _, item := range original.Errors {
		delete(item, "request_id")
	}
	if !reflect.DeepEqual(actual, original) {
		t.Fatal(actual, original)
	}
}

func TestUUIDLookupAndEmptyNameFollowPinnedSource(t *testing.T) {
	h, _, _ := harness(t)
	body := string(readFixture(t, "provider-create.request.body"))
	if w := request(h, "POST", "/resource_providers", body); w.Code != 201 {
		t.Fatal(w.Code)
	}
	compact := "4446c55fee694f16b6c0b562d5f200ab"
	if w := request(h, "GET", "/resource_providers/"+compact, ""); w.Code != 404 {
		t.Fatal("path UUID was rewritten", w.Code)
	}
	w := request(h, "GET", "/resource_providers?uuid="+compact, "")
	if w.Code != 400 {
		t.Fatal("compact UUID query must be rejected", w.Code, w.Body.String())
	}
	w = request(h, "GET", "/resource_providers?name=", "")
	if w.Code != 200 || strings.Contains(w.Body.String(), `"resource_providers":[]`) {
		t.Fatal("empty name should not filter", w.Code, w.Body.String())
	}
}

func TestLiveIntegerFormsAndRepeatedQuery(t *testing.T) {
	h, _, _ := harness(t)
	body := string(readFixture(t, "provider-create.request.body"))
	var p model.Provider
	_ = json.Unmarshal([]byte(body), &p)
	if w := request(h, "POST", "/resource_providers", body); w.Code != 201 {
		t.Fatal(w.Code)
	}
	path := "/resource_providers/" + p.UUID + "/inventories"
	for n, total := range []string{"3.0", "3e0", "30e-1"} {
		payload := fmt.Sprintf(`{"resource_provider_generation":%d,"inventories":{"VCPU":{"total":%s}}}`, n, total)
		if w := request(h, "PUT", path, payload); w.Code != 200 {
			t.Fatal(total, w.Code, w.Body.String())
		}
	}
	w := request(h, "GET", "/resource_providers?name=absent&name="+url.QueryEscape(p.Name), "")
	if w.Code != 200 || !strings.Contains(w.Body.String(), p.UUID) {
		t.Fatal("last query value must win at 1.0", w.Code, w.Body.String())
	}
	if w := request(h, "PUT", path, `{"resource_provider_generation":3,"inventories":{"VCPU":{"total":3.1}}}`); w.Code != 400 {
		t.Fatal("fraction accepted", w.Code)
	}
}

func TestLiveKeystone401Shape(t *testing.T) {
	h, _, _ := harness(t)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/resource_providers", nil))
	sameJSON(t, w.Body.Bytes(), []byte(`{"error":{"code":401,"title":"Unauthorized","message":"The request you have made requires authentication."}}`))
	if w.Header().Get("OpenStack-API-Version") != "" || w.Header().Get("Vary") != "" {
		t.Fatal("auth middleware must reject before microversion headers", w.Header())
	}
}

func TestLiveUUIDCaseInsensitiveLookup(t *testing.T) {
	h, _, _ := harness(t)
	request(h, "POST", "/resource_providers", string(readFixture(t, "provider-create.request.body")))
	id := "4446c55f-ee69-4f16-b6c0-b562d5f200ab"
	if w := request(h, "GET", "/resource_providers/"+strings.ToUpper(id), ""); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
}

func TestLiveStableErrorContracts(t *testing.T) {
	data, err := os.ReadFile("../../../tests/compatibility/fixtures/placement-live-errors.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Cases map[string]struct {
			Status  int               `json:"status"`
			Headers map[string]string `json:"headers"`
			Body    map[string]any    `json:"body"`
		} `json:"cases"`
	}
	if json.Unmarshal(data, &fixture) != nil {
		t.Fatal("invalid fixture")
	}
	id := "4446c55f-ee69-4f16-b6c0-b562d5f200ab"
	path := "/resource_providers/" + id
	for label, want := range fixture.Cases {
		t.Run(label, func(t *testing.T) {
			h, a, _ := harness(t)
			method, target, body := "GET", path, ""
			if label != "inventories-provider-missing" && label != "missing-token" {
				request(h, "POST", "/resource_providers", string(readFixture(t, "provider-create.request.body")))
			}
			switch label {
			case "stale-generation":
				request(h, "PUT", path+"/inventories", `{"resource_provider_generation":0,"inventories":{"VCPU":{"total":4}}}`)
				method, target, body = "PUT", path+"/inventories", `{"resource_provider_generation":0,"inventories":{}}`
			case "unknown-class":
				method, target, body = "PUT", path+"/inventories", `{"resource_provider_generation":0,"inventories":{"CUSTOM_B07_UNKNOWN":{"total":1}}}`
			case "reader-forbidden":
				a.who.Roles = []string{"reader"}
			case "inventories-provider-missing":
				target = path + "/inventories"
			}
			w := request(h, method, target, body)
			if label == "missing-token" {
				r := httptest.NewRequest("GET", target, nil)
				w = httptest.NewRecorder()
				h.ServeHTTP(w, r)
			}
			if w.Code != want.Status {
				t.Fatal(w.Code, want.Status)
			}
			var got map[string]any
			if json.Unmarshal(w.Body.Bytes(), &got) != nil {
				t.Fatal(w.Body.String())
			}
			if list, ok := got["errors"].([]any); ok {
				for _, item := range list {
					delete(item.(map[string]any), "request_id")
				}
			}
			if !reflect.DeepEqual(got, want.Body) {
				t.Fatal(got, want.Body)
			}
			for _, header := range []string{"content-type", "openstack-api-version", "vary", "www-authenticate"} {
				expected := want.Headers[header]
				if header == "www-authenticate" && expected != "" {
					expected = `Keystone uri="http://localhost/identity"`
				}
				equal := w.Header().Get(header) == expected
				if header == "vary" {
					equal = strings.EqualFold(w.Header().Get(header), expected)
				}
				if !equal {
					t.Fatal(header, w.Header().Get(header), want.Headers[header])
				}
			}
		})
	}
}
