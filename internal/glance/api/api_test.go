package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/PaoloCastanon/modules-devstack-optimized/internal/auth"
	"github.com/PaoloCastanon/modules-devstack-optimized/internal/glance/model"
	"github.com/PaoloCastanon/modules-devstack-optimized/internal/glance/repository"
	"github.com/PaoloCastanon/modules-devstack-optimized/internal/glance/service"
)

// These doubles exercise HTTP/business rules only; they do not validate MySQL.
type authFake struct {
	identity auth.Identity
	err      error
	calls    int
}

func (a *authFake) Authenticate(context.Context, string) (auth.Identity, error) {
	a.calls++
	return a.identity, a.err
}

type repoFake struct {
	image       model.Image
	err         error
	creates     int
	deleted     bool
	images      []model.Image
	listOptions model.ListOptions
}

func (r *repoFake) Create(ctx context.Context, image model.Image) error {
	r.creates++
	r.image = image
	return r.err
}
func (r *repoFake) Get(context.Context, string) (model.Image, error) {
	if r.deleted {
		return model.Image{}, repository.ErrNotFound
	}
	return r.image, r.err
}
func (r *repoFake) List(_ context.Context, o model.ListOptions) ([]model.Image, error) {
	r.listOptions = o
	if r.err != nil {
		return nil, r.err
	}
	if r.deleted {
		return []model.Image{}, nil
	}
	if r.images != nil {
		return r.images, nil
	}
	if r.image.ID != "" {
		return []model.Image{r.image}, nil
	}
	return []model.Image{}, nil
}
func (r *repoFake) Update(_ context.Context, _ string, f func(model.Image) (model.Image, error)) (model.Image, error) {
	if r.err != nil {
		return model.Image{}, r.err
	}
	if r.deleted || r.image.ID == "" {
		return model.Image{}, repository.ErrNotFound
	}
	i, err := f(r.image)
	if err == nil {
		r.image = i
	}
	return i, err
}
func (r *repoFake) Delete(_ context.Context, _ string, _ time.Time, f func(model.Image) error) error {
	if r.err != nil {
		return r.err
	}
	if r.deleted || r.image.ID == "" {
		return repository.ErrNotFound
	}
	if err := f(r.image); err != nil {
		return err
	}
	r.deleted = true
	return nil
}

func harness() (http.Handler, *repoFake, *authFake, *bytes.Buffer) {
	repo := &repoFake{}
	a := &authFake{identity: auth.Identity{UserID: "u", ProjectID: "p", Roles: []string{"member", "reader"}, ExpiresAt: time.Now().Add(time.Hour)}}
	logs := &bytes.Buffer{}
	logger := slog.New(slog.NewJSONHandler(logs, nil))
	h, err := New(service.New(repo, time.Now), a, "http://127.0.0.1:9293", "http://192.168.122.10/identity/v3", 1048576, logger)
	if err != nil {
		panic(err)
	} // Static test configuration must be valid.
	return h, repo, a, logs
}

