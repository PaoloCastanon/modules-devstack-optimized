package mysql

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"database/sql/driver"
	_ "embed"
	"errors"
	"fmt"
	"strings"
	"time"
)

//go:embed migrations/000_history.sql
var historySQL string

//go:embed migrations/001_metadata.sql
var metadataSQL string

type migration struct {
	version int64
	sql     string
}

// Append new immutable migrations here only when their SQL is approved. History
// initialization (000) is not a metadata schema version; no version 2 exists yet.
func migrations() []migration { return []migration{{version: 1, sql: metadataSQL}} }

func (m migration) checksum() string { return fmt.Sprintf("%x", sha256.Sum256([]byte(m.sql))) }
func checksum() string               { return migrations()[0].checksum() }

func validateRegistry(registry []migration) error {
	if len(registry) == 0 {
		return errors.New("migration registry is empty")
	}
	for n, m := range registry {
		if m.version != int64(n+1) || strings.TrimSpace(m.sql) == "" {
			return errors.New("migration registry must contain contiguous ordered versions with SQL")
		}
	}
	return nil
}

type querier interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

// readHistory accepts only an exact clean prefix of the available registry.
// Runtime requires the complete prefix; only the explicit migrator may extend it.
func readHistory(ctx context.Context, q querier, registry []migration, allowPrefix bool) (int, error) {
	if err := validateRegistry(registry); err != nil {
		return 0, err
	}
	if err := verifyHistoryStructure(ctx, q); err != nil {
		return 0, err
	}
	rows, err := q.QueryContext(ctx, "SELECT version, checksum, dirty FROM schema_migrations ORDER BY version")
	if err != nil {
		return 0, errors.New("schema unavailable; run explicit migrations with the migration account")
	}
	defer rows.Close()
	count := 0
	for rows.Next() {
		var version int64
		var hash string
		var dirty bool
		if rows.Scan(&version, &hash, &dirty) != nil || count >= len(registry) ||
			version != registry[count].version || hash != registry[count].checksum() || dirty {
			return 0, errors.New("unknown, changed or incomplete migration; manual review required")
		}
		count++
	}
	if rows.Err() != nil || (!allowPrefix && count != len(registry)) {
		return 0, errors.New("metadata migration has not been verified")
	}
	if rows.Close() != nil {
		return 0, errors.New("cannot finish migration history inspection")
	}
	return count, nil
}

func verifyHistory(ctx context.Context, q querier, allowPrefix bool) error {
	_, err := readHistory(ctx, q, migrations(), allowPrefix)
	return err
}

func VerifySchema(ctx context.Context, db *sql.DB) error { return verifyHistory(ctx, db, false) }

// Migrate is called only by the explicit migrate command. MySQL DDL implicitly
// commits: dirty is persisted before DDL and never cleared after a failed step.
func Migrate(ctx context.Context, db *sql.DB) error {
	return migrateVersions(ctx, db, migrations())
}

func migrateVersions(ctx context.Context, db *sql.DB, registry []migration) (result error) {
	if err := validateRegistry(registry); err != nil {
		return err
	}
	conn, err := db.Conn(ctx)
	if err != nil {
		return errors.New("cannot acquire migration connection")
	}
	defer conn.Close()
	if err := checkAccount(ctx, conn, "migrate"); err != nil {
		return err
	}
	// Check the actual pinned session, not only the DSN. Every history DML
	// statement must be committed before subsequent DDL or a success result.
	var autocommit int
	if conn.QueryRowContext(ctx, "SELECT @@SESSION.autocommit").Scan(&autocommit) != nil || autocommit != 1 {
		return errors.New("migration session must have autocommit enabled")
	}
	var name string
	if conn.QueryRowContext(ctx, "SELECT DATABASE()").Scan(&name) != nil || !AllowedDatabase(name) {
		return errors.New("unsafe migration database")
	}
	var locked sql.NullInt64
	lockName := fmt.Sprintf("glance_go:%x", sha256.Sum256([]byte(name)))[:63]
	if conn.QueryRowContext(ctx, "SELECT GET_LOCK(?, 0)", lockName).Scan(&locked) != nil || !locked.Valid || locked.Int64 != 1 {
		return errors.New("migration already running or lock unavailable")
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		var released sql.NullInt64
		if conn.QueryRowContext(cleanup, "SELECT RELEASE_LOCK(?)", lockName).Scan(&released) != nil || !released.Valid || released.Int64 != 1 {
			// A pooled connection must never retain the advisory lock.
			_ = conn.Raw(func(any) error { return driver.ErrBadConn })
			if result == nil {
				result = errors.New("cannot confirm migration lock release")
			}
		}
	}()
	exists, err := historyTable(ctx, conn)
	if err != nil {
		return err
	}
	if !exists {
		if _, err = conn.ExecContext(ctx, historySQL); err != nil {
			return errors.New("cannot initialize migration history")
		}
	}
	applied, err := readHistory(ctx, conn, registry, true)
	if err != nil {
		return err
	}
	var count int
	if conn.QueryRowContext(ctx, "SELECT COUNT(*) FROM schema_migrations").Scan(&count) != nil || count != applied {
		return errors.New("cannot inspect migration history")
	}
	for _, m := range registry[applied:] {
		// Versions are validated positive registry integers, never client input.
		// Keep the historical version-1 SQL and checksum binding unchanged.
		pending := fmt.Sprintf("INSERT INTO schema_migrations (version, checksum, dirty) VALUES (%d, ?, TRUE)", m.version)
		if _, err = conn.ExecContext(ctx, pending, m.checksum()); err != nil {
			return errors.New("cannot record pending migration")
		}
		for _, statement := range strings.Split(m.sql, ";") {
			if strings.TrimSpace(statement) == "" {
				continue
			}
			if _, err = conn.ExecContext(ctx, statement); err != nil {
				return errors.New("metadata DDL failed; migration remains dirty for manual review")
			}
		}
		finish := fmt.Sprintf("UPDATE schema_migrations SET dirty=FALSE, applied_at=UTC_TIMESTAMP(6) WHERE version=%d AND dirty=TRUE AND checksum=?", m.version)
		completed, err := conn.ExecContext(ctx, finish, m.checksum())
		if err != nil {
			return errors.New("cannot complete migration history")
		}
		if affected, err := completed.RowsAffected(); err != nil || affected != 1 {
			return errors.New("migration history completion was not confirmed")
		}
		// Autocommit acknowledges each history DML; verify the exact applied
		// prefix before attempting later DDL. No DDL rollback is promised.
		if _, err = readHistory(ctx, conn, registry[:m.version], false); err != nil {
			return err
		}
	}
	return nil
}
