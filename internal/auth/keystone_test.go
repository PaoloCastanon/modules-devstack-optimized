package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/PaoloCastanon/modules-devstack-optimized/internal/config"
)

func tokenBody(expires time.Time, project string, roles ...string) map[string]any {
	items := []map[string]string{}
	for _, role := range roles {
		items = append(items, map[string]string{"name": role})
	}
	return map[string]any{"token": map[string]any{"user": map[string]string{"id": "verified-user"}, "project": map[string]string{"id": project}, "roles": items, "expires_at": expires}}
}

func fixtureCredential() config.ServiceCredential {
	return config.ServiceCredential{ID: "test-app", Secret: "test-secret"}
}

func TestKeystoneProtocolAndServiceCache(t *testing.T) {
	var issued, validated atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v3/auth/tokens" {
			t.Errorf("path %s", r.URL.Path)
		}
		if r.Method == "POST" {
			var body struct {
				Auth struct {
					Identity struct {
						Methods    []string                    `json:"methods"`
						Credential struct{ ID, Secret string } `json:"application_credential"`
					} `json:"identity"`
				} `json:"auth"`
			}
			if json.NewDecoder(r.Body).Decode(&body) != nil || body.Auth.Identity.Credential.ID != "test-app" || body.Auth.Identity.Credential.Secret != "test-secret" || len(body.Auth.Identity.Methods) != 1 || body.Auth.Identity.Methods[0] != "application_credential" {
				t.Error("bad service authentication request")
			}
			issued.Add(1)
			w.Header().Set("X-Subject-Token", "service-token")
			w.WriteHeader(201)
			_ = json.NewEncoder(w).Encode(tokenBody(time.Now().Add(time.Hour), "service-project", "service"))
			return
		}
		if r.Method != "GET" || r.Header.Get("X-Auth-Token") != "service-token" || r.Header.Get("X-Subject-Token") != "user-token" || !r.URL.Query().Has("nocatalog") {
			t.Error("bad validation request")
		}
		validated.Add(1)
		_ = json.NewEncoder(w).Encode(tokenBody(time.Now().Add(time.Hour), "project", "member", "reader"))
	}))
	defer server.Close()
	k := NewKeystone(server.URL+"/v3", server.Client(), fixtureCredential())
	type result struct {
		identity Identity
		err      error
	}
	results := make(chan result, 2)
	for i := 0; i < 2; i++ {
		go func() { who, err := k.Authenticate(context.Background(), "user-token"); results <- result{who, err} }()
	}
	for i := 0; i < 2; i++ {
		got := <-results
		who, err := got.identity, got.err
		if err != nil || who.UserID != "verified-user" || who.ProjectID != "project" || !who.HasRole("reader") {
			t.Fatalf("validation: %#v %v", who, err)
		}
	}
	if issued.Load() != 1 || validated.Load() != 2 {
		t.Fatalf("service cache %d; user validations %d", issued.Load(), validated.Load())
	}
}

func TestKeystoneFailsClosed(t *testing.T) {
	for _, tc := range []struct {
		name     string
		status   int
		body     any
		expected error
	}{
		{"missing user", 404, nil, ErrInvalidToken},
		{"service forbidden", 403, nil, ErrUnavailable},
		{"server failure", 500, nil, ErrUnavailable},
		{"malformed JSON", 200, "not JSON", ErrUnavailable},
		{"missing identity", 200, map[string]any{}, ErrUnavailable},
		{"expired user", 200, tokenBody(time.Now().Add(-time.Minute), "p", "reader"), ErrInvalidToken},
		{"unscoped user", 200, tokenBody(time.Now().Add(time.Hour), "", "reader"), ErrForbidden},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == "POST" {
					w.Header().Set("X-Subject-Token", "service-token")
					w.WriteHeader(201)
					_ = json.NewEncoder(w).Encode(tokenBody(time.Now().Add(time.Hour), "s", "service"))
					return
				}
				w.WriteHeader(tc.status)
				if s, ok := tc.body.(string); ok {
					_, _ = w.Write([]byte(s))
				} else if tc.body != nil {
					_ = json.NewEncoder(w).Encode(tc.body)
				}
			}))
			defer server.Close()
			_, err := NewKeystone(server.URL+"/v3", server.Client(), fixtureCredential()).Authenticate(context.Background(), "user-token")
			if !errors.Is(err, tc.expected) {
				t.Fatalf("got %v want %v", err, tc.expected)
			}
		})
	}
}