func request(h http.Handler, method, path, body string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("X-Auth-Token", "fixture-user-token")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func TestCreateShowRepresentation(t *testing.T) {
	h, repo, _, _ := harness()
	w := request(h, "POST", "/v2/images", `{"name":"","disk_format":null,"container_format":null,"min_disk":0,"protected":false,"tags":[],"description":"","hw_rng_model":"virtio"}`)
	if w.Code != 201 {
		t.Fatalf("create %d %s", w.Code, w.Body.String())
	}
	var created map[string]any
	if json.Unmarshal(w.Body.Bytes(), &created) != nil {
		t.Fatal("invalid JSON")
	}
	if created["name"] != "" || created["status"] != "queued" || created["visibility"] != "shared" || created["owner"] != "p" || created["description"] != "" || created["hw_rng_model"] != "virtio" {
		t.Fatal(created)
	}
	for _, key := range []string{"size", "virtual_size", "checksum", "os_hash_algo", "os_hash_value", "disk_format", "container_format"} {
		value, exists := created[key]
		if !exists || value != nil {
			t.Fatalf("%s must exist and be null", key)
		}
	}
	if len(created) != 24 {
		t.Fatalf("unexpected representation keys (%d): %v", len(created), created)
	}
	if created["tags"] == nil || w.Header().Get("Location") != "http://127.0.0.1:9293/v2/images/"+repo.image.ID || w.Header().Get("Content-Type") != "application/json" || !strings.HasPrefix(w.Header().Get("X-Openstack-Request-Id"), "req-") {
		t.Fatal("wrong response headers or tags")
	}
	got := request(h, "GET", "/v2/images/"+repo.image.ID, "")
	var shown map[string]any
	_ = json.Unmarshal(got.Body.Bytes(), &shown)
	if got.Code != 200 || !reflect.DeepEqual(created, shown) {
		t.Fatalf("show mismatch: %s", got.Body.String())
	}
}

func TestProvidedUUIDAndReadFailures(t *testing.T) {
	h, repo, a, logs := harness()
	const id = "b2173dd3-7ad6-4362-baa6-a68bce3565cb"
	w := request(h, "POST", "/v2/images", `{"id":"`+id+`","visibility":"private","owner":"p"}`)
	if w.Code != 201 || repo.image.ID != id {
		t.Fatalf("provided UUID: %d %s", w.Code, w.Body.String())
	}
	a.identity.ProjectID = "other"
	if w = request(h, "GET", "/v2/images/"+id, ""); w.Code != 404 {
		t.Fatal("private image disclosed")
	}
	a.identity.ProjectID = "p"
	repo.err = errors.New("private-database-password")
	w = request(h, "GET", "/v2/images/"+id, "")
	if w.Code != 503 || strings.Contains(w.Body.String()+logs.String(), "private-database-password") {
		t.Fatal("unsafe database failure")
	}
}

func TestInputNullAndTypes(t *testing.T) {
	for _, body := range []string{`{}`, `{"name":null,"disk_format":null,"container_format":null}`, `{"name":"","tags":[],"min_ram":0,"protected":false}`} {
		h, _, _, _ := harness()
		w := request(h, "POST", "/v2/images", body)
		if w.Code != 201 {
			t.Fatalf("%s: %s", body, w.Body.String())
		}
	}
	for _, body := range []string{
		`null`, `[]`, `{`, `{} {}`, `{"name":1}`, `{"id":null}`, `{"visibility":null}`, `{"owner":null}`,
		`{"min_disk":null}`, `{"min_ram":1.5}`, `{"protected":null}`, `{"os_hidden":"false"}`, `{"tags":null}`, `{"tags":[null]}`, `{"tags":[1]}`,
		`{"description":null}`, `{"description":{}}`, `{"id":"invalid"}`, `{"disk_format":"invalid"}`, `{"min_ram":-1}`,
	} {
		t.Run(body, func(t *testing.T) {
			h, repo, _, _ := harness()
			w := request(h, "POST", "/v2/images", body)
			if w.Code != 400 || repo.creates != 0 {
				t.Fatalf("got %d: %s", w.Code, w.Body.String())
			}
		})
	}
	for _, body := range []string{`{"status":"active"}`, `{"size":0}`, `{"os_hash_algo":null}`, `{"self":"x"}`, `{"os_glance_task":"x"}`, `{"owner":"other"}`, `{"visibility":"public"}`} {
		h, repo, _, _ := harness()
		w := request(h, "POST", "/v2/images", body)
		if w.Code != 403 || repo.creates != 0 {
			t.Fatalf("forbidden %s: %d", body, w.Code)
		}
	}
}

func TestHTTPErrorMappingAndSecretRedaction(t *testing.T) {
	for _, tc := range []struct {
		name   string
		err    error
		status int
	}{
		{"duplicate", repository.ErrDuplicate, 409}, {"db unavailable", errors.New("secret-password SQL payload"), 503}, {"context timeout", context.DeadlineExceeded, 503},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h, repo, _, logs := harness()
			repo.err = tc.err
			w := request(h, "POST", "/v2/images", `{}`)
			if w.Code != tc.status {
				t.Fatalf("got %d", w.Code)
			}
			if strings.Contains(w.Body.String()+logs.String(), "secret-password") {
				t.Fatal("secret leaked")
			}
		})
	}
	h, repo, _, _ := harness()
	repo.err = repository.ErrNotFound
	w := request(h, "GET", "/v2/images/00000000-0000-4000-8000-000000000000", "")
	expected, err := os.ReadFile("../../../evidence/sprint2/glance/image-not-found.response.json")
	if err != nil {
		t.Fatal(err)
	}
	var original, actual any
	_ = json.Unmarshal(expected, &original)
	_ = json.Unmarshal(w.Body.Bytes(), &actual)
	if w.Code != 404 || !reflect.DeepEqual(original, actual) {
		t.Fatalf("historical 404 mismatch: %s", w.Body.String())
	}
	w = request(h, "GET", "/v2/images/bad", "")
	if w.Code != 400 {
		t.Fatal(w.Code)
	}
}

