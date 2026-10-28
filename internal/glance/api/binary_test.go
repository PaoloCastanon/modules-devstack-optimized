package api

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/PaoloCastanon/modules-devstack-optimized/internal/auth"
	"github.com/PaoloCastanon/modules-devstack-optimized/internal/glance/model"
	"github.com/PaoloCastanon/modules-devstack-optimized/internal/glance/repository"
	"github.com/PaoloCastanon/modules-devstack-optimized/internal/glance/service"
	"github.com/PaoloCastanon/modules-devstack-optimized/internal/glance/store"
)

const binaryImage = "00000000-0000-4000-8000-000000000001"
const binaryPath = "/v2/images/" + binaryImage + "/file"
const binaryMD5 = "e80b5017098950fc58aad83c8c14978e" // abcdef

type binaryDouble struct {
	max        int64
	err        error
	download   *service.Download
	puts, gets int
	identity   auth.Identity
	id         string
	expected   *int64
	read       int64
}

func (b *binaryDouble) MaxUploadBytes() int64 { return b.max }
func (b *binaryDouble) Upload(ctx context.Context, who auth.Identity, id string, src io.Reader, expected *int64) (model.DataContent, error) {
	b.puts++
	b.identity, b.id, b.expected = who, id, expected
	if b.err != nil {
		return model.DataContent{}, b.err
	}
	n, err := io.CopyBuffer(io.Discard, src, make([]byte, 32768))
	b.read = n
	return model.DataContent{Size: n}, err
}
func (b *binaryDouble) OpenDownload(_ context.Context, who auth.Identity, id string) (*service.Download, error) {
	b.gets++
	b.identity, b.id = who, id
	return b.download, b.err
}

type binaryBody struct {
	*bytes.Reader
	closes                     int
	readErr, seekErr, closeErr error
	onRead                     func()
}

func (b *binaryBody) Read(p []byte) (int, error) {
	if b.onRead != nil {
		b.onRead()
	}
	if b.readErr != nil {
		return 0, b.readErr
	}
	return b.Reader.Read(p)
}
func (b *binaryBody) Seek(offset int64, whence int) (int64, error) {
	if b.seekErr != nil {
		return 0, b.seekErr
	}
	return b.Reader.Seek(offset, whence)
}
func (b *binaryBody) Close() error { b.closes++; return b.closeErr }

func binaryHarness(t *testing.T) (http.Handler, *binaryDouble, *authFake, *bytes.Buffer, *binaryBody) {
	t.Helper()
	b := &binaryBody{Reader: bytes.NewReader([]byte("abcdef"))}
	transfer := &binaryDouble{max: 1024, download: &service.Download{Body: b, Size: 6, Checksum: binaryMD5}}
	a := &authFake{identity: auth.Identity{UserID: "u", ProjectID: "p", Roles: []string{"member", "reader"}, ExpiresAt: time.Now().Add(time.Hour)}}
	logs := &bytes.Buffer{}
	h, err := New(service.New(&repoFake{}, time.Now), a, "http://127.0.0.1:9293", "http://keystone/identity/v3", 1048576, slog.New(slog.NewJSONHandler(logs, nil)), WithBinaryTransfers(transfer))
	if err != nil {
		t.Fatal(err)
	}
	return h, transfer, a, logs, b
}
func binaryRequest(method string, body io.Reader) *http.Request {
	r := httptest.NewRequest(method, binaryPath, body)
	r.Header.Set("X-Auth-Token", "fixture-token")
	r.Header.Set("Content-Type", "application/octet-stream")
	return r
}

func TestBinaryPutSuccessAndDeclarations(t *testing.T) {
	for _, tc := range []string{"known", "unknown", "header-only", "both", "empty", "unicode-bytes"} {
		t.Run(tc, func(t *testing.T) {
			h, b, _, _, _ := binaryHarness(t)
			payload := "abcdef"
			if tc == "empty" {
				payload = ""
			}
			if tc == "unicode-bytes" {
				payload = "ñ\x00世界"
			}
			r := binaryRequest("PUT", strings.NewReader(payload))
			if tc == "unknown" || tc == "header-only" {
				r.ContentLength = -1
			}
			if tc == "header-only" || tc == "both" {
				r.Header.Set("X-Openstack-Image-Size", strconv.Itoa(len(payload)))
			}
			r.Header.Set("X-Project-Id", "attacker")
			r.Header.Set("X-Roles", "admin")
			r.Header.Set("X-Image-Attempt-Id", "attacker")
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if w.Code != 204 || w.Body.Len() != 0 || b.read != int64(len(payload)) || b.puts != 1 || b.identity.ProjectID != "p" || b.id != binaryImage {
				t.Fatal(w.Code, w.Body.String(), b)
			}
			if tc == "unknown" {
				if b.expected != nil {
					t.Fatal("invented size")
				}
			} else if b.expected == nil || *b.expected != int64(len(payload)) {
				t.Fatal("wrong size")
			}
		})
	}
}

