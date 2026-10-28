package mysql

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"
)

// Scripted SQL protocol double: asserts connection ownership and acknowledged
// statement ordering. It is not a SQL engine or proof of MySQL durability/DDL.
type sqlStep struct {
	query     string
	rows      [][]driver.Value
	exec      bool
	err       error
	rowsErr   error
	affected  int64
	checkArgs func([]driver.NamedValue)
}
type sessionScript struct {
	t      *testing.T
	steps  []sqlStep
	opened int
}
type scriptConnector struct{ script *sessionScript }

func (c scriptConnector) Connect(context.Context) (driver.Conn, error) {
	c.script.opened++
	return &scriptConn{c.script}, nil
}
func (c scriptConnector) Driver() driver.Driver { return scriptDriver{} }

type scriptDriver struct{}

func (scriptDriver) Open(string) (driver.Conn, error) { return nil, errors.New("use connector") }

type scriptConn struct{ s *sessionScript }

func (c *scriptConn) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("unexpected prepare")
}
func (c *scriptConn) Close() error { return nil }
func (c *scriptConn) Begin() (driver.Tx, error) {
	step := c.next("BEGIN", true)
	if step.err != nil {
		return nil, step.err
	}
	return &scriptTx{c}, nil
}

type scriptTx struct{ conn *scriptConn }

func (c *scriptConn) BeginTx(_ context.Context, _ driver.TxOptions) (driver.Tx, error) {
	return c.Begin()
}
func (t *scriptTx) Commit() error   { return t.conn.next("COMMIT", true).err }
func (t *scriptTx) Rollback() error { return t.conn.next("ROLLBACK", true).err }
func (c *scriptConn) next(q string, exec bool) sqlStep {
	c.s.t.Helper()
	if len(c.s.steps) == 0 {
		c.s.t.Errorf("unexpected SQL: %s", q)
		return sqlStep{err: errors.New("unexpected SQL")}
	}
	step := c.s.steps[0]
	c.s.steps = c.s.steps[1:]
	if strings.TrimSpace(step.query) != strings.TrimSpace(q) || step.exec != exec {
		c.s.t.Errorf("SQL ordering: got %q (exec=%v), want %q (exec=%v)", q, exec, step.query, step.exec)
		step.err = errors.New("unexpected SQL")
	}
	return step
}
func (c *scriptConn) QueryContext(_ context.Context, q string, args []driver.NamedValue) (driver.Rows, error) {
	step := c.next(q, false)
	if step.checkArgs != nil {
		step.checkArgs(args)
	}
	if step.err != nil {
		return nil, step.err
	}
	width := 1
	if len(step.rows) > 0 {
		width = len(step.rows[0])
	}
	return &scriptRows{data: step.rows, err: step.rowsErr, width: width}, nil
}
func (c *scriptConn) ExecContext(_ context.Context, q string, args []driver.NamedValue) (driver.Result, error) {
	step := c.next(q, true)
	if step.checkArgs != nil {
		step.checkArgs(args)
	}
	if strings.HasPrefix(q, "INSERT INTO schema_migrations") || strings.HasPrefix(q, "UPDATE schema_migrations") {
		if len(args) != 1 || args[0].Value != checksum() {
			c.s.t.Error("history checksum must be bound explicitly")
		}
	}
	return driver.RowsAffected(step.affected), step.err
}

type scriptRows struct {
	data  [][]driver.Value
	err   error
	width int
}