func TestAuthenticationAndUntrustedHeaders(t *testing.T) {
	h, repo, a, _ := harness()
	r := httptest.NewRequest("POST", "/v2/images", strings.NewReader(`{}`))
	r.Header.Set("X-Project-Id", "victim")
	r.Header.Set("X-Roles", "admin")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 401 || a.calls != 0 || repo.creates != 0 {
		t.Fatal("identity headers bypassed token requirement")
	}
	for _, tc := range []struct {
		err    error
		status int
	}{{auth.ErrInvalidToken, 401}, {auth.ErrUnavailable, 503}, {auth.ErrForbidden, 403}} {
		h, repo, a, _ := harness()
		a.err = tc.err
		w := request(h, "POST", "/v2/images", `{}`)
		if w.Code != tc.status || repo.creates != 0 {
			t.Fatalf("authentication mapping %d", w.Code)
		}
	}
	h, repo, a, _ = harness()
	a.identity.Roles = []string{"reader"}
	w = request(h, "POST", "/v2/images", `{}`)
	if w.Code != 403 || repo.creates != 0 {
		t.Fatal("reader wrote")
	}
	h, repo, a, _ = harness()
	a.identity.ExpiresAt = time.Now().Add(-time.Minute)
	w = request(h, "POST", "/v2/images", `{}`)
	if w.Code != 401 || repo.creates != 0 {
		t.Fatal("expired identity wrote")
	}
	h, repo, a, _ = harness()
	r = httptest.NewRequest("POST", "/v2/images", strings.NewReader(`{}`))
	r.Header.Set("X-Auth-Token", "fixture")
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("X-Project-Id", "victim")
	r.Header.Set("X-Roles", "admin")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 201 || repo.image.Owner != "p" {
		t.Fatal("client overrode verified project")
	}
}

func TestMediaSizeAndScope(t *testing.T) {
	h, repo, _, _ := harness()
	r := httptest.NewRequest("POST", "/v2/images", strings.NewReader(`{}`))
	r.Header.Set("X-Auth-Token", "fixture")
	r.Header.Set("Content-Type", "text/plain")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 415 {
		t.Fatal(w.Code)
	}
	r = httptest.NewRequest("POST", "/v2/images", strings.NewReader(`{}`))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Add("X-Auth-Token", "one")
	r.Header.Add("X-Auth-Token", "two")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 401 {
		t.Fatal("ambiguous authentication accepted")
	}
	r = httptest.NewRequest("POST", "/v2/images", strings.NewReader(`{}`))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("X-Auth-Token", "fixture")
	r.Header.Set("X-Service-Token", "unverified-service-token")
	// A legacy injected authenticator cannot silently ignore a service token.
	w = httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 403 || repo.creates != 0 {
		t.Fatal("authenticator without composite support accepted a service token")
	}
	w = request(h, "POST", "/v2/images", `{"description":"`+strings.Repeat("x", 1048576)+`"}`)
	if w.Code != 413 || repo.creates != 0 {
		t.Fatal("oversized request accepted")
	}
	for _, tc := range []struct {
		method, path string
		status       int
	}{{"GET", "/v2/images", 200}, {"POST", "/v2/images/00000000-0000-4000-8000-000000000000", 405}, {"PUT", "/v2/images/00000000-0000-4000-8000-000000000000/file", 404}, {"GET", "/v2/schemas/image", 200}} {
		if w := request(h, tc.method, tc.path, `{}`); w.Code != tc.status {
			t.Fatalf("unexpected route %s %s: %d", tc.method, tc.path, w.Code)
		}
	}
}

func TestHistoricalCreateSerialization(t *testing.T) {
	body, err := os.ReadFile("../../../evidence/sprint2/glance/image-create.response.json")
	if err != nil {
		t.Fatal(err)
	}
	var original map[string]any
	if json.Unmarshal(body, &original) != nil {
		t.Fatal("historical fixture")
	}
	value := func(key string) *string { s := original[key].(string); return &s }
	when, err := time.Parse(time.RFC3339, original["created_at"].(string))
	if err != nil {
		t.Fatal(err)
	}
	image, err := model.New(model.CreateInput{ID: value("id"), Name: value("name"), DiskFormat: value("disk_format"), ContainerFormat: value("container_format"), Visibility: value("visibility")}, original["owner"].(string), when)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(representation(image))
	if err != nil {
		t.Fatal(err)
	}
	var actual map[string]any
	_ = json.Unmarshal(encoded, &actual)
	if !reflect.DeepEqual(original, actual) {
		t.Fatalf("create representation differs: %s", encoded)
	}
}

