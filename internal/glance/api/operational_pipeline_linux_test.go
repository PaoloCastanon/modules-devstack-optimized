//go:build linux

package api

import (
	"bytes"
	"context"
	"crypto/md5"
	"crypto/sha512"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/PaoloCastanon/modules-devstack-optimized/internal/auth"
	"github.com/PaoloCastanon/modules-devstack-optimized/internal/config"
	"github.com/PaoloCastanon/modules-devstack-optimized/internal/glance/model"
	"github.com/PaoloCastanon/modules-devstack-optimized/internal/glance/repository"
	"github.com/PaoloCastanon/modules-devstack-optimized/internal/glance/service"
	"github.com/PaoloCastanon/modules-devstack-optimized/internal/glance/store"
	"github.com/google/uuid"
)

// Catalog + transfer double, independent of the real filesystem and the real
// Keystone adapter. The adapter talks to an HTTP FIXTURE, not real Keystone.
type operationalSQL struct{ *deletionPipelineSQL }

func (d *operationalSQL) Create(_ context.Context, i model.Image) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.image.ID != "" {
		return repository.ErrDuplicate
	}
	d.image = i
	return nil
}
func (d *operationalSQL) Update(_ context.Context, id string, f func(model.Image) (model.Image, error)) (model.Image, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.image.ID != id || d.image.Status == "deleted" {
		return model.Image{}, repository.ErrNotFound
	}
	i, err := f(d.image)
	if err == nil {
		d.image = i
	}
	return i, err
}

