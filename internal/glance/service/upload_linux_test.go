//go:build linux

package service

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/PaoloCastanon/modules-devstack-optimized/internal/glance/store"
	"github.com/google/uuid"
)

// Only ownership is checked against a temporary real filesystem here. There is
// no MySQL/Keystone or physical durability claim; upload/fault cases use doubles.
func TestProductionCoordinatorClaimsExclusiveFilesystem(t *testing.T) {
	temp := t.TempDir()
	if err := os.Chmod(temp, 0700); err != nil {
		t.Fatal(err)
	}
	scope := uuid.NewString()
	if err := store.Initialize(temp, scope); err != nil {
		t.Fatal(err)
	}
	fs, err := store.NewFilesystem(temp, scope)
	if err != nil {
		t.Fatal(err)
	}
	defer fs.Close()
	_, d, _, _ := uploadFixture(t)
	u, err := NewUploadCoordinator(uploadCatalogDouble{sql: d}, d, fs, UploadLimits{MaxBytes: 100, MaxConcurrent: 1, ResolutionTimeout: time.Second, RecoveryTimeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	defer u.Close()
	if _, err = NewUploadCoordinator(uploadCatalogDouble{sql: d}, d, fs, u.limits); !errors.Is(err, store.ErrBusy) {
		t.Fatal("duplicate owner accepted", err)
	}
	if _, err = store.NewFilesystem(temp, scope); !errors.Is(err, store.ErrBusy) {
		t.Fatal("second root writer accepted", err)
	}
	if _, err = u.RecoverUploads(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err = u.Close(); err != nil {
		t.Fatal(err)
	}
	next, err := NewUploadCoordinator(uploadCatalogDouble{sql: d}, d, fs, u.limits)
	if err != nil {
		t.Fatal("ownership not released", err)
	}
	defer next.Close()
}
