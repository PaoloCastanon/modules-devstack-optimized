package main

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestRouteDeadlines(t *testing.T) {
	c := testConfig()
	for _, tc := range []struct {
		method, path string
		want         time.Duration
	}{{"GET", "/v2/images", c.RequestTimeout}, {"PUT", "/v2/images/id/file", c.Binary.UploadTimeout}, {"GET", "/v2/images/id/file", c.Binary.DownloadTimeout}, {"DELETE", "/v2/images/id", c.RequestTimeout}, {"PATCH", "/v2/images/id/file", c.RequestTimeout}} {
		r, _ := http.NewRequest(tc.method, "http://local"+tc.path, nil)
		if got := requestTimeout(c, r); got != tc.want {
			t.Fatal(tc, got)
		}
	}
}

func TestDeadlineSupportAndAdmissionFailClosed(t *testing.T) {
	gate := newRequestGate()
	called := false
	h := timedHandler(testConfig(), gate, http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true }))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/v2/images", nil))
	if w.Code != 503 || called {
		t.Fatal("served without effective network deadlines")
	}
	gate.stop()
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/v2/images", nil))
	if w.Code != 503 || called {
		t.Fatal("accepted request after admission stopped")
	}
	gate.wait()
}

func localListener(t *testing.T) net.Listener {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	return ln
}

func TestShutdownWaitsForHandlerAfterConnectionCancellation(t *testing.T) {
	ln := localListener(t)
	c := testConfig()
	c.Binary.ShutdownTimeout = 20 * time.Millisecond
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	entered, unblock, cancelled := make(chan struct{}), make(chan struct{}), make(chan struct{})
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(entered)
		<-r.Context().Done()
		close(cancelled)
		<-unblock
		w.WriteHeader(204)
	})
	done := make(chan error, 1)
	go func() { done <- serveHTTP(ctx, ln, c, h, nil, testLogger()) }()
	clientDone := make(chan struct{})
	go func() {
		defer close(clientDone)
		resp, err := http.Get("http://" + ln.Addr().String() + "/v2/images")
		if err == nil {
			resp.Body.Close()
		}
	}()
	<-entered
	cancel()
	select {
	case <-cancelled:
	case <-time.After(time.Second):
		t.Fatal("request not cancelled")
	}
	select {
	case <-done:
		t.Fatal("shutdown abandoned live handler")
	default:
	}
	close(unblock)
	select {
	case err := <-done:
		if err == nil || !strings.Contains(err.Error(), "drained") {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("shutdown did not join")
	}
	<-clientDone
}

func TestHTTPMetadataAndTransferHaveEffectiveDifferentDeadlines(t *testing.T) {
	ln := localListener(t)
	c := testConfig()
	c.RequestTimeout = 30 * time.Millisecond
	c.Binary.UploadTimeout = 250 * time.Millisecond
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "PUT" {
			if _, err := io.Copy(io.Discard, r.Body); err != nil {
				return
			}
			time.Sleep(70 * time.Millisecond)
		}
		if r.Context().Err() != nil {
			return
		}
		w.WriteHeader(204)
	})
	go func() { done <- serveHTTP(ctx, ln, c, h, nil, testLogger()) }()
	resp, err := http.Post("http://"+ln.Addr().String()+"/v2/images", "application/json", strings.NewReader("{}"))
	if err != nil || resp.StatusCode != 204 {
		t.Fatal(err)
	}
	resp.Body.Close()
	req, _ := http.NewRequest("PUT", "http://"+ln.Addr().String()+"/v2/images/id/file", strings.NewReader("bytes"))
	resp, err = http.DefaultClient.Do(req)
	if err != nil || resp.StatusCode != 204 {
		t.Fatal("metadata timeout clipped upload", err)
	}
	resp.Body.Close()
	// A client which never sends its declared body is unblocked by ReadDeadline.
	conn, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	_, err = io.WriteString(conn, "PUT /v2/images/id/file HTTP/1.1\r\nHost: localhost\r\nContent-Length: 100\r\n\r\n")
	if err != nil {
		t.Fatal(err)
	}
	_ = conn.SetReadDeadline(time.Now().Add(time.Second))
	buf := make([]byte, 1024)
	_, _ = conn.Read(buf)
	conn.Close()
	cancel()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestHTTPGracefulShutdown(t *testing.T) {
	ln := localListener(t)
	c := testConfig()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- serveHTTP(ctx, ln, c, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) }), nil, testLogger())
	}()
	resp, err := http.Get("http://" + ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	cancel()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestHTTPDeadlineCoversUnreadBodyAfterHandlerReturns(t *testing.T) {
	ln := localListener(t)
	c := testConfig()
	c.RequestTimeout = 40 * time.Millisecond
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- serveHTTP(ctx, ln, c, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Error(w, "Authentication required.", 401) }), nil, testLogger())
	}()
	defer func() { cancel(); <-done }()
	conn, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	// No request bytes follow the headers. net/http may drain this unread body
	// while flushing its buffered response AFTER the application handler returns.
	if _, err = io.WriteString(conn, "POST /v2/images HTTP/1.1\r\nHost: localhost\r\nContent-Length: 100\r\n\r\n"); err != nil {
		t.Fatal(err)
	}
	_ = conn.SetReadDeadline(time.Now().Add(400 * time.Millisecond))
	buf := make([]byte, 4096)
	_, err = conn.Read(buf)
	if timeout, ok := err.(net.Error); ok && timeout.Timeout() {
		t.Fatal("unread-body drain lost its network deadline after handler return")
	}
}