func TestBinaryPutRejectsInputBeforeService(t *testing.T) {
	for _, tc := range []struct {
		name string
		code int
	}{
		{"media-missing", 415}, {"media-json", 415}, {"media-duplicate", 415}, {"negative-framing", 400}, {"size-negative", 400}, {"size-sign", 400}, {"size-empty", 400}, {"size-overflow", 400}, {"size-duplicate", 400}, {"size-comma", 400}, {"size-different", 400}, {"known-too-large", 413},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h, b, _, _, _ := binaryHarness(t)
			r := binaryRequest("PUT", strings.NewReader("abcdef"))
			switch tc.name {
			case "media-missing":
				r.Header.Del("Content-Type")
			case "media-json":
				r.Header.Set("Content-Type", "application/json")
			case "media-duplicate":
				r.Header.Add("Content-Type", "application/octet-stream")
			case "negative-framing":
				r.ContentLength = -2
			case "size-negative":
				r.Header.Set("X-Openstack-Image-Size", "-1")
			case "size-sign":
				r.Header.Set("X-Openstack-Image-Size", "+6")
			case "size-empty":
				r.Header["X-Openstack-Image-Size"] = []string{""}
			case "size-overflow":
				r.Header.Set("X-Openstack-Image-Size", "9223372036854775808")
			case "size-duplicate":
				r.Header["X-Openstack-Image-Size"] = []string{"6", "6"}
			case "size-comma":
				r.Header.Set("X-Openstack-Image-Size", "6,6")
			case "size-different":
				r.Header.Set("X-Openstack-Image-Size", "5")
			case "known-too-large":
				r.ContentLength = 1025
			}
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if w.Code != tc.code || b.puts != 0 {
				t.Fatal(w.Code, w.Body.String(), b.puts)
			}
		})
	}
}

func TestBinaryPutRealStreamBoundAndMismatch(t *testing.T) {
	for _, tc := range []struct {
		name    string
		length  int64
		payload string
		max     int64
		code    int
	}{
		{"excess-unknown", -1, "abcdefg", 6, 413}, {"short", 7, "abcdef", 1024, 400}, {"long", 5, "abcdef", 1024, 400}, {"at-limit", 6, "abcdef", 6, 204},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h, b, _, _, _ := binaryHarness(t)
			b.max = tc.max
			r := binaryRequest("PUT", strings.NewReader(tc.payload))
			r.ContentLength = tc.length
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if w.Code != tc.code {
				t.Fatal(w.Code, w.Body.String())
			}
		})
	}
}

func TestBinaryErrorMappingAndSanitizing(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		code int
	}{
		{"forbidden", model.Fail(model.Forbidden, "Denied."), 403}, {"missing", model.Fail(model.NotFound, "Not found."), 404}, {"conflict", model.Fail(model.Conflict, "Conflict."), 409},
		{"not-ready", service.ErrUploadNotReady, 503}, {"busy", service.ErrUploadBusy, 503}, {"uncertain", repository.ErrCommitUncertain, 503}, {"reconcile", service.ErrUploadReconciliation, 503},
		{"reconcile-after-limit", errors.Join(store.ErrTooLarge, service.ErrUploadReconciliation), 503}, {"gone", service.ErrUploadGone, 410}, {"full", store.ErrStoreFull, 413},
		{"interrupted", service.ErrUploadInterrupted, 400}, {"cancelled", context.Canceled, 503}, {"timeout", context.DeadlineExceeded, 503}, {"sql", service.ErrUploadUnavailable, 503},
		{"blob-missing", service.ErrBlobMissing, 503}, {"blob-corrupt", service.ErrBlobCorrupt, 503}, {"filesystem", service.ErrDownloadUnavailable, 503}, {"raw", errors.New("secret DSN /private/root SELECT token"), 503},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, method := range []string{"PUT", "GET"} {
				h, b, _, logs, body := binaryHarness(t)
				b.err = tc.err
				w := httptest.NewRecorder()
				h.ServeHTTP(w, binaryRequest(method, strings.NewReader("abcdef")))
				if w.Code != tc.code || strings.Contains(w.Body.String()+logs.String(), "secret") || strings.Contains(w.Body.String()+logs.String(), "/private") {
					t.Fatal(method, w.Code, w.Body.String(), logs.String())
				}
				if method == "GET" && body.closes != 1 {
					t.Fatal("error descriptor leaked")
				}
			}
		})
	}
}

