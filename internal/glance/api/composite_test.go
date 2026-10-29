package api

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/PaoloCastanon/modules-devstack-optimized/internal/auth"
	"github.com/PaoloCastanon/modules-devstack-optimized/internal/config"
	"github.com/PaoloCastanon/modules-devstack-optimized/internal/glance/model"
	"github.com/PaoloCastanon/modules-devstack-optimized/internal/glance/service"
)

// Real adapter + API/service against a local protocol fixture and repository
// double. This exercises authorization, not real Keystone or SQL integration.
func compositeHTTPHarness(t *testing.T) (http.Handler, *repoFake, *bytes.Buffer, *atomic.Int32) {
	t.Helper()
	var validations atomic.Int32
	identityServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		project, user, roles := "service-project", "go-validator", []string{"service"}
		expires := time.Now().Add(time.Hour)
		if r.Method == http.MethodPost {
			w.Header().Set("X-Subject-Token", "private-validator")
			w.WriteHeader(201)
		} else {
			validations.Add(1)
			if r.Header.Get("X-Auth-Token") != "private-validator" || !r.URL.Query().Has("nocatalog") {
				t.Error("bad validation protocol")
			}
			project, user, roles = "p", "original-user", []string{"reader", "member"}
			switch r.Header.Get("X-Subject-Token") {
			case "nova", "expired-nova", "revoked-nova", "wrong-project-service", "wrong-role-service":
				if r.URL.Query().Has("allow_expired") {
					t.Error("service token used expiry allowance")
				}
				project, user, roles = "service-project", "nova-user", []string{"service", "admin"}
				switch r.Header.Get("X-Subject-Token") {
				case "expired-nova", "revoked-nova":
					w.WriteHeader(404)
					return
				case "wrong-project-service":
					project = "untrusted-project"
				case "wrong-role-service":
					roles = []string{"admin"}
				}
			case "reader":
				roles = []string{"reader"}
			case "foreign":
				project = "foreign-project"
			case "admin":
				project, roles = "admin-project", []string{"admin"}
			case "expired-user":
				if r.URL.Query().Get("allow_expired") != "true" {
					w.WriteHeader(404)
					return
				}
				expires = time.Now().Add(-time.Minute)
			case "member":
			case "invalid-user", "revoked-user", "outside-window-user":
				w.WriteHeader(404)
				return
			case "dependency-failure":
				w.WriteHeader(503)
				return
			default:
				w.WriteHeader(404)
				return
			}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"token": map[string]any{
			"user": map[string]string{"id": user}, "project": map[string]string{"id": project},
			"roles": roleClaims(roles), "expires_at": expires,
		}})
	}))
	t.Cleanup(identityServer.Close)
	repo := &repoFake{}
	logs := &bytes.Buffer{}
	k := auth.NewKeystone(identityServer.URL+"/v3", identityServer.Client(), config.ServiceCredential{ID: "fixture-app", Secret: "fixture-secret"})
	h, err := New(service.New(repo, time.Now), k, "http://127.0.0.1:9293", identityServer.URL+"/v3", 1048576, slog.New(slog.NewJSONHandler(logs, nil)))
	if err != nil {
		t.Fatal(err)
	}
	return h, repo, logs, &validations
}

func roleClaims(roles []string) []map[string]string {
	claims := []map[string]string{}
	for _, role := range roles {
		claims = append(claims, map[string]string{"name": role})
	}
	return claims
}

