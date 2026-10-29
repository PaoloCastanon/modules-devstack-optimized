//go:build linux

package api

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/PaoloCastanon/modules-devstack-optimized/internal/auth"
	"github.com/PaoloCastanon/modules-devstack-optimized/internal/glance/model"
	"github.com/PaoloCastanon/modules-devstack-optimized/internal/glance/repository"
	"github.com/PaoloCastanon/modules-devstack-optimized/internal/glance/service"
	"github.com/PaoloCastanon/modules-devstack-optimized/internal/glance/store"
	"github.com/google/uuid"
)

// SQL/identity are DOUBLES. Filesystem is real but only on t.TempDir; no MySQL,
// Keystone, real server, restart or physical durability is demonstrated here.
type deletionPipelineSQL struct {
	*pipelineSQL
	tombstones                  int
	tombstoneErr, errorComplete error
	applied                     bool
}

func (d *deletionPipelineSQL) TombstoneData(ctx context.Context, id string, attempt *string, at time.Time, check func(model.Image) error) (model.DataState, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return model.DataState{}, err
	}
	if d.image.ID != id || d.image.Status == "deleted" {
		return model.DataState{}, repository.ErrNotFound
	}
	if err := check(d.image); err != nil {
		return model.DataState{}, err
	}
	if attempt == nil && d.op != nil || attempt != nil && (d.op == nil || d.op.AttemptID != *attempt) {
		return model.DataState{}, repository.ErrConflict
	}
	d.tombstones++
	if d.tombstoneErr != nil && !d.applied {
		return model.DataState{}, d.tombstoneErr
	}
	d.image.Status = "deleted"
	if d.op != nil {
		d.op.Phase = model.DataCleanup
		target, code := "deleted", "deleted"
		d.op.CleanupTarget = &target
		d.op.LastErrorCode = &code
		d.op.UpdatedAt = at
	}
	return model.DataState{}, d.tombstoneErr // caller must read fresh state, not trust this return
}
func (d *deletionPipelineSQL) CompleteDataCleanup(ctx context.Context, id, attempt string, at time.Time) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	if d.errorComplete != nil {
		return d.errorComplete
	}
	if id != d.image.ID || d.op == nil || d.op.AttemptID != attempt || d.op.Phase != model.DataCleanup || d.op.CleanupTarget == nil {
		return repository.ErrConflict
	}
	target := *d.op.CleanupTarget
	if target == "deleted" {
		if d.image.Status != "deleted" {
			return repository.ErrConflict
		}
	} else if target == "queued" || target == "killed" {
		if d.image.Status != "saving" {
			return repository.ErrConflict
		}
		d.image.Status = target
		d.image.Size = nil
		d.image.VirtualSize = nil
		d.image.Checksum = nil
		d.image.HashValue = nil
		d.image.HashAlgorithm = nil
	} else {
		return repository.ErrConflict
	}
	d.op.Phase = model.DataCleaned
	d.op.UpdatedAt = at
	return nil
}
func (d *deletionPipelineSQL) List(_ context.Context, _ model.ListOptions) ([]model.Image, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.image.Status == "deleted" {
		return []model.Image{}, nil
	}
	return []model.Image{d.image}, nil
}