func TestBinaryUsesB20MiddlewareAndClosesRejectedBodies(t *testing.T) {
	for _, tc := range []string{"missing", "invalid", "composite"} {
		t.Run(tc, func(t *testing.T) {
			h, b, a, logs, _ := binaryHarness(t)
			body := &binaryBody{Reader: bytes.NewReader([]byte("secret body"))}
			r := binaryRequest("PUT", nil)
			r.Body = body
			r.ContentLength = 11
			code := 401
			switch tc {
			case "missing":
				r.Header.Del("X-Auth-Token")
			case "invalid":
				a.err = auth.ErrInvalidToken
			case "composite":
				r.Header.Set("X-Service-Token", "secret service token")
				code = 403
			}
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if w.Code != code || b.puts != 0 || body.closes != 1 || body.Len() != 11 {
				t.Fatal(w.Code, b.puts, body.closes, body.Len())
			}
			if code == 401 && w.Header().Get("WWW-Authenticate") != `Keystone uri="http://keystone/identity"` {
				t.Fatal(w.Header())
			}
			if strings.Contains(logs.String(), "secret") || strings.Contains(logs.String(), binaryImage) {
				t.Fatal(logs.String())
			}
		})
	}
}

func TestBinaryGetHeadersAndRanges(t *testing.T) {
	for _, tc := range []struct {
		value              string
		code               int
		data, contentRange string
	}{
		{"", 200, "abcdef", ""}, {"bytes=1-3", 206, "bcd", "bytes 1-3/6"}, {"bytes=2-", 206, "cdef", "bytes 2-5/6"}, {"bytes=-2", 206, "ef", "bytes 4-5/6"},
		{"bytes=0-99", 206, "abcdef", "bytes 0-5/6"}, {"bytes=-99", 206, "abcdef", "bytes 0-5/6"}, {"bytes=0-1,3-4", 400, "", ""},
		{"bytes=6-", 416, "", "bytes */6"}, {"bytes=3-2", 416, "", "bytes */6"}, {"bytes=-0", 416, "", "bytes */6"}, {"items=0-2", 416, "", "bytes */6"}, {"bytes=9223372036854775808-", 416, "", "bytes */6"}, {"bytes=a-b", 416, "", "bytes */6"},
	} {
		t.Run(tc.value, func(t *testing.T) {
			h, _, _, logs, body := binaryHarness(t)
			r := binaryRequest("GET", nil)
			if tc.value != "" {
				r.Header.Set("Range", tc.value)
			}
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if w.Code != tc.code || body.closes != 1 || w.Header().Get("Content-Range") != tc.contentRange {
				t.Fatal(w.Code, w.Header(), body.closes)
			}
			if tc.code < 300 && (w.Body.String() != tc.data || w.Header().Get("Content-Length") != strconv.Itoa(len(tc.data)) || w.Header().Get("Content-MD5") != binaryMD5 || w.Header().Get("Content-Type") != "application/octet-stream") {
				t.Fatal(w.Header(), w.Body.String())
			}
			if w.Header().Get("ETag") != "" || strings.Contains(logs.String(), binaryImage) {
				t.Fatal("extra conditional/identifier")
			}
		})
	}
}