func (r *scriptRows) Columns() []string {
	cols := make([]string, r.width)
	for i := range cols {
		cols[i] = fmt.Sprint(i)
	}
	return cols
}
func (*scriptRows) Close() error { return nil }
func (r *scriptRows) Next(dest []driver.Value) error {
	if len(r.data) == 0 {
		if r.err != nil {
			return r.err
		}
		return io.EOF
	}
	copy(dest, r.data[0])
	r.data = r.data[1:]
	return nil
}
func query(q string, values ...driver.Value) sqlStep {
	return sqlStep{query: q, rows: [][]driver.Value{values}}
}
func execute(q string) sqlStep { return sqlStep{query: q, exec: true, affected: 1} }
func scriptDB(t *testing.T, steps []sqlStep) *sql.DB {
	t.Helper()
	s := &sessionScript{t: t, steps: steps}
	db := sql.OpenDB(scriptConnector{s})
	db.SetMaxOpenConns(4)
	t.Cleanup(func() {
		_ = db.Close()
		if len(s.steps) != 0 {
			t.Errorf("%d expected SQL operations not executed; next: %s", len(s.steps), s.steps[0].query)
		}
		if s.opened != 1 {
			t.Errorf("inspection and migration must use exactly one session, got %d", s.opened)
		}
	})
	return db
}
func accountSteps(literal bool, mandatory, active string, grants ...string) []sqlStep {
	steps := []sqlStep{query("SELECT DATABASE()", "glance_go"), query("SELECT @@GLOBAL.partial_revokes", literal), query("SELECT @@GLOBAL.mandatory_roles, CURRENT_ROLE()", mandatory, active)}
	data := [][]driver.Value{}
	for _, g := range grants {
		data = append(data, []driver.Value{g})
	}
	return append(steps, sqlStep{query: "SHOW GRANTS", rows: data})
}
func TestAccountEffectivePrivilegeBoundary(t *testing.T) {
	const safe = "GRANT SELECT, INSERT, UPDATE, DELETE ON `glance_go`.* TO `fixture`@`localhost`"
	for _, tc := range []struct {
		name              string
		literal           bool
		mandatory, active string
		grants            []string
		fail              bool
	}{
		{"limited direct grants", true, "", "NONE", []string{safe}, false},
		{"global privileges", true, "", "NONE", []string{safe, "GRANT SELECT ON *.* TO `fixture`@`localhost`"}, true},
		{"other schema", true, "", "NONE", []string{safe, "GRANT SELECT ON `glance`.* TO `fixture`@`localhost`"}, true},
		{"mandatory excessive role", true, "`powerful`@`%`", "NONE", nil, true},
		{"mandatory limited role unsupported", true, "`limited`@`%`", "NONE", nil, true},
		{"default active role", true, "", "`limited`@`%`", nil, true},
		{"inactive safe role not expanded", true, "", "NONE", []string{safe, "GRANT `limited`@`%` TO `fixture`@`localhost`"}, true},
		{"partial revokes off escaped", false, "", "NONE", []string{strings.ReplaceAll(safe, "glance_go", `glance\_go`)}, false},
		{"partial revokes off wildcard", false, "", "NONE", []string{safe}, true},
		{"partial revokes on escaped mismatch", true, "", "NONE", []string{strings.ReplaceAll(safe, "glance_go", `glance\_go`)}, true},
		{"percent wildcard", false, "", "NONE", []string{"GRANT SELECT ON `glance%`.* TO `fixture`@`localhost`"}, true},
		{"partial revoke statement", true, "", "NONE", []string{safe, "REVOKE SELECT ON `glance`.* FROM `fixture`@`localhost`"}, true},
		{"grant option", true, "", "NONE", []string{safe + " WITH GRANT OPTION"}, true},
		{"proxy", true, "", "NONE", []string{"GRANT PROXY ON `root`@`localhost` TO `fixture`@`localhost`"}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			steps := accountSteps(tc.literal, tc.mandatory, tc.active, tc.grants...)
			if tc.mandatory != "" || tc.active != "NONE" {
				steps = steps[:3]
			}
			err := CheckAccount(context.Background(), scriptDB(t, steps), "serve")
			if (err != nil) != tc.fail {
				t.Fatalf("rejection=%v want=%v", err, tc.fail)
			}
		})
	}
	for _, index := range []int{1, 2, 3} {
		t.Run(fmt.Sprintf("inspection unavailable %d", index), func(t *testing.T) {
			steps := accountSteps(true, "", "NONE", safe)[:index+1]
			steps[index].err = errors.New("permission unavailable")
			if CheckAccount(context.Background(), scriptDB(t, steps), "serve") == nil {
				t.Fatal("uninspected privileges accepted")
			}
		})
	}
}

const historyQuery = "SELECT version, checksum, dirty FROM schema_migrations ORDER BY version"

func migrationSteps(existing bool) []sqlStep {
	steps := accountSteps(true, "", "NONE", "GRANT SELECT, INSERT, UPDATE, CREATE, ALTER, INDEX, REFERENCES ON `glance_go`.* TO `fixture`@`localhost`")
	steps = append(steps, query("SELECT @@SESSION.autocommit", int64(1)), query("SELECT DATABASE()", "glance_go"), query("SELECT GET_LOCK(?, 0)", int64(1)))
	table := sqlStep{query: historyTableQuery}
	history := sqlStep{query: historyQuery}
	count := int64(0)
	if existing {
		table.rows = [][]driver.Value{{"BASE TABLE", "InnoDB"}}
		history.rows = [][]driver.Value{{int64(1), checksum(), false}}
		count = 1
	}
	steps = append(steps, table)
	if !existing {
		steps = append(steps, execute(historySQL))
	}
	steps = append(steps, historyStructureSteps()...)
	steps = append(steps, history, query("SELECT COUNT(*) FROM schema_migrations", count))
	if !existing {
		steps = append(steps, execute("INSERT INTO schema_migrations (version, checksum, dirty) VALUES (1, ?, TRUE)"))
		for _, q := range strings.Split(metadataSQL, ";") {
			if strings.TrimSpace(q) != "" {
				steps = append(steps, execute(q))
			}
		}
		steps = append(steps, execute("UPDATE schema_migrations SET dirty=FALSE, applied_at=UTC_TIMESTAMP(6) WHERE version=1 AND dirty=TRUE AND checksum=?"))
		steps = append(steps, historyStructureSteps()...)
		steps = append(steps, query(historyQuery, int64(1), checksum(), false))
	}
	return append(steps, query("SELECT RELEASE_LOCK(?)", int64(1)))
}

