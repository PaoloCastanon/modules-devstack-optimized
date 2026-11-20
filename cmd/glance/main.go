package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
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

func run(args []string, logger *slog.Logger) (result error) {
	started := time.Now()
	if len(args) != 1 || (args[0] != "serve" && args[0] != "migrate" && args[0] != "store-scope" && args[0] != "store-init") {
		return errors.New("usage: glance serve | migrate | store-scope | store-init")
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
	defer func() {
		if db.Close() != nil {
			result = errors.Join(result, errors.New("database pool close failed"))
		}
	}()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	startup, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if err = db.PingContext(startup); err != nil {
		return errors.New("database connection failed")
	}
	official := c.OfficialCatalog != ""
	var database string
	if db.QueryRowContext(startup, "SELECT DATABASE()").Scan(&database) != nil || mysql.Shared(database) != official {
		return errors.New("GLANCE_GO_CATALOG=official requires the glance_go_shared database, and only it")
	}
	if mode == "migrate" {
		migrate := mysql.Migrate
		if official {
			migrate = mysql.MigrateShared
		}
		if err = migrate(startup, db); err != nil {
			return err
		}
		logger.Info("metadata migration verified")
		return nil
	}
	checks := startupChecks{ping: db.PingContext, account: func(ctx context.Context) error { return mysql.CheckAccount(ctx, db, "serve") }, schema: func(ctx context.Context) error {
		if official {
			return mysql.VerifySharedSchema(ctx, db)
		}
		return mysql.VerifySchema(ctx, db)
	}, scope: func(ctx context.Context) (string, error) { return databaseScope(ctx, db) }}
	if err = verifyBackend(startup, checks); err != nil {
		return err
	}
	if mode == "store-scope" || mode == "store-init" {
		scope, err := checks.scope(startup)
		if err != nil {
			return err
		}
		if mode == "store-scope" {
			_, err = fmt.Fprintln(os.Stdout, scope)
			return err
		}
		if scope != c.Binary.Scope {
			return errors.New("SQL/store scope mismatch")
		}
		if startup.Err() != nil {
			return errors.New("store initialization cancelled")
		}
		if err = initStore(c.Binary, scope); err != nil {
			return errors.New("store initialization failed; inspect partial setup without automatic repair")
		}
		logger.Info("store explicitly initialized")
		return nil
	}
	r := mysql.New(db, c.DependencyTimeout)
	if official {
		if err = r.UseOfficialCatalog(c.OfficialCatalog, c.Binary.OfficialStoreDir); err != nil {
			return err
		}
		if err = r.VerifyOfficialCatalog(startup); err != nil {
			return err
		}
		logger.Info("serving the official catalog", "database", c.OfficialCatalog, "store", c.Binary.OfficialStoreDir)
	}
	if err = r.PrepareStatements(startup); err != nil {
		// Correctness does not depend on it: every query still runs unprepared.
		logger.Warn("hot statements not prepared; continuing without reuse")
	}
	notifier, closeNotifier, err := notifierFor(c.Notifications, logger)
	if err != nil {
		return err
	}
	defer func() { // after HTTP and transfers stopped: deliver what is queued
		drain, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if closeNotifier(drain) != nil {
			logger.Warn("notifications not fully delivered before exit")
		}
	}()
	var binary *binaryRuntime
	var options []api.Option
	if c.Binary.Enabled {
		// Account/schema checks have their own 30s budget; recovery has its
		// configured finite budget and may hash large confirmed objects.
		binary, err = prepareBinary(ctx, c, r, checks.scope, logger)
		if err != nil {
			return err
		}
		defer func() {
			if binary.close() != nil {
				result = errors.Join(result, errors.New("store close failed"))
			}
		}()
		binary.owner.SetNotifier(notifier)
		options = append(options, api.WithBinaryTransfers(binary.owner))
	}
	// Reuse Keystone connections under concurrency (net/http keeps only 2 idle
	// per host by default; keystoneauth's requests pool keeps 10).
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.MaxIdleConnsPerHost = 32
	client := &http.Client{Timeout: c.DependencyTimeout, Transport: transport}
	authenticator := auth.NewKeystone(c.KeystoneURL, client, c.Credential, auth.WithTokenCache(c.TokenCacheTime, 10000))
	catalog := service.New(r, time.Now)
	catalog.SetNotifier(notifier)
	handler, err := api.New(catalog, authenticator, c.PublicURL, c.KeystoneURL, c.MaxBodyBytes, logger, options...)
	if err != nil {
		return err
	}
	if err = ctx.Err(); err != nil {
		return errors.New("startup cancelled before listener")
	}
	ln, err := net.Listen("tcp", c.ListenAddress)
	if err != nil {
		return errors.New("HTTP listener failed")
	}
	defer ln.Close()
	logger.Info("effective settings", effectiveSettings(c)...)
	logger.Info("image API listening", "address", c.ListenAddress, "binary_enabled", c.Binary.Enabled, "startup_ms", time.Since(started).Milliseconds())
	var owner *service.UploadCoordinator
	if binary != nil {
		owner = binary.owner
	}
	return serveHTTP(ctx, ln, c, handler, owner, logger)
}