func TestKeystoneRejectsUnsafeServiceAndRedirects(t *testing.T) {
	for _, roles := range [][]string{{"admin"}, {"service", "admin"}, {"member"}} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-Subject-Token", "service-token")
			w.WriteHeader(201)
			_ = json.NewEncoder(w).Encode(tokenBody(time.Now().Add(time.Hour), "s", roles...))
		}))
		_, err := NewKeystone(server.URL+"/v3", server.Client(), fixtureCredential()).Authenticate(context.Background(), "user-token")
		server.Close()
		if !errors.Is(err, ErrUnavailable) {
			t.Fatal("unsafe service account accepted")
		}
	}
	var received atomic.Int32
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { received.Add(1) }))
	defer other.Close()
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, other.URL, http.StatusTemporaryRedirect)
	}))
	defer redirect.Close()
	_, err := NewKeystone(redirect.URL, redirect.Client(), fixtureCredential()).Authenticate(context.Background(), "user-token")
	if !errors.Is(err, ErrUnavailable) || received.Load() != 0 {
		t.Fatal("followed credential redirect")
	}
}

func TestKeystoneRetriesOwnExpiredTokenOnlyOnce(t *testing.T) {
	var issued, validated atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			issued.Add(1)
			w.Header().Set("X-Subject-Token", "service-token")
			w.WriteHeader(201)
			_ = json.NewEncoder(w).Encode(tokenBody(time.Now().Add(time.Hour), "s", "service"))
			return
		}
		validated.Add(1)
		w.WriteHeader(401)
	}))
	defer server.Close()
	_, err := NewKeystone(server.URL+"/v3", server.Client(), fixtureCredential()).Authenticate(context.Background(), "user-token")
	if !errors.Is(err, ErrUnavailable) || issued.Load() != 2 || validated.Load() != 2 {
		t.Fatal("unbounded retry or wrong error")
	}
}

func TestKeystoneTransportAndMissingToken(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		select {
		case <-r.Context().Done():
		case <-time.After(100 * time.Millisecond):
		}
	}))
	defer server.Close()
	k := NewKeystone(server.URL, &http.Client{Timeout: 20 * time.Millisecond}, fixtureCredential())
	for _, token := range []string{"", "bad token", "bad\ntoken"} {
		if _, err := k.Authenticate(context.Background(), token); !errors.Is(err, ErrInvalidToken) {
			t.Fatal(err)
		}
	}
	if calls.Load() != 0 {
		t.Fatal("invalid token reached upstream")
	}
	if _, err := k.Authenticate(context.Background(), "user-token"); !errors.Is(err, ErrUnavailable) {
		t.Fatal(err)
	}
}

// Signals that a waiter has reached its cancelable wait, without sleeps or
// inspecting mutex state. The embedded context supplies cancellation normally.
type watchedContext struct {
	context.Context
	waiting chan struct{}
	once    sync.Once
}