func TestCompositeHTTPAuthentication(t *testing.T) {
	for _, tc := range []struct {
		name        string
		users, svcs []string
		want, calls int
	}{
		{"neither", nil, nil, 401, 0},
		{"service only", nil, []string{"nova"}, 401, 0},
		{"user only B20", []string{"member"}, nil, 201, 1},
		{"both valid", []string{"member"}, []string{"nova"}, 201, 2},
		{"expired user allowed", []string{"expired-user"}, []string{"nova"}, 201, 2},
		{"expired without service", []string{"expired-user"}, nil, 401, 1},
		{"expired beyond window", []string{"outside-window-user"}, []string{"nova"}, 401, 2},
		{"invalid user", []string{"invalid-user"}, []string{"nova"}, 401, 2},
		{"revoked user", []string{"revoked-user"}, []string{"nova"}, 401, 2},
		{"invalid service", []string{"member"}, []string{"unknown"}, 401, 1},
		{"revoked service", []string{"member"}, []string{"revoked-nova"}, 401, 1},
		{"expired service", []string{"member"}, []string{"expired-nova"}, 401, 1},
		{"wrong service role", []string{"member"}, []string{"wrong-role-service"}, 403, 1},
		{"wrong service project", []string{"member"}, []string{"wrong-project-service"}, 403, 1},
		{"empty service header", []string{"member"}, []string{""}, 401, 0},
		{"blank service header", []string{"member"}, []string{" "}, 401, 0},
		{"duplicate service header", []string{"member"}, []string{"nova", "nova"}, 401, 0},
		{"duplicate user header", []string{"member", "reader"}, []string{"nova"}, 401, 0},
		{"comma service header", []string{"member"}, []string{"nova,nova"}, 401, 0},
		{"comma user header", []string{"member,reader"}, []string{"nova"}, 401, 0},
		{"dependency fails closed", []string{"dependency-failure"}, []string{"nova"}, 503, 2},
		{"reader is not service admin", []string{"reader"}, []string{"nova"}, 403, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h, repo, logs, validations := compositeHTTPHarness(t)
			r := httptest.NewRequest("POST", "/v2/images", strings.NewReader(`{"name":"nova-fixture","owner":"p"}`))
			r.Header.Set("Content-Type", "application/json")
			for _, value := range tc.users {
				r.Header.Add("X-Auth-Token", value)
			}
			for _, value := range tc.svcs {
				r.Header.Add("X-Service-Token", value)
			}
			// These client-controlled identity headers must never grant admin.
			r.Header.Set("X-Roles", "admin")
			r.Header.Set("X-Project-Id", "service-project")
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if w.Code != tc.want || validations.Load() != int32(tc.calls) {
				t.Fatalf("HTTP=%d validations=%d want %d/%d", w.Code, validations.Load(), tc.want, tc.calls)
			}
			if tc.want == 201 {
				if repo.creates != 1 || repo.image.Owner != "p" || repo.image.Status != "queued" {
					t.Fatal("original owner/state changed")
				}
			} else if repo.creates != 0 {
				t.Fatal("rejected request wrote metadata")
			}
			if tc.want == 401 && !strings.HasPrefix(w.Header().Get("WWW-Authenticate"), "Keystone uri=") {
				t.Fatal("lost authentication challenge")
			}
			for _, secret := range []string{"private-validator", "fixture-secret", "nova-user", "service-project", "X-Auth-Token", "X-Service-Token"} {
				if strings.Contains(logs.String(), secret) || strings.Contains(w.Body.String(), secret) {
					t.Fatal("authentication data leaked")
				}
			}
		})
	}
}

