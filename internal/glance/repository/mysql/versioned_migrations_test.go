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
	if len(r) != 2 || r[0].version != 1 || r[0].sql != metadataSQL || r[0].checksum() != "ba6a24fd786cca2198c49e7e88d49ed0eebf910d84bef1e6557049dc9513690b" || r[1].version != 2 || r[1].sql != dataOperationsSQL || r[1].verify == nil {
		t.Fatal("invalid append-only registry")
	}
	for _, r := range [][]migration{nil, {{version: 2, sql: "SELECT 1"}}, {{version: 1, sql: "SELECT 1"}, {version: 1, sql: "SELECT 1"}}, {{version: 1, sql: "SELECT 1"}, {version: 3, sql: "SELECT 1"}}, {{version: 1, sql: " "}}} {
		if validateRegistry(r) == nil {
			t.Fatal("invalid registry accepted")
		}
	}
	// The current approved DDL is exactly one semicolon-delimited statement.
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
	h1, h2 := r[0].checksum(), r[1].checksum()
	for _, tc := range []struct {
		name          string
		rows          [][]driver.Value
		prefix, valid bool
	}{
		{"empty runtime", nil, false, false}, {"empty migrator", nil, true, true},
		{"v1 runtime", historyRows(1), false, false}, {"v1 migrator", historyRows(1), true, true},
		{"complete", historyRows(2), false, true},
		{"gap", [][]driver.Value{{int64(2), h2, false}}, true, false},
		{"duplicate", [][]driver.Value{{int64(1), h1, false}, {int64(1), h1, false}}, true, false},
		{"unknown", [][]driver.Value{{int64(1), h1, false}, {int64(3), h2, false}}, true, false},
		{"future", append(historyRows(2), []driver.Value{int64(3), h2, false}), true, false},
		{"dirty first", [][]driver.Value{{int64(1), h1, true}}, true, false},
		{"dirty second", [][]driver.Value{{int64(1), h1, false}, {int64(2), h2, true}}, true, false},
		{"changed first", [][]driver.Value{{int64(1), strings.Repeat("0", 64), false}}, true, false},
		{"changed second", [][]driver.Value{{int64(1), h1, false}, {int64(2), strings.Repeat("0", 64), false}}, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			steps := append(historyStructureSteps(), sqlStep{query: historyQuery, rows: tc.rows})
			n, err := readHistory(context.Background(), scriptDB(t, steps), r, tc.prefix)
			if (err == nil) != tc.valid || (err == nil && n != len(tc.rows)) {
				t.Fatal(n, err)
			}
		})
	}
	// Serve does not upgrade v1. No DDL/Exec operations exist in this script.
	steps := append(historyStructureSteps(), sqlStep{query: historyQuery, rows: historyRows(1)})
	if VerifySchema(context.Background(), scriptDB(t, steps)) == nil {
		t.Fatal("v1 accepted for serve")
	}
}

func TestMigration002ApplicationAndFailures(t *testing.T) {
	for _, applied := range []int{0, 1, 2} {
		t.Run([]string{"initial-v1-v2", "upgrade-v1", "idempotent-v2"}[applied], func(t *testing.T) {
			if err := Migrate(context.Background(), scriptDB(t, migrationStepsFrom(applied))); err != nil {
				t.Fatal(err)
			}
		})
	}
	for _, label := range []string{"dirty-insert", "DDL", "structural", "completion", "unacknowledged", "dirty-readback", "release"} {
		t.Run(label, func(t *testing.T) {
			steps := migrationStepsFrom(1)
			r := migrations()[1]
			q := "INSERT INTO schema_migrations (version, checksum, dirty) VALUES (2, ?, TRUE)"
			switch label {
			case "DDL":
				q = strings.Split(r.sql, ";")[0]
			case "structural":
				q = transferTableQuery
			case "completion", "unacknowledged":
				q = "UPDATE schema_migrations SET dirty=FALSE, applied_at=UTC_TIMESTAMP(6) WHERE version=2 AND dirty=TRUE AND checksum=?"
			case "dirty-readback":
				at := sqlStepIndex(steps, historyQuery, true)
				steps[at].rows[1][2] = true
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