func (c *watchedContext) Done() <-chan struct{} {
	c.once.Do(func() { close(c.waiting) })
	return c.Context.Done()
}
func await[T any](t *testing.T, ch <-chan T) T {
	t.Helper()
	select {
	case value := <-ch:
		return value
	case <-time.After(2 * time.Second):
		t.Fatal("concurrent operation did not finish")
		var zero T
		return zero
	}
}
func TestKeystoneRefreshWaitCancellation(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	var unlock sync.Once
	var issued, validated atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			if issued.Add(1) == 1 {
				close(entered)
			}
			select {
			case <-release:
			case <-r.Context().Done():
				return
			}
			w.Header().Set("X-Subject-Token", "service-token")
			w.WriteHeader(201)
			_ = json.NewEncoder(w).Encode(tokenBody(time.Now().Add(time.Hour), "service-project", "service"))
			return
		}
		validated.Add(1)
		// Distinct client identities must still be validated independently.
		_ = json.NewEncoder(w).Encode(tokenBody(time.Now().Add(time.Hour), r.Header.Get("X-Subject-Token"), "reader"))
	}))
	defer server.Close()
	defer unlock.Do(func() { close(release) })
	k := NewKeystone(server.URL+"/v3", &http.Client{Timeout: time.Second}, fixtureCredential())
	first := make(chan error, 1)
	go func() {
		who, e := k.Authenticate(context.Background(), "first-project")
		if e == nil && who.ProjectID != "first-project" {
			e = errors.New("wrong first identity")
		}
		first <- e
	}()
	await(t, entered)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	waiter := &watchedContext{Context: ctx, waiting: make(chan struct{})}
	second := make(chan error, 1)
	go func() { _, e := k.Authenticate(waiter, "cancelled-project"); second <- e }()
	await(t, waiter.waiting)
	cancel()
	if err := await(t, second); !errors.Is(err, context.Canceled) || !errors.Is(err, ErrUnavailable) {
		t.Fatalf("cancellation: %v", err)
	}
	select {
	case <-first:
		t.Fatal("leader should still be refreshing")
	default:
	}
	if issued.Load() != 1 {
		t.Fatal("redundant refresh")
	}
	unlock.Do(func() { close(release) })
	if err := await(t, first); err != nil {
		t.Fatal(err)
	}
	who, err := k.Authenticate(context.Background(), "another-project")
	if err != nil || who.ProjectID != "another-project" || issued.Load() != 1 || validated.Load() != 2 {
		t.Fatal("cache shared client identity or lost service token")
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestKeystoneRefreshFailureTimeoutRecovery(t *testing.T) {
	for _, timeout := range []bool{false, true} {
		t.Run(map[bool]string{false: "renewal failed", true: "renewal deadline"}[timeout], func(t *testing.T) {
			entered, release := make(chan struct{}), make(chan struct{})
			var issued atomic.Int32
			client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				status := 200
				header := make(http.Header)
				var payload any
				if r.Method == "POST" {
					if issued.Add(1) == 1 {
						close(entered)
						select {
						case <-r.Context().Done():
							return nil, r.Context().Err()
						case <-release:
						}
						return &http.Response{StatusCode: 500, Header: header, Body: io.NopCloser(strings.NewReader(""))}, nil
					}
					status = 201
					header.Set("X-Subject-Token", "recovered-service")
					payload = tokenBody(time.Now().Add(time.Hour), "s", "service")
				} else {
					payload = tokenBody(time.Now().Add(time.Hour), "client-project", "reader")
				}
				data, _ := json.Marshal(payload)
				return &http.Response{StatusCode: status, Header: header, Body: io.NopCloser(bytes.NewReader(data))}, nil
			})}
			k := NewKeystone("https://identity.example.invalid/v3", client, fixtureCredential())
			leader := context.Background()
			var stop context.CancelFunc
			if timeout {
				leader, stop = context.WithTimeout(leader, 500*time.Millisecond)
				defer stop()
			}
			first := make(chan error, 1)
			go func() { _, e := k.Authenticate(leader, "client-one"); first <- e }()
			await(t, entered)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			waiter := &watchedContext{Context: ctx, waiting: make(chan struct{})}
			second := make(chan error, 1)
			go func() { _, e := k.Authenticate(waiter, "client-two"); second <- e }()
			await(t, waiter.waiting)
			if !timeout {
				close(release)
			}
			if e := await(t, first); !errors.Is(e, ErrUnavailable) || (timeout && !errors.Is(e, context.DeadlineExceeded)) {
				t.Fatalf("leader failure %v", e)
			}
			if e := await(t, second); !errors.Is(e, ErrUnavailable) {
				t.Fatalf("waiter failure %v", e)
			}
			if issued.Load() != 1 {
				t.Fatal("waiter redundantly renewed a failed flight")
			}
			who, e := k.Authenticate(context.Background(), "client-three")
			if e != nil || who.ProjectID != "client-project" || issued.Load() != 2 {
				t.Fatal("renewal did not recover")
			}
		})
	}
}
