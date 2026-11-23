//go:build integration

package mysql

import (
	"context"
	"database/sql"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/PaoloCastanon/modules-devstack-optimized/internal/config"
	driver "github.com/go-sql-driver/mysql"
)

// IntegrationFixture opens the disposable pair an integration suite runs on:
// the bookkeeping database named by GLANCE_GO_TEST_DSN_FILE
// (glance_go_shared_test_<suffix>, NEW EMPTY) and its paired copy of the
// original's catalog, glance_official_test_<suffix>, created by the runner
// from the original's DDL at the supported revision with no images. It
// migrates the bookkeeping and returns the open pool, its DSN and the
// official schema name to pass to New.
func IntegrationFixture(t *testing.T, ctx context.Context) (*sql.DB, string, string) {
	t.Helper()
	secret, err := config.ReadSecret(os.Getenv("GLANCE_GO_TEST_DSN_FILE"))
	if err != nil {
		t.Fatal(err)
	}
	dsn := strings.TrimSpace(string(secret))
	if err = ValidateTestDSN(dsn); err != nil {
		t.Fatal(err)
	}
	official := PairedOfficialSchema(t, dsn)
	db, err := Open(dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	var tables, images int
	if db.QueryRowContext(ctx, "SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=DATABASE()").Scan(&tables) != nil || tables != 0 {
		t.Fatal("integration suite requires a NEW EMPTY bookkeeping schema")
	}
	r, err := New(db, 0, official, "/opt/stack/data/glance/images")
	if err != nil {
		t.Fatal(err)
	}
	if err = r.VerifyOfficialCatalog(ctx); err != nil {
		t.Fatal("paired official catalog missing or at another revision:", err)
	}
	if db.QueryRowContext(ctx, "SELECT COUNT(*) FROM `"+official+"`.images").Scan(&images) != nil || images != 0 {
		t.Fatal("paired official catalog must have no images")
	}
	if err = Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	if err = Migrate(ctx, db); err != nil { // idempotent
		t.Fatal(err)
	}
	if err = VerifySchema(ctx, db); err != nil {
		t.Fatal(err)
	}
	return db, dsn, official
}

func IntegrationRepo(t *testing.T, db *sql.DB, timeout time.Duration, official string) *Repository {
	t.Helper()
	r, err := New(db, timeout, official, "/opt/stack/data/glance/images")
	if err != nil {
		t.Fatal(err)
	}
	return r
}

// PairedOfficialSchema is the copy of the original's catalog paired with a
// glance_go_shared_test_<suffix> DSN: glance_official_test_<suffix>.
func PairedOfficialSchema(t *testing.T, dsn string) string {
	t.Helper()
	c, err := driver.ParseDSN(dsn)
	if err != nil || ValidateTestDSN(dsn) != nil {
		t.Fatal("unsafe test DSN")
	}
	return "glance_official_test_" + strings.TrimPrefix(c.DBName, "glance_go_shared_test_")
}
