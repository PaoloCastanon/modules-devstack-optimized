package mysql

import (
	"context"
	"database/sql/driver"
	"errors"
	"strings"
	"testing"
)

func TestAvailableMigrationRegistry(t *testing.T) {
	r := migrations()
	// Same SQL and checksum as the glance_go_shared databases already
	// installed beside the original catalog: they verify without migrating.
	if len(r) != 1 || r[0].version != 1 || r[0].sql != dataOperationsSQL || r[0].verify == nil || r[0].checksum() != "3445effd9f3236f0adf513ef95b0cb8a92de744f12049ce44d063c1536527931" {
		t.Fatal("invalid append-only registry")
	}
	for _, r := range [][]migration{nil, {{version: 2, sql: "SELECT 1"}}, {{version: 1, sql: "SELECT 1"}, {version: 1, sql: "SELECT 1"}}, {{version: 1, sql: "SELECT 1"}, {version: 3, sql: "SELECT 1"}}, {{version: 1, sql: " "}}} {
		if validateRegistry(r) == nil {
			t.Fatal("invalid registry accepted")
		}
	}
	// The approved DDL is exactly one semicolon-delimited statement.
	var statements []string
	for _, sql := range strings.Split(dataOperationsSQL, ";") {
		if strings.TrimSpace(sql) != "" {
			statements = append(statements, sql)
		}
	}
	if len(statements) != 1 || !strings.HasPrefix(strings.TrimSpace(statements[0]), "CREATE TABLE image_data_operations (") || strings.Contains(dataOperationsSQL, "CASCADE") {
		t.Fatal("unexpected transfer DDL")
	}
}

func TestVersionedHistoryRejectsInvalidSequences(t *testing.T) {
	r := migrations()
	h1 := r[0].checksum()
	for _, tc := range []struct {
		name          string
		rows          [][]driver.Value
		prefix, valid bool
	}{
		{"empty runtime", nil, false, false}, {"empty migrator", nil, true, true},
		{"complete", historyRows(1), false, true},
		{"gap", [][]driver.Value{{int64(2), h1, false}}, true, false},
		{"duplicate", [][]driver.Value{{int64(1), h1, false}, {int64(1), h1, false}}, true, false},
		{"future", append(historyRows(1), []driver.Value{int64(2), h1, false}), true, false},
		{"dirty", [][]driver.Value{{int64(1), h1, true}}, true, false},
		{"changed", [][]driver.Value{{int64(1), strings.Repeat("0", 64), false}}, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			steps := append(historyStructureSteps(), sqlStep{query: historyQuery, rows: tc.rows})
			n, err := readHistory(context.Background(), scriptDB(t, steps), r, tc.prefix)
			if (err == nil) != tc.valid || (err == nil && n != len(tc.rows)) {
				t.Fatal(n, err)
			}
		})
	}
	// Serve never migrates: an empty history is refused without DDL.
	steps := append(historyStructureSteps(), sqlStep{query: historyQuery})
	if VerifySchema(context.Background(), scriptDB(t, steps)) == nil {
		t.Fatal("unmigrated schema accepted for serve")
	}
}

func TestMigrationApplicationAndFailures(t *testing.T) {
	for _, applied := range []int{0, 1} {
		t.Run([]string{"initial", "idempotent"}[applied], func(t *testing.T) {
			if err := Migrate(context.Background(), scriptDB(t, migrationStepsFrom(applied))); err != nil {
				t.Fatal(err)
			}
		})
	}
	for _, label := range []string{"dirty-insert", "DDL", "structural", "completion", "unacknowledged", "dirty-readback", "release"} {
		t.Run(label, func(t *testing.T) {
			steps := migrationStepsFrom(0)
			q := "INSERT INTO schema_migrations (version, checksum, dirty) VALUES (1, ?, TRUE)"
			switch label {
			case "DDL":
				q = strings.Split(dataOperationsSQL, ";")[0]
			case "structural":
				q = transferTableQuery
			case "completion", "unacknowledged":
				q = "UPDATE schema_migrations SET dirty=FALSE, applied_at=UTC_TIMESTAMP(6) WHERE version=1 AND dirty=TRUE AND checksum=?"
			case "dirty-readback":
				at := sqlStepIndex(steps, historyQuery, true)
				steps[at].rows[0][2] = true
				if Migrate(context.Background(), scriptDB(t, steps)) == nil {
					t.Fatal("dirty completion accepted")
				}
				return
			case "release":
				steps[len(steps)-1].rows = [][]driver.Value{{int64(0)}}
				if Migrate(context.Background(), scriptDB(t, steps)) == nil {
					t.Fatal("release failure accepted")
				}
				return
			}
			at := sqlStepIndex(steps, q, false)
			if label == "unacknowledged" {
				steps[at].affected = 0
				steps = append(steps[:at+1], steps[len(steps)-1])
			} else {
				steps = failedMigrationStep(steps, at, errors.New("migration failure"))
			}
			if Migrate(context.Background(), scriptDB(t, steps)) == nil {
				t.Fatal("failed migration accepted")
			}
		})
	}
}
