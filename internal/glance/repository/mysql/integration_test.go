//go:build integration

package mysql_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/PaoloCastanon/modules-devstack-optimized/internal/config"
	"github.com/PaoloCastanon/modules-devstack-optimized/internal/glance/model"
	"github.com/PaoloCastanon/modules-devstack-optimized/internal/glance/repository"
	"github.com/PaoloCastanon/modules-devstack-optimized/internal/glance/repository/mysql"
)

// This suite MUTATES an explicitly supplied empty, disposable database. Missing
// opt-in is a failure, never a silently passing or skipped integration run.
func TestMySQLPersistenceAndTransactions(t *testing.T) {
	if os.Getenv("GLANCE_GO_MYSQL_TEST") != "1" {
		t.Fatal("set GLANCE_GO_MYSQL_TEST=1 only for an independently authorized disposable database")
	}
	secret, err := config.ReadSecret(os.Getenv("GLANCE_GO_TEST_DSN_FILE"))
	if err != nil {
		t.Fatal(err)
	}
	dsn := strings.TrimSpace(string(secret))
	if err = mysql.ValidateTestDSN(dsn); err != nil {
		t.Fatal(err)
	}
	db, err := mysql.Open(dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if db != nil {
			_ = db.Close()
		}
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if db.PingContext(ctx) != nil {
		t.Fatal("isolated MySQL unavailable")
	}
	if err = mysql.CheckAccount(ctx, db, "migrate"); err != nil {
		t.Fatal(err)
	}
	var tables int
	if err = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=DATABASE()").Scan(&tables); err != nil || tables != 0 {
		t.Fatal("integration database must be empty before this suite")
	}
	if err = mysql.VerifySchema(ctx, db); err == nil {
		t.Fatal("accepted an unmigrated schema")
	}
	// Pin the isolated pool to prove the runtime session guard, independently
	// of the DSN parser. No DDL may run with autocommit disabled.
	db.SetMaxOpenConns(1)
	if _, err = db.ExecContext(ctx, "SET SESSION autocommit=0"); err != nil {
		t.Fatal("cannot prepare autocommit fixture")
	}
	if mysql.Migrate(ctx, db) == nil {
		t.Fatal("migration accepted autocommit=0")
	}
	if err = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=DATABASE()").Scan(&tables); err != nil || tables != 0 {
		t.Fatal("unsafe session performed DDL")
	}
	if _, err = db.ExecContext(ctx, "SET SESSION autocommit=1"); err != nil {
		t.Fatal("cannot restore isolated session")
	}
	if err = mysql.Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	if err = mysql.Migrate(ctx, db); err != nil {
		t.Fatal("idempotent migration failed:", err)
	}
	if err = mysql.VerifySchema(ctx, db); err != nil {
		t.Fatal(err)
	}
	// A fresh pool must observe the committed final history, before any later
	// DDL could implicitly commit it and accidentally hide an autocommit bug.
	if err = db.Close(); err != nil {
		t.Fatal("cannot close migration pool")
	}
	db, err = mysql.Open(dsn)
	if err != nil {
		t.Fatal(err)
	}
	if err = mysql.VerifySchema(ctx, db); err != nil {
		t.Fatal("migration history was not durable across connections")
	}
	// The same account has CREATE privileges and must not be accepted by serve.
	if err = mysql.CheckAccount(ctx, db, "serve"); err == nil {
		t.Fatal("serve accepted the migration account")
	}
	now := time.Now().UTC().Truncate(time.Microsecond)
	name := ""
	image, err := model.New(model.CreateInput{Name: &name, Tags: []string{"a", "b"}, Properties: map[string]string{"description": "fixture", "owner_specified.openstack.object": ""}}, "integration-project", now)
	if err != nil {
		t.Fatal(err)
	}
	repo := mysql.New(db, 3*time.Second)
	if err = repo.Create(ctx, image); err != nil {
		t.Fatal("create failed")
	}
	stored, err := repo.Get(ctx, image.ID)
	if err != nil || stored.Name == nil || *stored.Name != "" || stored.DiskFormat != nil || stored.Size != nil || stored.Visibility != "shared" || len(stored.Tags) != 2 || stored.Properties["description"] != "fixture" || !stored.CreatedAt.Equal(now) {
		t.Fatal("metadata roundtrip failed")
	}
	if err = repo.Create(ctx, image); !errors.Is(err, repository.ErrDuplicate) {
		t.Fatal("duplicate UUID not mapped")
	}
	// Force a child INSERT failure after the image INSERT. The whole transaction
	// must disappear, not just the invalid property (strict per-session SQL mode).
	broken, err := model.New(model.CreateInput{}, "integration-project", now)
	if err != nil {
		t.Fatal(err)
	}
	broken.Properties["too_large"] = strings.Repeat("x", 65536)
	if err = repo.Create(ctx, broken); err == nil {
		t.Fatal("expected child SQL failure")
	}
	if _, err = repo.Get(ctx, broken.ID); !errors.Is(err, repository.ErrNotFound) {
		t.Fatal("failed transaction left an image")
	}
	cancelled, stop := context.WithCancel(ctx)
	stop()
	if err = repo.Create(cancelled, broken); err == nil {
		t.Fatal("cancelled operation succeeded")
	}
	// Closing and reopening the SQL pool proves the data outlives its connection.
	if err = db.Close(); err != nil {
		t.Fatal("close pool failed")
	}
	db, err = mysql.Open(dsn)
	if err != nil {
		t.Fatal(err)
	}
	repo = mysql.New(db, 3*time.Second)
	stored, err = repo.Get(ctx, image.ID)
	if err != nil || stored.ID != image.ID || len(stored.Tags) != 2 {
		t.Fatal("persistence across connections failed")
	}
	if _, err = db.ExecContext(ctx, "UPDATE images SET deleted=TRUE, deleted_at=UTC_TIMESTAMP(6) WHERE id=?", image.ID); err != nil {
		t.Fatal("soft delete fixture failed")
	}
	if _, err = repo.Get(ctx, image.ID); !errors.Is(err, repository.ErrNotFound) {
		t.Fatal("deleted record visible")
	}
	if err = repo.Create(ctx, image); !errors.Is(err, repository.ErrDuplicate) {
		t.Fatal("deleted UUID was reused")
	}
	var originalChecksum string
	if db.QueryRowContext(ctx, "SELECT checksum FROM schema_migrations WHERE version=1").Scan(&originalChecksum) != nil {
		t.Fatal("cannot inspect migration fixture")
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if _, err := db.ExecContext(cleanup, "UPDATE schema_migrations SET dirty=FALSE, checksum=? WHERE version=1", originalChecksum); err != nil {
			t.Error("cannot restore isolated migration fixture")
		}
	}()
	if _, err = db.ExecContext(ctx, "UPDATE schema_migrations SET dirty=TRUE WHERE version=1"); err != nil {
		t.Fatal("dirty fixture failed")
	}
	if mysql.VerifySchema(ctx, db) == nil || mysql.Migrate(ctx, db) == nil {
		t.Fatal("dirty migration accepted")
	}
	if _, err = db.ExecContext(ctx, "UPDATE schema_migrations SET dirty=FALSE, checksum=REPEAT('0',64) WHERE version=1"); err != nil {
		t.Fatal("checksum fixture failed")
	}
	if mysql.VerifySchema(ctx, db) == nil || mysql.Migrate(ctx, db) == nil {
		t.Fatal("changed migration accepted")
	}
	// Leave fixture records in this disposable schema for operator review.
	// No database, user, table or preexisting data is automatically dropped.
}

