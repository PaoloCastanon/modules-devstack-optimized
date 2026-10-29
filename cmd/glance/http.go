package main

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/PaoloCastanon/modules-devstack-optimized/internal/config"
	"github.com/PaoloCastanon/modules-devstack-optimized/internal/glance/service"
)

// Tracks handler completion even after Server.Close (which closes connections,
// but does not join handlers). Add/Wait never race on a zero WaitGroup.
type requestGate struct {
	mu       sync.Mutex
	stopping bool
	active   int
	changed  chan struct{}
}

func newRequestGate() *requestGate { return &requestGate{changed: make(chan struct{})} }
func (g *requestGate) enter() bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.stopping {
		return false
	}
	g.active++
	return true
}
func (g *requestGate) leave() {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.active--
	close(g.changed)
	g.changed = make(chan struct{})
}
func (g *requestGate) stop() { g.mu.Lock(); g.stopping = true; g.mu.Unlock() }
func (g *requestGate) wait() {
	for {
		g.mu.Lock()
		n, ch := g.active, g.changed
		g.mu.Unlock()
		if n == 0 {
			return
		}
		<-ch
	}
}

func requestTimeout(c config.Config, r *http.Request) time.Duration {
	// Broad classification cannot extend a metadata route; only /file receives
	// the transfer deadline. The mux still rejects unknown routes/methods.
	if c.Binary.Enabled && strings.HasPrefix(r.URL.Path, "/v2/images/") && strings.HasSuffix(r.URL.Path, "/file") {
		if r.Method == http.MethodPut {
			return c.Binary.UploadTimeout
		}
		if r.Method == http.MethodGet {
			return c.Binary.DownloadTimeout
		}
	}
	return c.RequestTimeout
}

func timedHandler(c config.Config, g *requestGate, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !g.enter() {
			http.Error(w, "Service shutting down.", 503)
			return
		}
		defer g.leave()
		d := requestTimeout(c, r)
		ctx, cancel := context.WithTimeout(r.Context(), d)
		defer cancel()
		rc := http.NewResponseController(w)
		// Absolute finite transfer deadlines, including the full pre-hash pass.
		// Response allowance permits bounded detached resolution/cleanup to finish.
		if rc.SetReadDeadline(time.Now().Add(d)) != nil || rc.SetWriteDeadline(time.Now().Add(d+c.Binary.ResolutionTimeout+time.Second)) != nil {
			http.Error(w, "HTTP deadlines unavailable.", 503)
			return
		}
		done := make(chan struct{})
		stop := context.AfterFunc(ctx, func() { _ = rc.SetReadDeadline(time.Now()); _ = rc.SetWriteDeadline(time.Now()); close(done) })
		defer func() {
			if !stop() {
				<-done
			}
			// Keep deadlines through net/http's post-handler response flush and
			// unread-body drain. The server resets them for keepalive/next request.
		}()
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func serveHTTP(ctx context.Context, ln net.Listener, c config.Config, handler http.Handler, owner *service.UploadCoordinator, logger *slog.Logger) error {
	requests, cancel := context.WithCancel(context.Background())
	defer cancel()
	gate := newRequestGate()
	server := &http.Server{ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 30 * time.Second, Handler: timedHandler(c, gate, handler), BaseContext: func(net.Listener) context.Context { return requests }}
	finished := make(chan error, 1)
	go func() { finished <- server.Serve(ln) }()
	var serveErr error
	select {
	case <-ctx.Done():
	case serveErr = <-finished:
		finished = nil
	}
	gate.stop()
	if owner != nil {
		owner.StopAdmission()
	}
	grace, stop := context.WithTimeout(context.Background(), c.Binary.ShutdownTimeout)
	shutdownErr := server.Shutdown(grace)
	stop()
	if shutdownErr != nil {
		logger.Warn("HTTP shutdown grace exceeded; cancelling requests and retaining ownership until drained")
		cancel()
		_ = server.Close()
	}
	gate.wait() // join handlers including detached compensation, never abandon I/O
	if owner != nil {
		_ = owner.WaitIdle(context.Background())
	}
	if finished != nil {
		serveErr = <-finished
	}
	if serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
		return errors.New("HTTP listener failed")
	}
	if shutdownErr != nil {
		return errors.New("HTTP shutdown required forced connection cancellation; handlers drained")
	}
	logger.Info("HTTP shutdown drained")
	return nil
}