func deletionHTTPFixture(t *testing.T) (func(auth.Identity) http.Handler, *deletionPipelineSQL, *service.UploadCoordinator, string, *bytes.Buffer) {
	t.Helper()
	root := t.TempDir()
	if err := os.Chmod(root, 0700); err != nil {
		t.Fatal(err)
	}
	scope := uuid.NewString()
	if err := store.Initialize(root, scope); err != nil {
		t.Fatal(err)
	}
	fs, err := store.NewFilesystem(root, scope)
	if err != nil {
		t.Fatal(err)
	}
	id, raw, bare := binaryImage, "raw", "bare"
	i, err := model.New(model.CreateInput{ID: &id, DiskFormat: &raw, ContainerFormat: &bare}, "p", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	d := &deletionPipelineSQL{pipelineSQL: &pipelineSQL{repoFake: &repoFake{image: i}}}
	u, err := service.NewUploadCoordinator(d, d, fs, service.UploadLimits{MaxBytes: 1024, MaxConcurrent: 2, ResolutionTimeout: time.Second, RecoveryTimeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := u.Close(); err != nil {
			t.Error(err)
		}
		if err := fs.Close(); err != nil {
			t.Error(err)
		}
	})
	if _, err = u.RecoverUploads(context.Background()); err != nil {
		t.Fatal(err)
	}
	logs := &bytes.Buffer{}
	makeHandler := func(who auth.Identity) http.Handler {
		h, err := New(service.New(d, time.Now), binaryIdentity{who}, "http://127.0.0.1:9293", "http://keystone/identity/v3", 1048576, slog.New(slog.NewJSONHandler(logs, nil)), WithBinaryTransfers(u))
		if err != nil {
			t.Fatal(err)
		}
		return h
	}
	return makeHandler, d, u, root, logs
}
func deleteRequest(h http.Handler) *httptest.ResponseRecorder {
	r := httptest.NewRequest("DELETE", "/v2/images/"+binaryImage, nil)
	r.Header.Set("X-Auth-Token", "fixture-token")
	r.Header.Set("X-Attempt-Id", "client-must-not-control")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func TestDeleteHTTPActualCoordinatorAndFilesystem(t *testing.T) {
	for _, tc := range []string{"queued", "active", "missing-blob", "corrupt-blob", "uncertain-applied"} {
		t.Run(tc, func(t *testing.T) {
			makeHandler, d, _, root, _ := deletionHTTPFixture(t)
			h := makeHandler(pipelineIdentity("member", "reader"))
			if tc != "queued" {
				w := httptest.NewRecorder()
				h.ServeHTTP(w, binaryRequest("PUT", strings.NewReader("abcdef")))
				if w.Code != 204 {
					t.Fatal(w.Code, w.Body.String())
				}
				key := filepath.Join(root, "objects", binaryImage+"-"+d.op.AttemptID+".blob")
				if tc == "missing-blob" {
					if err := os.Remove(key); err != nil {
						t.Fatal(err)
					}
				}
				if tc == "corrupt-blob" {
					if err := os.WriteFile(key, []byte("corrupt"), 0600); err != nil {
						t.Fatal(err)
					}
				}
				if tc == "missing-blob" || tc == "corrupt-blob" {
					get := httptest.NewRecorder()
					h.ServeHTTP(get, binaryRequest("GET", nil))
					if get.Code != 503 || d.image.Status != "active" || d.op.Phase != model.DataCommitted {
						t.Fatal("GET repaired corrupt state", get.Code)
					}
				}
			}
			if tc == "uncertain-applied" {
				d.tombstoneErr = repository.ErrCommitUncertain
				d.applied = true
			}
			w := deleteRequest(h)
			if w.Code != 204 || w.Body.Len() != 0 || d.image.Status != "deleted" {
				t.Fatal(w.Code, w.Body.String())
			}
			if tc == "queued" && d.op != nil {
				t.Fatal("fake attempt created")
			}
			if tc != "queued" && (d.op.Phase != model.DataCleaned || *d.op.CleanupTarget != "deleted") {
				t.Fatal("cleanup incomplete")
			}
			entries, err := os.ReadDir(filepath.Join(root, "objects"))
			if err != nil || len(entries) != 0 {
				t.Fatal("objects remained", err)
			}
			get := httptest.NewRecorder()
			h.ServeHTTP(get, binaryRequest("GET", nil))
			if get.Code != 404 {
				t.Fatal("download survived tombstone", get.Code)
			}
			meta := httptest.NewRequest("GET", "/v2/images/"+binaryImage, nil)
			meta.Header.Set("X-Auth-Token", "fixture-token")
			get = httptest.NewRecorder()
			h.ServeHTTP(get, meta)
			if get.Code != 404 {
				t.Fatal("metadata survived tombstone")
			}
			list := httptest.NewRequest("GET", "/v2/images", nil)
			list.Header.Set("X-Auth-Token", "fixture-token")
			get = httptest.NewRecorder()
			h.ServeHTTP(get, list)
			if get.Code != 200 || !strings.Contains(get.Body.String(), `"images":[]`) {
				t.Fatal("deleted image listed", get.Body.String())
			}
			if w = deleteRequest(h); w.Code != 404 {
				t.Fatal("repeated delete", w.Code)
			}
		})
	}
}

func TestDeleteHTTPAuthorizationAndSanitizedErrors(t *testing.T) {
	for _, tc := range []struct {
		name string
		code int
	}{
		{"reader", 403}, {"private", 404}, {"shared", 404}, {"public", 403}, {"community", 403}, {"protected", 403}, {"protected-admin", 403}, {"admin", 204}, {"missing-token", 401}, {"invalid-id", 400}, {"sql", 503}, {"uncertain-unapplied", 503}, {"complete-unavailable", 503},
	} {
		t.Run(tc.name, func(t *testing.T) {
			makeHandler, d, _, _, logs := deletionHTTPFixture(t)
			who := pipelineIdentity("member", "reader")
			switch tc.name {
			case "reader":
				who = pipelineIdentity("reader")
			case "private", "shared", "public", "community":
				who.ProjectID = "other"
				d.image.Visibility = tc.name
			case "protected":
				d.image.Protected = true
			case "protected-admin":
				d.image.Protected = true
				who = pipelineIdentity("admin")
			case "admin":
				who = pipelineIdentity("admin")
				who.ProjectID = "other"
			case "sql":
				d.tombstoneErr = errors.New("secret DSN SELECT path")
			case "uncertain-unapplied":
				d.tombstoneErr = repository.ErrCommitUncertain
			}
			h := makeHandler(who)
			if tc.name == "complete-unavailable" {
				w := httptest.NewRecorder()
				h.ServeHTTP(w, binaryRequest("PUT", strings.NewReader("abcdef")))
				if w.Code != 204 {
					t.Fatal(w.Code)
				}
				d.errorComplete = errors.New("secret SQL completion")
			}
			r := httptest.NewRequest("DELETE", "/v2/images/"+binaryImage, nil)
			r.Header.Set("X-Auth-Token", "fixture-token")
			if tc.name == "missing-token" {
				r.Header.Del("X-Auth-Token")
				r.Header.Set("X-Roles", "admin")
			}
			if tc.name == "invalid-id" {
				r.URL.Path = "/v2/images/not-a-uuid"
			}
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if w.Code != tc.code || strings.Contains(w.Body.String()+logs.String(), "secret") || d.op != nil && strings.Contains(w.Body.String()+logs.String(), d.op.AttemptID) {
				t.Fatal(w.Code, w.Body.String(), logs.String())
			}
			if tc.code == 401 && w.Header().Get("WWW-Authenticate") != `Keystone uri="http://keystone/identity"` {
				t.Fatal(w.Header())
			}
			if tc.name == "complete-unavailable" && (d.image.Status != "deleted" || d.op.Phase != model.DataCleanup) {
				t.Fatal("lost recovery evidence")
			}
			if tc.name == "complete-unavailable" && deleteRequest(h).Code != 404 {
				t.Fatal("repeated pending delete did not preserve 404")
			}
		})
	}
}

type blockedDownloadWriter struct {
	*httptest.ResponseRecorder
	entered, release chan struct{}
	once             sync.Once
}

func (w *blockedDownloadWriter) Write(p []byte) (int, error) {
	w.once.Do(func() { close(w.entered); <-w.release })
	return w.ResponseRecorder.Write(p)
}

func TestDeleteHTTPWhileDownloadOwnsRealDescriptor(t *testing.T) {
	makeHandler, _, _, _, _ := deletionHTTPFixture(t)
	h := makeHandler(pipelineIdentity("member", "reader"))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, binaryRequest("PUT", strings.NewReader("abcdef")))
	if w.Code != 204 {
		t.Fatal(w.Code)
	}
	writer := &blockedDownloadWriter{ResponseRecorder: httptest.NewRecorder(), entered: make(chan struct{}), release: make(chan struct{})}
	done := make(chan struct{})
	go func() { defer close(done); h.ServeHTTP(writer, binaryRequest("GET", nil)) }()
	select {
	case <-writer.entered:
	case <-time.After(time.Second):
		t.Fatal("download did not reach body")
	}
	deleted := deleteRequest(h)
	if deleted.Code != 204 {
		t.Error(deleted.Code, deleted.Body.String())
	}
	close(writer.release)
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("download leaked")
	}
	if writer.Code != 200 || writer.Body.String() != "abcdef" {
		t.Fatal("open fd did not finish", writer.Code, writer.Body.String())
	}
}

