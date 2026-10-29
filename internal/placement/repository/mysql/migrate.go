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

//go:embed migrations/001_providers.sql
var providersSQL string

func checksum() string { return fmt.Sprintf("%x", sha256.Sum256([]byte(providersSQL))) }

type querier interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

func verifyHistory(ctx context.Context, q querier, allowEmpty bool) error {
	if err := verifyHistoryStructure(ctx, q); err != nil {
		return err
	}
	rows, err := q.QueryContext(ctx, "SELECT version, checksum, dirty FROM schema_migrations ORDER BY version")
	if err != nil {
		return errors.New("schema unavailable; run explicit migrations with the migration account")
	}
	defer rows.Close()
	count := 0
	for rows.Next() {
		var version int64
		var hash string
		var dirty bool
		if rows.Scan(&version, &hash, &dirty) != nil || version != 1 || hash != checksum() || dirty {
			return errors.New("unknown, changed or incomplete migration; manual review required")
		}
		count++
	}
	if rows.Err() != nil || count > 1 || (!allowEmpty && count != 1) {
		return errors.New("provider/inventory migration has not been verified")
	}
	return nil
}

func VerifySchema(ctx context.Context, db *sql.DB) error { return verifyHistory(ctx, db, false) }

// Migrate is called only by the explicit migrate command. MySQL DDL implicitly
// commits: dirty is persisted before DDL and never cleared after a failed step.
func Migrate(ctx context.Context, db *sql.DB) (result error) {
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
	lockName := fmt.Sprintf("placement_go:%x", sha256.Sum256([]byte(name)))[:63]
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
	if err = verifyHistory(ctx, conn, true); err != nil {
		return err
	}
	var count int
	if conn.QueryRowContext(ctx, "SELECT COUNT(*) FROM schema_migrations").Scan(&count) != nil {
		return errors.New("cannot inspect migration history")
	}
	if count == 1 {
		return nil
	}
	if _, err = conn.ExecContext(ctx, "INSERT INTO schema_migrations (version, checksum, dirty) VALUES (1, ?, TRUE)", checksum()); err != nil {
		return errors.New("cannot record pending migration")
	}
	for _, statement := range strings.Split(providersSQL, ";") {
		if strings.TrimSpace(statement) == "" {
			continue
		}
		if _, err = conn.ExecContext(ctx, statement); err != nil {
			return errors.New("provider/inventory DDL failed; migration remains dirty for manual review")
		}
	}
	completed, err := conn.ExecContext(ctx, "UPDATE schema_migrations SET dirty=FALSE, applied_at=UTC_TIMESTAMP(6) WHERE version=1 AND dirty=TRUE AND checksum=?", checksum())
	if err != nil {
		return errors.New("cannot complete migration history")
	}
	if affected, err := completed.RowsAffected(); err != nil || affected != 1 {
		return errors.New("migration history completion was not confirmed")
	}
	// Autocommit makes the UPDATE durable on successful acknowledgement.
	// Read it back before reporting success; DDL remains non-transactional.
	return verifyHistory(ctx, conn, false)
}