func TestCompositeHTTPUserAuthorization(t *testing.T) {
	for _, tc := range []struct {
		name, user, method, visibility, body string
		want                                 int
	}{
		{"owner private read", "member", "GET", "private", "", 200},
		{"foreign private read", "foreign", "GET", "private", "", 404},
		{"foreign shared read", "foreign", "GET", "shared", "", 404},
		{"foreign public read", "foreign", "GET", "public", "", 200},
		{"foreign community read", "foreign", "GET", "community", "", 200},
		{"reader patch denied", "reader", "PATCH", "private", `[{"op":"replace","path":"/name","value":"changed"}]`, 403},
		{"reader delete denied", "reader", "DELETE", "private", "", 403},
		{"foreign patch denied", "foreign", "PATCH", "private", `[{"op":"replace","path":"/name","value":"changed"}]`, 404},
		{"foreign delete denied", "foreign", "DELETE", "private", "", 404},
		{"owner patch", "member", "PATCH", "private", `[{"op":"replace","path":"/name","value":"changed"}]`, 200},
		{"owner delete", "member", "DELETE", "private", "", 204},
		{"original admin read", "admin", "GET", "private", "", 200},
		{"original admin patch", "admin", "PATCH", "private", `[{"op":"replace","path":"/name","value":"changed"}]`, 200},
		{"member cannot publish", "member", "PATCH", "private", `[{"op":"replace","path":"/visibility","value":"public"}]`, 403},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h, repo, _, _ := compositeHTTPHarness(t)
			// JSON owns independent values for every pointer, slice and map.
			// A struct copy would still share nullable fields and collections.
			snapshot := func(image model.Image) []byte {
				t.Helper()
				data, err := json.Marshal(image)
				if err != nil {
					t.Fatal(err)
				}
				return data
			}
			name := "original"
			image, err := model.New(model.CreateInput{
				Name: &name, Tags: []string{"original-tag"},
				Properties: map[string]string{"description": "original-description"},
			}, "p", time.Now().Add(-time.Minute))
			if err != nil {
				t.Fatal(err)
			}
			image.Visibility = tc.visibility
			repo.image = image
			before := snapshot(repo.image)
			r := httptest.NewRequest(tc.method, "/v2/images/"+image.ID, strings.NewReader(tc.body))
			r.Header.Set("X-Auth-Token", tc.user)
			r.Header.Set("X-Service-Token", "nova")
			r.Header.Set("Content-Type", "application/openstack-images-v2.1-json-patch")
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if w.Code != tc.want {
				t.Fatalf("got %d want %d: %s", w.Code, tc.want, w.Body.String())
			}
			if tc.want >= 400 {
				if repo.deleted || repo.creates != 0 || !bytes.Equal(snapshot(repo.image), before) {
					t.Fatal("denied request changed metadata")
				}
				return
			}
			get := func(path string) *httptest.ResponseRecorder {
				request := httptest.NewRequest("GET", path, nil)
				request.Header.Set("X-Auth-Token", tc.user)
				request.Header.Set("X-Service-Token", "nova")
				response := httptest.NewRecorder()
				h.ServeHTTP(response, request)
				return response
			}
			switch tc.method {
			case "PATCH":
				var expected model.Image
				if err := json.Unmarshal(before, &expected); err != nil {
					t.Fatal(err)
				}
				if repo.deleted || repo.creates != 0 || !repo.image.UpdatedAt.After(expected.UpdatedAt) {
					t.Fatal("PATCH did not persist an update")
				}
				changed := "changed"
				expected.Name, expected.UpdatedAt = &changed, repo.image.UpdatedAt
				if !bytes.Equal(snapshot(repo.image), snapshot(expected)) {
					t.Fatal("PATCH failed to persist the name or changed unrelated metadata")
				}
				var patched, shown map[string]any
				readback := get("/v2/images/" + expected.ID)
				if json.Unmarshal(w.Body.Bytes(), &patched) != nil || json.Unmarshal(readback.Body.Bytes(), &shown) != nil || readback.Code != 200 || patched["name"] != changed || shown["name"] != changed || !bytes.Equal(w.Body.Bytes(), readback.Body.Bytes()) {
					t.Fatal("PATCH response and subsequent GET do not reflect the stored update")
				}
			case "DELETE":
				if !repo.deleted || repo.creates != 0 || w.Body.Len() != 0 || !bytes.Equal(snapshot(repo.image), before) {
					t.Fatal("DELETE did not mark deletion or changed unrelated metadata")
				}
				if readback := get("/v2/images/" + image.ID); readback.Code != 404 {
					t.Fatal("deleted image remains readable")
				}
				var listed struct {
					Images []json.RawMessage `json:"images"`
				}
				list := get("/v2/images")
				if list.Code != 200 || json.Unmarshal(list.Body.Bytes(), &listed) != nil || listed.Images == nil || len(listed.Images) != 0 {
					t.Fatal("deleted image remains listed")
				}
			case "GET":
				if repo.deleted || repo.creates != 0 || !bytes.Equal(snapshot(repo.image), before) {
					t.Fatal("GET changed metadata")
				}
			}
		})
	}
}