func TestOperationalPipelineWithKeystoneHTTPFixtureAndFilesystem(t *testing.T) {
	identity := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		roles, project := []string{"service"}, "service-project"
		if r.Method == "POST" {
			var doc map[string]any
			if json.NewDecoder(r.Body).Decode(&doc) != nil {
				t.Error("invalid service auth body")
			}
			w.Header().Set("X-Subject-Token", "fixture-service-token")
			w.WriteHeader(201)
		} else {
			if r.Header.Get("X-Auth-Token") != "fixture-service-token" {
				w.WriteHeader(401)
				return
			}
			switch r.Header.Get("X-Subject-Token") {
			case "fixture-member-token":
				roles, project = []string{"reader", "member"}, "p"
			case "fixture-reader-token":
				roles, project = []string{"reader"}, "p"
			case "fixture-other-token":
				roles, project = []string{"reader", "member"}, "other"
			default:
				w.WriteHeader(404)
				return
			}
		}
		roleObjects := []map[string]string{}
		for _, role := range roles {
			roleObjects = append(roleObjects, map[string]string{"name": role})
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"token": map[string]any{"user": map[string]string{"id": "fixture-user"}, "project": map[string]string{"id": project}, "roles": roleObjects, "expires_at": time.Now().Add(time.Hour)}})
	}))
	defer identity.Close()
	a := auth.NewKeystone(identity.URL+"/v3", identity.Client(), config.ServiceCredential{ID: "fixture-app-id", Secret: "fixture-app-secret"})
	d := &operationalSQL{&deletionPipelineSQL{pipelineSQL: &pipelineSQL{repoFake: &repoFake{}}}}
	root, scope := t.TempDir(), uuid.NewString()
	if err := os.Chmod(root, 0700); err != nil {
		t.Fatal(err)
	}
	if err := store.Initialize(root, scope); err != nil {
		t.Fatal(err)
	}
	var fs *store.Filesystem
	var u *service.UploadCoordinator
	open := func() {
		var err error
		fs, err = store.NewFilesystem(root, scope)
		if err != nil {
			t.Fatal(err)
		}
		u, err = service.NewUploadCoordinator(d, d, fs, service.UploadLimits{MaxBytes: 1024, MaxConcurrent: 2, ResolutionTimeout: time.Second, RecoveryTimeout: time.Second})
		if err != nil {
			t.Fatal(err)
		}
		if _, err = u.RecoverUploads(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	closeOwner := func() {
		u.StopAdmission()
		if err := u.WaitIdle(context.Background()); err != nil {
			t.Fatal(err)
		}
		if err := u.Close(); err != nil {
			t.Fatal(err)
		}
		if err := fs.Close(); err != nil {
			t.Fatal(err)
		}
	}
	open()
	defer func() { closeOwner() }()
	logs := &bytes.Buffer{}
	handler := func() http.Handler {
		h, err := New(service.New(d, time.Now), a, "http://127.0.0.1:9293", identity.URL+"/v3", 1048576, slog.New(slog.NewJSONHandler(logs, nil)), WithBinaryTransfers(u))
		if err != nil {
			t.Fatal(err)
		}
		return h
	}
	h := handler()
	call := func(method, path, body, token, typ string, want int) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		if token != "" {
			req.Header.Set("X-Auth-Token", token)
		}
		if typ != "" {
			req.Header.Set("Content-Type", typ)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		if w.Code != want {
			t.Fatalf("%s %s: %d want %d body=%s", method, path, w.Code, want, w.Body.String())
		}
		return w
	}
	for _, token := range []string{"", "fixture-invalid-token"} {
		w := call("GET", "/v2/images", "", token, "", 401)
		if w.Header().Get("WWW-Authenticate") == "" {
			t.Fatal("missing challenge")
		}
	}
	w := call("POST", "/v2/images", `{"name":"before","visibility":"private","disk_format":"raw","container_format":"bare","tags":["kept"],"custom":"kept"}`, "fixture-member-token", "application/json", 201)
	var i map[string]any
	if json.Unmarshal(w.Body.Bytes(), &i) != nil {
		t.Fatal("invalid metadata")
	}
	id := i["id"].(string)
	base := "/v2/images/" + id
	if _, err := uuid.Parse(id); err != nil || i["owner"] != "p" || i["status"] != "queued" {
		t.Fatal(i)
	}
	call("PUT", base+"/file", "abcdef", "fixture-reader-token", "application/octet-stream", 403)
	call("PUT", base+"/file", "abcdef", "fixture-other-token", "application/octet-stream", 404)
	call("PUT", base+"/file", "abcdef", "fixture-member-token", "application/octet-stream", 204)
	w = call("GET", base, "", "fixture-member-token", "", 200)
	json.Unmarshal(w.Body.Bytes(), &i)
	if i["status"] != "active" || i["size"] != float64(6) || i["virtual_size"] != float64(6) || i["checksum"] != fmt.Sprintf("%x", md5.Sum([]byte("abcdef"))) || i["os_hash_value"] != fmt.Sprintf("%x", sha512.Sum512([]byte("abcdef"))) {
		t.Fatal("receipt", i)
	}
	call("PATCH", base, `[{"op":"replace","path":"/name","value":"after"}]`, "fixture-member-token", patchMediaType, 200)
	call("PATCH", base, `[{"op":"replace","path":"/name","value":"partial"},{"op":"replace","path":"/size","value":999}]`, "fixture-member-token", patchMediaType, 403)
	w = call("GET", base+"/file", "", "fixture-member-token", "", 200)
	if w.Body.String() != "abcdef" || w.Header().Get("Content-Length") != "6" || w.Header().Get("Content-MD5") != fmt.Sprintf("%x", md5.Sum([]byte("abcdef"))) {
		t.Fatal("binary receipt")
	}
	call("GET", base, "", "fixture-other-token", "", 404)
	closeOwner()
	open()
	h = handler() // NEW components, same SQL double + real root; NOT process/MySQL restart
	w = call("GET", base, "", "fixture-member-token", "", 200)
	json.Unmarshal(w.Body.Bytes(), &i)
	if i["name"] != "after" || i["custom"] != "kept" || len(i["tags"].([]any)) != 1 {
		t.Fatal("metadata lost", i)
	}
	call("GET", base+"/file", "", "fixture-member-token", "", 200)
	call("DELETE", base, "", "fixture-reader-token", "", 403)
	call("DELETE", base, "", "fixture-member-token", "", 204)
	call("GET", base, "", "fixture-member-token", "", 404)
	w = call("GET", "/v2/images", "", "fixture-member-token", "", 200)
	if !strings.Contains(w.Body.String(), `"images":[]`) {
		t.Fatal("tombstone visible")
	}
	if d.op.Phase != model.DataCleaned || d.image.Status != "deleted" {
		t.Fatal("cleanup incomplete")
	}
	for _, dir := range []string{"staging", "objects"} {
		entries, err := os.ReadDir(root + "/" + dir)
		if err != nil || len(entries) != 0 {
			t.Fatal("files retained", dir, err)
		}
	}
	for _, secret := range []string{"fixture-member-token", "fixture-service-token", "fixture-app-secret", root} {
		if strings.Contains(logs.String(), secret) {
			t.Fatal("private data in logs")
		}
	}
}
