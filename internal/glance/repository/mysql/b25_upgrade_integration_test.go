//go:build integration

package mysql

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/PaoloCastanon/modules-devstack-optimized/internal/config"
	"github.com/PaoloCastanon/modules-devstack-optimized/internal/glance/model"
)

// MUTATING, NEW EMPTY independent schema, explicit permission required. Uses
// immutable registry[:1] to construct v1, then ONLY Migrate appends v2. Deliberate
// history faults are confined here; this schema must NEVER be used by API tests.
func TestB25MySQLVersionedUpgrade(t *testing.T) {
	if os.Getenv("GLANCE_GO_B25_MYSQL_TEST") != "1" {
		t.Fatal("explicit disposable B25 MySQL opt-in required")
	}
	b, err := config.ReadSecret(os.Getenv("GLANCE_GO_TEST_DSN_FILE"))
	if err != nil {
		t.Fatal(err)
	}
	dsn := strings.TrimSpace(string(b))
	if err = ValidateTestDSN(dsn); err != nil {
		t.Fatal(err)
	}
	db, err := Open(dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	if err = CheckAccount(ctx, db, "migrate"); err != nil {
		t.Fatal(err)
	}
	var count int
	if db.QueryRowContext(ctx, "SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=DATABASE()").Scan(&count) != nil || count != 0 {
		t.Fatal("NEW EMPTY schema required, separate from ALL other suites")
	}
	if err = migrateVersions(ctx, db, migrations()[:1]); err != nil {
		t.Fatal(err)
	}
	if err = VerifySchema(ctx, db); err == nil {
		t.Fatal("runtime accepted old schema")
	}
	r := New(db, 5*time.Second)
	i, err := model.New(model.CreateInput{Properties: map[string]string{"preserve": "version-one"}, Tags: []string{"v1"}}, "upgrade-project", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err = r.Create(ctx, i); err != nil {
		t.Fatal(err)
	}
	var before string
	if db.QueryRowContext(ctx, "SELECT checksum FROM schema_migrations WHERE version=1").Scan(&before) != nil {
		t.Fatal("v1 history absent")
	}
	if err = Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	if err = VerifySchema(ctx, db); err != nil {
		t.Fatal(err)
	}
	if err = Migrate(ctx, db); err != nil {
		t.Fatal("idempotence", err)
	}
	var after string
	if db.QueryRowContext(ctx, "SELECT checksum FROM schema_migrations WHERE version=1").Scan(&after) != nil || before != after || after != migrations()[0].checksum() {
		t.Fatal("v1 checksum changed")
	}
	got, err := r.Get(ctx, i.ID)
	if err != nil || got.Properties["preserve"] != "version-one" || len(got.Tags) != 1 || got.Status != "queued" {
		t.Fatal("v1 metadata lost")
	}
	// Exact live table/key/CHECK inspection is part of VerifySchema. Record
	// actual engine and history row count, not just the migration return value.
	var engine, version string
	if db.QueryRowContext(ctx, "SELECT ENGINE FROM information_schema.tables WHERE table_schema=DATABASE() AND table_name='image_data_operations'").Scan(&engine) != nil || engine != "InnoDB" {
		t.Fatal("wrong engine")
	}
	if db.QueryRowContext(ctx, "SELECT VERSION()").Scan(&version) != nil {
		t.Fatal("server version unavailable")
	}
	if db.QueryRowContext(ctx, "SELECT COUNT(*) FROM schema_migrations WHERE dirty=FALSE").Scan(&count) != nil || count != 2 {
		t.Fatal("incomplete history")
	}
	t.Log("MySQL effective version:", version, "engine:", engine, "v1 checksum:", after)
	if _, err = db.ExecContext(ctx, "UPDATE schema_migrations SET checksum=? WHERE version=2", strings.Repeat("0", 64)); err != nil {
		t.Fatal(err)
	}
	if VerifySchema(ctx, db) == nil || Migrate(ctx, db) == nil {
		t.Fatal("modified checksum accepted")
	}
	if _, err = db.ExecContext(ctx, "UPDATE schema_migrations SET checksum=?, dirty=TRUE WHERE version=2", migrations()[1].checksum()); err != nil {
		t.Fatal(err)
	}
	if VerifySchema(ctx, db) == nil || Migrate(ctx, db) == nil {
		t.Fatal("dirty history accepted")
	}
	t.Log("Schema intentionally retained DIRTY for review, never reuse it for normal API tests; no DDL rollback claimed")
}