// Independently provisioned fixture servers/accounts only. This test reads
// privileges; it never creates roles or changes global server configuration.
func TestMySQLAccountBoundaryFixtures(t *testing.T) {
	if os.Getenv("GLANCE_GO_MYSQL_TEST") != "1" {
		t.Fatal("explicit isolated MySQL opt-in required")
	}
	data, err := config.ReadSecret(os.Getenv("GLANCE_GO_TEST_ACCOUNTS_FILE"))
	if err != nil {
		t.Fatal(err)
	}
	var fixtures []struct {
		Name           string `json:"name"`
		DSNFile        string `json:"dsn_file"`
		Reject         bool   `json:"reject"`
		PartialRevokes bool   `json:"partial_revokes"`
		MandatoryRoles string `json:"mandatory_roles"`
		ActiveRoles    string `json:"active_roles"`
	}
	if json.Unmarshal(data, &fixtures) != nil || len(fixtures) == 0 {
		t.Fatal("nonempty private account fixture manifest required")
	}
	for i, f := range fixtures {
		// Index-only subtest names avoid leaking account metadata from a private file.
		t.Run(fmt.Sprintf("fixture-%d", i), func(t *testing.T) {
			secret, err := config.ReadSecret(f.DSNFile)
			if err != nil {
				t.Fatal(err)
			}
			dsn := strings.TrimSpace(string(secret))
			if mysql.ValidateTestDSN(dsn) != nil {
				t.Fatal("unsafe account fixture DSN")
			}
			db, err := mysql.Open(dsn)
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			db.SetMaxOpenConns(1)
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			var partial bool
			var mandatory, active string
			if db.QueryRowContext(ctx, "SELECT @@GLOBAL.partial_revokes, @@GLOBAL.mandatory_roles, CURRENT_ROLE()").Scan(&partial, &mandatory, &active) != nil {
				t.Fatal("cannot verify fixture preconditions")
			}
			if partial != f.PartialRevokes || mandatory != f.MandatoryRoles || active != f.ActiveRoles {
				t.Fatal("fixture server state differs from manifest")
			}
			if rejected := mysql.CheckAccount(ctx, db, "serve") != nil; rejected != f.Reject {
				t.Fatal("account boundary differs from expected policy")
			}
		})
	}
}