func TestDeleteHTTPDuringUploadIsControlledBusy(t *testing.T) {
	makeHandler, d, _, _, _ := deletionHTTPFixture(t)
	h := makeHandler(pipelineIdentity("member", "reader"))
	body := &pipelineBlockedBody{entered: make(chan struct{}), closed: make(chan struct{})}
	r := binaryRequest("PUT", nil)
	r.Body = body
	r.ContentLength = -1
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	r = r.WithContext(ctx)
	done := make(chan struct{})
	put := httptest.NewRecorder()
	go func() { defer close(done); h.ServeHTTP(put, r) }()
	select {
	case <-body.entered:
	case <-time.After(time.Second):
		t.Fatal("upload did not start")
	}
	w := deleteRequest(h)
	if w.Code != 503 {
		t.Error(w.Code, w.Body.String())
	}
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("writer leaked")
	}
	if d.tombstones != 0 || d.image.Status != "queued" || d.op.Phase != model.DataCleaned {
		t.Fatal("live writer was tombstoned")
	}
	if w = deleteRequest(h); w.Code != 204 {
		t.Fatal("retry after writer stopped", w.Code, w.Body.String())
	}
}

func TestDeleteHTTPDoesNotFallBackWithoutCoordinatedCapability(t *testing.T) {
	h, b, _, _, _ := binaryHarness(t)
	w := deleteRequest(h)
	if w.Code != 503 || b.puts != 0 || b.gets != 0 {
		t.Fatal("metadata fallback bypassed owner", w.Code)
	}
}

func TestDeleteHTTPInvalidTokenUsesExistingMiddleware(t *testing.T) {
	h, _, a, logs, _ := binaryHarness(t)
	a.err = auth.ErrInvalidToken
	w := deleteRequest(h)
	if w.Code != 401 || w.Header().Get("WWW-Authenticate") != `Keystone uri="http://keystone/identity"` || strings.Contains(logs.String(), "fixture-token") {
		t.Fatal(w.Code, w.Header(), logs.String())
	}
}
