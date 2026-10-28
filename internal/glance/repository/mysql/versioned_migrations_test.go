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
	if len(r) != 1 || r[0].version != 1 || r[0].sql != metadataSQL || r[0].checksum() != "ba6a24fd786cca2198c49e7e88d49ed0eebf910d84bef1e6557049dc9513690b" {
		t.Fatal("historical migration changed or a future version was registered")
	}
	for _, r := range [][]migration{nil, {{2, "SELECT 1"}}, {{1, "SELECT 1"}, {1, "SELECT 1"}}, {{1, "SELECT 1"}, {3, "SELECT 1"}}, {{1, " "}}} {
		if validateRegistry(r) == nil {
			t.Fatal("invalid registry accepted", r)
		}
	}
}

func TestVersionedHistoryRejectsInvalidSequences(t *testing.T) {
	// Only a double uses this synthetic statement: it is not migration 002 SQL.
	r := append(migrations(), migration{version: 2, sql: "SELECT 1"})
	h1, h2 := r[0].checksum(), r[1].checksum()
	for _, tc := range []struct {
		name   string
		rows   [][]driver.Value
		prefix bool
		valid  bool
	}{
		{"empty runtime", nil, false, false},
		{"empty migrator", nil, true, true},
		{"v1 runtime missing available version", [][]driver.Value{{int64(1), h1, false}}, false, false},
		{"v1 migrator prefix", [][]driver.Value{{int64(1), h1, false}}, true, true},
		{"complete", [][]driver.Value{{int64(1), h1, false}, {int64(2), h2, false}}, false, true},
		{"gap", [][]driver.Value{{int64(2), h2, false}}, true, false},
		{"duplicate", [][]driver.Value{{int64(1), h1, false}, {int64(1), h1, false}}, true, false},
		{"unknown", [][]driver.Value{{int64(1), h1, false}, {int64(3), h2, false}}, true, false},
		{"dirty second", [][]driver.Value{{int64(1), h1, false}, {int64(2), h2, true}}, true, false},
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
	// Production has no version 2: never accept a future record even with a hash.
	steps := append(historyStructureSteps(), sqlStep{query: historyQuery, rows: [][]driver.Value{{int64(1), h1, false}, {int64(2), h2, false}}})
	if VerifySchema(context.Background(), scriptDB(t, steps)) == nil {
		t.Fatal("unavailable version accepted")
	}
}

func TestAppendOnlyMigrationExecutionWithSyntheticSQL(t *testing.T) {
	r := append(migrations(), migration{version: 2, sql: "SELECT 1"})
	for _, state := range []string{"from-empty", "from-v1", "idempotent", "DDL-failure", "final-ack-failure", "lock-unavailable"} {
		t.Run(state, func(t *testing.T) {
			steps := migrationSteps(true)
			// Retain all account, autocommit, lock and structure checks from B20.
			i := sqlStepIndex(steps, historyQuery, false)
			rows := [][]driver.Value{{int64(1), r[0].checksum(), false}}
			if state == "from-empty" {
				steps = migrationSteps(false)
				i = sqlStepIndex(steps, historyQuery, false)
				// Existing B20 initial application already includes verified readback.
				steps = steps[:len(steps)-1]
			} else if state == "idempotent" {
				rows = append(rows, []driver.Value{int64(2), r[1].checksum(), false})
				steps[i].rows = rows
				steps[i+1].rows = [][]driver.Value{{int64(2)}}
			}
			if state == "lock-unavailable" {
				at := sqlStepIndex(steps, "SELECT GET_LOCK(?, 0)", false)
				steps[at].rows = [][]driver.Value{{int64(0)}}
				if migrateVersions(context.Background(), scriptDB(t, steps[:at+1]), r) == nil {
					t.Fatal("lock failure accepted")
				}
				return
			}
			if state != "idempotent" {
				if state != "from-empty" {
					steps = steps[:len(steps)-1]
				}
				pending := execute("INSERT INTO schema_migrations (version, checksum, dirty) VALUES (2, ?, TRUE)")
				checkHash := func(args []driver.NamedValue) {
					if len(args) != 1 || args[0].Value != r[1].checksum() {
						t.Error("wrong version checksum binding")
					}
				}
				pending.checkArgs = checkHash
				steps = append(steps, pending)
				ddl := execute(r[1].sql)
				if state == "DDL-failure" {
					ddl.err = errors.New("synthetic failure")
				}
				steps = append(steps, ddl)
				if state != "DDL-failure" {
					finish := execute("UPDATE schema_migrations SET dirty=FALSE, applied_at=UTC_TIMESTAMP(6) WHERE version=2 AND dirty=TRUE AND checksum=?")
					finish.checkArgs = checkHash
					if state == "final-ack-failure" {
						finish.affected = 0
					}
					steps = append(steps, finish)
					if state != "final-ack-failure" {
						steps = append(steps, historyStructureSteps()...)
						steps = append(steps, sqlStep{query: historyQuery, rows: [][]driver.Value{{int64(1), r[0].checksum(), false}, {int64(2), r[1].checksum(), false}}})
					}
				}
				steps = append(steps, query("SELECT RELEASE_LOCK(?)", int64(1)))
			}
			err := migrateVersions(context.Background(), scriptDB(t, steps), r)
			wantOK := state == "from-empty" || state == "from-v1" || state == "idempotent"
			if (err == nil) != wantOK {
				t.Fatal(err)
			}
		})
	}
}