func TestBinaryGetEmptyAbsentAndMethodBoundaries(t *testing.T) {
	for _, tc := range []string{"no-content", "zero", "duplicate-range", "head", "delete", "seek-failure", "nil-result", "invalid-result", "close-error"} {
		t.Run(tc, func(t *testing.T) {
			h, b, _, logs, body := binaryHarness(t)
			r := binaryRequest("GET", nil)
			code := 200
			switch tc {
			case "no-content":
				b.download = nil
				b.err = service.ErrNoImageContent
				code = 204
			case "zero":
				b.download.Size = 0
				b.download.Checksum = "d41d8cd98f00b204e9800998ecf8427e"
				body.Reader = bytes.NewReader(nil)
			case "duplicate-range":
				r.Header["Range"] = []string{"bytes=0-1", "bytes=2-3"}
				code = 400
			case "head":
				r.Method = "HEAD"
				code = 405
			case "delete":
				r.Method = "DELETE"
				code = 405
			case "seek-failure":
				body.seekErr = errors.New("secret file")
				code = 503
			case "nil-result":
				b.download = nil
				code = 503
			case "invalid-result":
				b.download.Size = -1
				code = 503
			case "close-error":
				body.closeErr = errors.New("secret close")
			}
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if w.Code != code || strings.Contains(w.Body.String()+logs.String(), "secret") {
				t.Fatal(w.Code, w.Body.String(), logs.String())
			}
			if b.gets != 0 && b.download != nil && body.closes != 1 {
				t.Fatal("descriptor leaked")
			}
			if tc == "no-content" && w.Body.Len() != 0 {
				t.Fatal("204 body")
			}
		})
	}
}

type failingBinaryWriter struct{ *httptest.ResponseRecorder }

func (w failingBinaryWriter) Write(p []byte) (int, error) {
	return 0, errors.New("secret client write")
}

func TestBinaryGetAbortsAfterHeadersWithoutJSON(t *testing.T) {
	for _, tc := range []string{"writer", "short", "read", "cancel"} {
		t.Run(tc, func(t *testing.T) {
			h, _, _, logs, body := binaryHarness(t)
			r := binaryRequest("GET", nil)
			ctx, cancel := context.WithCancel(r.Context())
			defer cancel()
			r = r.WithContext(ctx)
			w := httptest.NewRecorder()
			var writer http.ResponseWriter = w
			switch tc {
			case "writer":
				writer = failingBinaryWriter{w}
			case "short":
				body.Reader = bytes.NewReader([]byte("ab"))
			case "read":
				body.readErr = errors.New("secret read")
			case "cancel":
				body.onRead = cancel
			}
			func() {
				defer func() {
					if recover() != http.ErrAbortHandler {
						t.Error("missing HTTP stream abort")
					}
				}()
				h.ServeHTTP(writer, r)
			}()
			if w.Code != 200 || strings.Contains(w.Body.String(), "{") || body.closes != 1 || strings.Contains(logs.String(), "secret") {
				t.Fatal(w.Code, w.Body.String(), body.closes, logs.String())
			}
		})
	}
}

func TestBinaryOptionRejectsInvalidLimits(t *testing.T) {
	for _, max := range []int64{0, -1, service.MaxServerUploadBytes + 1} {
		if WithBinaryTransfers(&binaryDouble{max: max})(&Handler{}) == nil {
			t.Fatal("invalid max accepted")
		}
	}
	if WithBinaryTransfers(nil)(&Handler{}) == nil {
		t.Fatal("nil dependency accepted")
	}
}

func TestBinaryPutClosesInputOnAllOutcomes(t *testing.T) {
	for _, tc := range []string{"success", "invalid-type", "unavailable", "read-error", "cancel-before-service"} {
		t.Run(tc, func(t *testing.T) {
			h, b, _, logs, _ := binaryHarness(t)
			body := &binaryBody{Reader: bytes.NewReader([]byte("abcdef"))}
			r := binaryRequest("PUT", nil)
			r.Body = body
			r.ContentLength = 6
			code := 204
			switch tc {
			case "invalid-type":
				r.Header.Set("Content-Type", "text/plain")
				code = 415
			case "unavailable":
				b.err = service.ErrUploadUnavailable
				code = 503
			case "read-error":
				body.readErr = io.ErrUnexpectedEOF
				code = 400
			case "cancel-before-service":
				ctx, cancel := context.WithCancel(r.Context())
				cancel()
				r = r.WithContext(ctx)
				code = 503
			}
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if w.Code != code || body.closes != 1 || strings.Contains(logs.String(), "fixture-token") {
				t.Fatal(w.Code, body.closes, logs.String())
			}
		})
	}
}

func TestBinaryCancelledDownloadBeforeHeaders(t *testing.T) {
	h, _, _, _, body := binaryHarness(t)
	r := binaryRequest("GET", nil)
	ctx, cancel := context.WithCancel(r.Context())
	cancel()
	r = r.WithContext(ctx)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 503 || body.closes != 1 || w.Header().Get("Content-Type") != "application/json" {
		t.Fatal(w.Code, w.Header(), body.closes)
	}
}
