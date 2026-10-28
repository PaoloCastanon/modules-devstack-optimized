package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/PaoloCastanon/modules-devstack-optimized/internal/auth"
	"github.com/PaoloCastanon/modules-devstack-optimized/internal/config"
	"github.com/PaoloCastanon/modules-devstack-optimized/internal/glance/api"
	"github.com/PaoloCastanon/modules-devstack-optimized/internal/glance/repository/mysql"
	"github.com/PaoloCastanon/modules-devstack-optimized/internal/glance/service"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stderr, nil))
	if err := run(os.Args[1:], logger); err != nil {
		logger.Error("glance stopped", "reason", err.Error())
		os.Exit(1)
	}
}

func run(args []string, logger *slog.Logger) error {
	if len(args) != 1 || (args[0] != "serve" && args[0] != "migrate") {
		return errors.New("usage: glance serve | glance migrate")
	}
	mode := args[0]
	c, err := config.Load(os.Getenv, mode)
	if err != nil {
		return err
	}
	db, err := mysql.Open(c.DatabaseDSN)
	if err != nil {
		return err
	}
	defer db.Close()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	startup, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if err = db.PingContext(startup); err != nil {
		return errors.New("database connection failed")
	}
	if mode == "migrate" {
		if err = mysql.Migrate(startup, db); err != nil {
			return err
		}
		logger.Info("metadata migration verified")
		return nil
	}
	if err = mysql.CheckAccount(startup, db, "serve"); err != nil {
		return err
	}
	if err = mysql.VerifySchema(startup, db); err != nil {
		return err
	}
	client := &http.Client{Timeout: c.DependencyTimeout}
	authenticator := auth.NewKeystone(c.KeystoneURL, client, c.Credential)
	handler, err := api.New(service.New(mysql.New(db, c.DependencyTimeout), time.Now), authenticator, c.PublicURL, c.KeystoneURL, c.MaxBodyBytes, logger)
	if err != nil {
		return err
	}
	server := &http.Server{Addr: c.ListenAddress, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: c.RequestTimeout,
		WriteTimeout: c.RequestTimeout + 5*time.Second, IdleTimeout: 30 * time.Second,
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			requestCtx, cancel := context.WithTimeout(r.Context(), c.RequestTimeout)
			defer cancel()
			handler.ServeHTTP(w, r.WithContext(requestCtx))
		})}
	closed := make(chan struct{})
	shutdownFinished := make(chan struct{})
	go func() {
		defer close(shutdownFinished)
		select {
		case <-ctx.Done():
			shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if server.Shutdown(shutdown) != nil {
				_ = server.Close()
			}
		case <-closed:
		}
	}()
	logger.Info("metadata API listening", "address", c.ListenAddress)
	err = server.ListenAndServe()
	close(closed)
	<-shutdownFinished
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		return errors.New("HTTP listener failed")
	}
	return nil
}