func sqlStepIndex(steps []sqlStep, q string, last bool) int {
	found := -1
	for i, s := range steps {
		if s.query == q {
			found = i
			if !last {
				return i
			}
		}
	}
	if found < 0 {
		panic("missing expected SQL fixture step")
	}
	return found
}

func failedMigrationStep(steps []sqlStep, index int, err error) []sqlStep {
	steps[index].err = err
	return append(steps[:index+1], steps[len(steps)-1])
}

func TestMigrationSessionAutocommit(t *testing.T) {
	for _, value := range []driver.Value{int64(0), int64(2), nil, "unexpected"} {
		t.Run(fmt.Sprint(value), func(t *testing.T) {
			steps := migrationSteps(false)[:5]
			steps[4] = query("SELECT @@SESSION.autocommit", value)
			if Migrate(context.Background(), scriptDB(t, steps)) == nil {
				t.Fatal("unsafe session was allowed to write")
			}
		})
	}
}
func TestMigrationAcknowledgementAndFailures(t *testing.T) {
	for _, existing := range []bool{false, true} {
		t.Run(fmt.Sprintf("success existing=%v", existing), func(t *testing.T) {
			if err := Migrate(context.Background(), scriptDB(t, migrationSteps(existing))); err != nil {
				t.Fatal(err)
			}
		})
	}
	ddl := ""
	for _, statement := range strings.Split(metadataSQL, ";") {
		if strings.TrimSpace(statement) != "" {
			ddl = statement
			break
		}
	}
	for _, tc := range []struct {
		name string
		edit func([]sqlStep) []sqlStep
	}{
		{"SQL DDL failure leaves dirty", func(s []sqlStep) []sqlStep {
			return failedMigrationStep(s, sqlStepIndex(s, ddl, false), errors.New("DDL failed"))
		}},
		{"interrupted after dirty marker", func(s []sqlStep) []sqlStep {
			return failedMigrationStep(s, sqlStepIndex(s, ddl, false), context.Canceled)
		}},
		{"dirty history", func(s []sqlStep) []sqlStep {
			i := sqlStepIndex(s, historyQuery, false)
			s[i].rows = [][]driver.Value{{int64(1), checksum(), true}}
			return append(s[:i+1], s[len(s)-1])
		}},
		{"checksum mismatch", func(s []sqlStep) []sqlStep {
			i := sqlStepIndex(s, historyQuery, false)
			s[i].rows = [][]driver.Value{{int64(1), strings.Repeat("0", 64), false}}
			return append(s[:i+1], s[len(s)-1])
		}},
		{"final DML failed", func(s []sqlStep) []sqlStep {
			return failedMigrationStep(s, sqlStepIndex(s, "UPDATE schema_migrations SET dirty=FALSE, applied_at=UTC_TIMESTAMP(6) WHERE version=1 AND dirty=TRUE AND checksum=?", false), errors.New("update not acknowledged"))
		}},
		{"final DML affected no history", func(s []sqlStep) []sqlStep {
			i := sqlStepIndex(s, "UPDATE schema_migrations SET dirty=FALSE, applied_at=UTC_TIMESTAMP(6) WHERE version=1 AND dirty=TRUE AND checksum=?", false)
			s[i].affected = 0
			return append(s[:i+1], s[len(s)-1])
		}},
		{"readback still dirty", func(s []sqlStep) []sqlStep {
			s[len(s)-2].rows = [][]driver.Value{{int64(1), checksum(), true}}
			return s
		}},
		{"readback unavailable", func(s []sqlStep) []sqlStep { s[len(s)-2].err = errors.New("connection interrupted"); return s }},
		{"lock not released", func(s []sqlStep) []sqlStep { s[len(s)-1].rows = [][]driver.Value{{int64(0)}}; return s }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if Migrate(context.Background(), scriptDB(t, tc.edit(migrationSteps(false)))) == nil {
				t.Fatal("incomplete migration reported success")
			}
		})
	}
}
