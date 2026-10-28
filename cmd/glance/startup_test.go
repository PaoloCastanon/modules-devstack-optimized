package main

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"reflect"
	"testing"
	"time"

	"github.com/PaoloCastanon/modules-devstack-optimized/internal/config"
	"github.com/PaoloCastanon/modules-devstack-optimized/internal/glance/model"
	"github.com/PaoloCastanon/modules-devstack-optimized/internal/glance/store"
	"github.com/google/uuid"
)

type startupRepo struct {
	binaryRepository
	err         error
	inventories int
}

func (r *startupRepo) ListDataOperations(context.Context, string, int) ([]model.DataOperation, error) {
	r.inventories++
	return nil, r.err
}
func testLogger() *slog.Logger { return slog.New(slog.NewJSONHandler(io.Discard, nil)) }
func testConfig() config.Config {
	return config.Config{RequestTimeout: time.Second, DependencyTimeout: time.Second, Binary: config.Binary{Enabled: true, MaxUploadBytes: 1024, MaxConcurrent: 2, UploadTimeout: 2 * time.Second, DownloadTimeout: 2 * time.Second, ResolutionTimeout: time.Second, RecoveryTimeout: time.Second, ShutdownTimeout: time.Second}}
}

func TestBackendChecksFailClosedInOrder(t *testing.T) {
	for _, fail := range []string{"", "ping", "account", "old schema", "dirty schema"} {
		t.Run(fail, func(t *testing.T) {
			var calls []string
			step := func(name string) func(context.Context) error {
				return func(context.Context) error {
					calls = append(calls, name)
					if fail == name || name == "schema" && (fail == "old schema" || fail == "dirty schema") {
						return errors.New("SECRET SQL diagnostics")
					}
					return nil
				}
			}
			checks := startupChecks{ping: step("ping"), account: step("account"), schema: step("schema")}
			err := verifyBackend(context.Background(), checks)
			want := []string{"ping", "account", "schema"}
			if fail == "ping" {
				want = want[:1]
			}
			if fail == "account" {
				want = want[:2]
			}
			if !reflect.DeepEqual(calls, want) || (err == nil) != (fail == "") {
				t.Fatal(calls, err)
			}
			if err != nil && err.Error() == "SECRET SQL diagnostics" {
				t.Fatal("raw error escaped")
			}
		})
	}
}

func TestBinaryStartupRequiresBindingOwnershipAndRecovery(t *testing.T) {
	for _, which := range []string{"success", "scope mismatch", "identity unavailable", "wrong root", "uninitialized", "permissions", "occupied", "recovery failure", "cancelled"} {
		t.Run(which, func(t *testing.T) {
			c := testConfig()
			c.Binary.Root = t.TempDir()
			c.Binary.Scope = uuid.NewString()
			if err := os.Chmod(c.Binary.Root, 0700); err != nil {
				t.Fatal(err)
			}
			if which != "uninitialized" {
				if err := store.Initialize(c.Binary.Root, c.Binary.Scope); err != nil {
					t.Fatal(err)
				}
			}
			r := &startupRepo{}
			scope := func(context.Context) (string, error) {
				if which == "scope mismatch" {
					return uuid.NewString(), nil
				}
				if which == "identity unavailable" {
					return "", errors.New("SECRET")
				}
				return c.Binary.Scope, nil
			}
			if which == "wrong root" {
				c.Binary.Root += "/absent"
			}
			if which == "permissions" {
				if err := os.Chmod(c.Binary.Root, 0755); err != nil {
					t.Fatal(err)
				}
			}
			if which == "occupied" {
				fs, err := store.NewFilesystem(c.Binary.Root, c.Binary.Scope)
				if err != nil {
					t.Fatal(err)
				}
				defer fs.Close()
			}
			if which == "recovery failure" {
				r.err = errors.New("SECRET")
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if which == "cancelled" {
				cancel()
			}
			b, err := prepareBinary(ctx, c, r, scope, testLogger())
			if which == "success" {
				if err != nil || r.inventories != 1 {
					t.Fatal(err, r.inventories)
				}
				if _, err := b.owner.RecoverUploads(ctx); err == nil {
					t.Fatal("recovery reran while ready")
				}
				if err := b.close(); err != nil {
					t.Fatal(err)
				}
				if err := b.close(); err != nil {
					t.Fatal(err)
				}
			} else {
				if err == nil || b != nil {
					t.Fatal("unsafe startup succeeded")
				}
			}
			if which == "recovery failure" || which == "success" || which == "cancelled" {
				fs, err := store.NewFilesystem(c.Binary.Root, c.Binary.Scope)
				if err != nil {
					t.Fatal("flock leaked", err)
				}
				if err = fs.Close(); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}

func TestStoreOwnershipAcrossProcesses(t *testing.T) {
	if os.Getenv("GLANCE_TEST_LOCK_CHILD") == "1" {
		fs, err := store.NewFilesystem(os.Getenv("GLANCE_TEST_LOCK_ROOT"), os.Getenv("GLANCE_TEST_LOCK_SCOPE"))
		if fs != nil {
			_ = fs.Close()
		}
		if !errors.Is(err, store.ErrBusy) {
			t.Fatal("second process acquired live store", err)
		}
		return
	}
	c := testConfig()
	c.Binary.Root = t.TempDir()
	c.Binary.Scope = uuid.NewString()
	if err := os.Chmod(c.Binary.Root, 0700); err != nil {
		t.Fatal(err)
	}
	if err := store.Initialize(c.Binary.Root, c.Binary.Scope); err != nil {
		t.Fatal(err)
	}
	b, err := prepareBinary(context.Background(), c, &startupRepo{}, func(context.Context) (string, error) { return c.Binary.Scope, nil }, testLogger())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := b.close(); err != nil {
			t.Error(err)
		}
	})
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, executable, "-test.run=^TestStoreOwnershipAcrossProcesses$")
	command.Env = append(os.Environ(), "GLANCE_TEST_LOCK_CHILD=1", "GLANCE_TEST_LOCK_ROOT="+c.Binary.Root, "GLANCE_TEST_LOCK_SCOPE="+c.Binary.Scope, "GOMAXPROCS=2")
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("child lock regression: %v %s", err, output)
	}
	if err = b.close(); err != nil {
		t.Fatal(err)
	}
	fs, err := store.NewFilesystem(c.Binary.Root, c.Binary.Scope)
	if err != nil {
		t.Fatal("ownership not released after drain", err)
	}
	if err = fs.Close(); err != nil {
		t.Fatal(err)
	}
}