func TestLocationUsesParsedBase(t *testing.T) {
	const id = "b2173dd3-7ad6-4362-baa6-a68bce3565cb"
	for _, base := range []string{"http://example.invalid", "http://example.invalid/", "https://example.invalid/image/", "https://example.invalid/image///"} {
		t.Run(base, func(t *testing.T) {
			_, repo, a, logs := harness()
			h, err := New(service.New(repo, time.Now), a, base, "http://192.168.122.10/identity/v3", 1048576, slog.New(slog.NewJSONHandler(logs, nil)))
			if err != nil {
				t.Fatal(err)
			}
			w := request(h, "POST", "/v2/images", `{"id":"B2173DD3-7AD6-4362-BAA6-A68BCE3565CB"}`)
			location, err := url.Parse(w.Header().Get("Location"))
			want := "/v2/images/" + id
			if strings.Contains(base, "/image/") {
				want = "/image" + want
			}
			if err != nil || w.Code != 201 || location.Path != want || location.RawQuery != "" || location.Fragment != "" || location.Host != "example.invalid" {
				t.Fatalf("invalid Location: %q, status=%d", w.Header().Get("Location"), w.Code)
			}
		})
	}
	for _, base := range []string{"http://example.invalid?", "http://example.invalid/#", "http://example.invalid?a=b", "http://example.invalid/#frag"} {
		t.Run("reject "+base, func(t *testing.T) {
			_, repo, a, logs := harness()
			h, err := New(service.New(repo, time.Now), a, base, "http://192.168.122.10/identity/v3", 1024, slog.New(slog.NewJSONHandler(logs, nil)))
			if err == nil || h != nil || repo.creates != 0 {
				t.Fatal("handler accepted invalid public URL")
			}
		})
	}
}
func TestMetadataUTF8(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		valid      bool
	}{
		{"invalid name", "{\"name\":\"" + string([]byte{0xff}) + "\"}", false},
		{"truncated multibyte property", "{\"description\":\"" + string([]byte{0xe2, 0x82}) + "\"}", false},
		{"invalid property key", "{\"" + string([]byte{0x80}) + "\":\"value\"}", false},
		{"Unicode", `{"name":"Imagen ñ 日本語 🚀","description":"Propiedad á 漢字"}`, true},
		{"replacement character explicitly supplied", `{"name":"�"}`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h, repo, _, _ := harness()
			w := request(h, "POST", "/v2/images", tc.body)
			if !tc.valid {
				if w.Code != 400 || repo.creates != 0 {
					t.Fatal("invalid UTF-8 was accepted")
				}
				return
			}
			if w.Code != 201 {
				t.Fatalf("valid Unicode rejected: %d", w.Code)
			}
			var out map[string]any
			_ = json.Unmarshal(w.Body.Bytes(), &out)
			var in map[string]any
			_ = json.Unmarshal([]byte(tc.body), &in)
			for k, v := range in {
				if out[k] != v {
					t.Fatal("Unicode changed")
				}
			}
		})
	}
	h, repo, _, _ := harness()
	w := request(h, "POST", "/v2/images", `{"description":"`+strings.Repeat("x", 1048576)+string([]byte{0xff})+`"}`)
	if w.Code != 413 || repo.creates != 0 {
		t.Fatal("UTF-8 validation bypassed body limit")
	}
}
func TestRequestObservabilityAndRedaction(t *testing.T) {
	for _, tc := range []struct {
		name, method, path, body, kind string
		status                         int
		authErr, repoErr               error
	}{
		{"success", "POST", "/v2/images", "{}", "", 201, nil, nil},
		{"invalid UTF8", "POST", "/v2/images", "{\"name\":\"" + string([]byte{0xff}) + "\"}", "invalid_request", 400, nil, nil},
		{"invalid token", "POST", "/v2/images", "{}", "invalid_token", 401, auth.ErrInvalidToken, nil},
		{"identity failure", "POST", "/v2/images", "{}", "identity_unavailable", 503, auth.ErrUnavailable, nil},
		{"metadata failure", "GET", "/v2/images/b2173dd3-7ad6-4362-baa6-a68bce3565cb?secret-query", "", "metadata_unavailable", 503, nil, errors.New("private-dsn-password")},
		{"unknown route", "GET", "/private-sensitive-path?secret-query", "", "not_found", 404, nil, nil},
		{"wrong method", "DELETE", "/v2/images", "{}", "method", 405, nil, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h, repo, a, logs := harness()
			a.err = tc.authErr
			repo.err = tc.repoErr
			w := request(h, tc.method, tc.path, tc.body)
			if w.Code != tc.status {
				t.Fatalf("status %d", w.Code)
			}
			var entry map[string]any
			if json.Unmarshal(bytes.TrimSpace(logs.Bytes()), &entry) != nil {
				t.Fatalf("expected one structured completion log: %s", logs.String())
			}
			if entry["request_id"] != w.Header().Get("X-Openstack-Request-Id") || entry["method"] != tc.method || entry["status"] != float64(tc.status) || entry["error_kind"] != tc.kind || entry["duration_ms"] == nil || entry["route"] == nil {
				t.Fatalf("missing correlation fields: %v", entry)
			}
			for _, secret := range []string{"fixture-user-token", "private-dsn-password", "secret-query", "private-sensitive-path", "b2173dd3-7ad6-4362-baa6-a68bce3565cb"} {
				if strings.Contains(logs.String(), secret) {
					t.Fatal("private request data logged")
				}
			}
		})
	}
}