// Reproduces partial DDL on a separate empty disposable schema. It intentionally
// leaves a colliding fixture table and dirty history for manual review.
func TestMySQLPartialDDLRemainsDirtyAcrossConnections(t *testing.T) {
	if os.Getenv("GLANCE_GO_MYSQL_TEST") != "1" {
		t.Fatal("explicit isolated MySQL opt-in required")
	}
	secret, err := config.ReadSecret(os.Getenv("GLANCE_GO_TEST_PARTIAL_DSN_FILE"))
	if err != nil {
		t.Fatal(err)
	}
	dsn := strings.TrimSpace(string(secret))
	if mysql.ValidateTestDSN(dsn) != nil {
		t.Fatal("unsafe partial migration fixture DSN")
	}
	db, err := mysql.Open(dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if db != nil {
			_ = db.Close()
		}
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err = mysql.CheckAccount(ctx, db, "migrate"); err != nil {
		t.Fatal(err)
	}
	var tables int
	if db.QueryRowContext(ctx, "SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=DATABASE()").Scan(&tables) != nil || tables != 0 {
		t.Fatal("partial migration fixture requires an empty schema")
	}
	if _, err = db.ExecContext(ctx, "CREATE TABLE image_tags (fixture INT) ENGINE=InnoDB"); err != nil {
		t.Fatal("cannot create isolated collision fixture")
	}
	if mysql.Migrate(ctx, db) == nil {
		t.Fatal("partial DDL failure reported success")
	}
	if err = db.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = mysql.Open(dsn)
	if err != nil {
		t.Fatal(err)
	}
	var dirty bool
	if db.QueryRowContext(ctx, "SELECT dirty FROM schema_migrations WHERE version=1").Scan(&dirty) != nil || !dirty {
		t.Fatal("partial DDL did not leave durable dirty history")
	}
	if mysql.VerifySchema(ctx, db) == nil || mysql.Migrate(ctx, db) == nil {
		t.Fatal("partial migration accepted on reopen")
	}
}

// Read-only checkpoint for a future operator-controlled restart experiment.
// The operator must separately record that only the disposable server restarted;
// this test never starts/stops a server and cannot itself prove a restart occurred.
func TestMySQLHistoryReadOnlyVerification(t *testing.T) {
	if os.Getenv("GLANCE_GO_MYSQL_TEST") != "1" {
		t.Fatal("explicit isolated MySQL opt-in required")
	}
	secret, err := config.ReadSecret(os.Getenv("GLANCE_GO_TEST_HISTORY_DSN_FILE"))
	if err != nil {
		t.Fatal(err)
	}
	dsn := strings.TrimSpace(string(secret))
	if mysql.ValidateTestDSN(dsn) != nil {
		t.Fatal("unsafe history verification DSN")
	}
	expectedUUID := strings.TrimSpace(os.Getenv("GLANCE_GO_TEST_HISTORY_SERVER_UUID"))
	if expectedUUID == "" {
		t.Fatal("independently recorded disposable server UUID required")
	}
	db, err := mysql.Open(dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var name, serverUUID string
	if db.QueryRowContext(ctx, "SELECT DATABASE(), @@GLOBAL.server_uuid").Scan(&name, &serverUUID) != nil {
		t.Fatal("cannot identify history verification database")
	}
	if !strings.HasPrefix(name, "glance_go_test_history_") || serverUUID != expectedUUID {
		t.Fatal("history verification target differs from the authorized fixture")
	}
	if err = mysql.CheckAccount(ctx, db, "serve"); err != nil {
		t.Fatal(err)
	}
	if err = mysql.VerifySchema(ctx, db); err != nil {
		t.Fatal(err)
	}
}
