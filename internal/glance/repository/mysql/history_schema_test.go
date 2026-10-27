package mysql

import (
	"context"
	"database/sql/driver"
	"errors"
	"strings"
	"testing"
)

// Independent fixtures for MySQL 8 metadata. These are protocol doubles, not a
// running server: they cannot establish disk durability or restart recovery.
func historyStructureSteps() []sqlStep {
	return []sqlStep{
		query(historyTableQuery, "BASE TABLE", "InnoDB"),
		{query: historyColumnsQuery, rows: [][]driver.Value{
			{"version", "bigint", "bigint", "NO", nil, nil, nil, nil, nil, "", ""},
			{"checksum", "char", "char(64)", "NO", int64(64), "ascii", "ascii_bin", nil, nil, "", ""},
			{"dirty", "tinyint", "tinyint(1)", "NO", nil, nil, nil, nil, nil, "", ""},
			{"applied_at", "datetime", "datetime(6)", "YES", nil, nil, nil, int64(6), nil, "", ""},
		}},
		query(historyPrimaryQuery, "version", int64(1), int64(0), nil),
	}
}

func TestHistoryStructureValid(t *testing.T) {
	for _, widths := range []bool{false, true} {
		name := "current integer types"
		if widths {
			name = "legacy integer display widths"
		}
		t.Run(name, func(t *testing.T) {
			steps := historyStructureSteps()
			if widths {
				steps[1].rows[0][2] = "bigint(20)"
				steps[1].rows[2][2] = "tinyint(4)"
			}
			steps = append(steps, query(historyQuery, int64(1), checksum(), false))
			if err := VerifySchema(context.Background(), scriptDB(t, steps)); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestHistoryIncompatibleMetadata(t *testing.T) {
	for _, tc := range []struct {
		name  string
		index int
		edit  func(*sqlStep)
	}{
		{"MEMORY engine", 0, func(s *sqlStep) { s.rows[0][1] = "MEMORY" }},
		{"MyISAM engine", 0, func(s *sqlStep) { s.rows[0][1] = "MyISAM" }},
		{"view", 0, func(s *sqlStep) { s.rows[0] = []driver.Value{"VIEW", nil} }},
		{"unknown engine", 0, func(s *sqlStep) { s.rows[0][1] = nil }},
		{"missing table", 0, func(s *sqlStep) { s.rows = nil }},
		{"ambiguous table", 0, func(s *sqlStep) { s.rows = append(s.rows, []driver.Value{"BASE TABLE", "InnoDB"}) }},
		{"missing column", 1, func(s *sqlStep) { s.rows = s.rows[:3] }},
		{"extra column", 1, func(s *sqlStep) {
			s.rows = append(s.rows, []driver.Value{"extra", "int", "int", "YES", nil, nil, nil, nil, nil, "", ""})
		}},
		{"duplicate column", 1, func(s *sqlStep) { s.rows = append(s.rows, s.rows[0]) }},
		{"version wrong type", 1, func(s *sqlStep) { s.rows[0][1] = "int"; s.rows[0][2] = "int" }},
		{"version unsigned", 1, func(s *sqlStep) { s.rows[0][2] = "bigint unsigned" }},
		{"version nullable", 1, func(s *sqlStep) { s.rows[0][3] = "YES" }},
		{"version auto increment", 1, func(s *sqlStep) { s.rows[0][9] = "auto_increment" }},
		{"checksum varchar", 1, func(s *sqlStep) { s.rows[1][1] = "varchar"; s.rows[1][2] = "varchar(64)" }},
		{"checksum wrong length", 1, func(s *sqlStep) { s.rows[1][4] = int64(63) }},
		{"checksum wrong charset", 1, func(s *sqlStep) { s.rows[1][5] = "utf8mb4" }},
		{"checksum wrong collation", 1, func(s *sqlStep) { s.rows[1][6] = "ascii_general_ci" }},
		{"checksum nullable", 1, func(s *sqlStep) { s.rows[1][3] = "YES" }},
		{"dirty nullable", 1, func(s *sqlStep) { s.rows[2][3] = "YES" }},
		{"dirty unsigned", 1, func(s *sqlStep) { s.rows[2][2] = "tinyint unsigned" }},
		{"dirty default", 1, func(s *sqlStep) { s.rows[2][8] = "0" }},
		{"generated column", 1, func(s *sqlStep) { s.rows[2][10] = "(1)" }},
		{"invisible column", 1, func(s *sqlStep) { s.rows[2][9] = "INVISIBLE" }},
		{"applied_at timestamp", 1, func(s *sqlStep) { s.rows[3][1] = "timestamp"; s.rows[3][2] = "timestamp(6)" }},
		{"applied_at precision", 1, func(s *sqlStep) { s.rows[3][7] = int64(3) }},
		{"applied_at nonnullable", 1, func(s *sqlStep) { s.rows[3][3] = "NO" }},
		{"missing primary key", 2, func(s *sqlStep) { s.rows = nil }},
		{"wrong primary column", 2, func(s *sqlStep) { s.rows[0][0] = "checksum" }},
		{"composite primary key", 2, func(s *sqlStep) { s.rows = append(s.rows, []driver.Value{"checksum", int64(2), int64(0), nil}) }},
		{"nonunique primary metadata", 2, func(s *sqlStep) { s.rows[0][2] = int64(1) }},
		{"prefix primary key", 2, func(s *sqlStep) { s.rows[0][3] = int64(4) }},
		{"expression primary metadata", 2, func(s *sqlStep) { s.rows[0][0] = nil }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			steps := historyStructureSteps()
			tc.edit(&steps[tc.index])
			steps = steps[:tc.index+1]
			if err := VerifySchema(context.Background(), scriptDB(t, steps)); err == nil {
				t.Fatal("incompatible history accepted")
			}
		})
	}
}

func TestHistoryMetadataInspectionFailsClosed(t *testing.T) {
	for i, label := range []string{"table", "columns", "primary key"} {
		for _, rowError := range []bool{false, true} {
			name := label + " query"
			if rowError {
				name = label + " rows"
			}
			t.Run(name, func(t *testing.T) {
				steps := historyStructureSteps()[:i+1]
				failure := errors.New("private-metadata-driver-password")
				if rowError {
					steps[i].rowsErr = failure
				} else {
					steps[i].err = failure
				}
				err := VerifySchema(context.Background(), scriptDB(t, steps))
				if err == nil || strings.Contains(err.Error(), "private-metadata-driver-password") {
					t.Fatal("inspection failure accepted or raw SQL error disclosed")
				}
			})
		}
	}
	for _, index := range []int{0, 1, 2} {
		t.Run([]string{"table scan", "column scan", "primary scan"}[index], func(t *testing.T) {
			steps := historyStructureSteps()[:index+1]
			switch index {
			case 0:
				steps[index].rows[0][0] = nil
			case 1:
				steps[index].rows[0][0] = nil
			case 2:
				steps[index].rows[0][1] = "not-an-integer"
			}
			if VerifySchema(context.Background(), scriptDB(t, steps)) == nil {
				t.Fatal("unreadable metadata accepted")
			}
		})
	}
}

func TestMigrateRejectsPreexistingHistoryWithoutWrites(t *testing.T) {
	for _, memory := range []bool{true, false} {
		name := "MEMORY"
		if !memory {
			name = "InnoDB incompatible columns"
		}
		t.Run(name, func(t *testing.T) {
			steps := migrationSteps(true)
			i := sqlStepIndex(steps, historyTableQuery, false)
			if memory {
				steps[i].rows[0][1] = "MEMORY"
			} else {
				i = sqlStepIndex(steps, historyColumnsQuery, false)
				steps[i].rows[0][1] = "int"
			}
			steps = append(steps[:i+1], steps[len(steps)-1])
			if err := Migrate(context.Background(), scriptDB(t, steps)); err == nil {
				t.Fatal("incompatible preexisting history accepted")
			}
			// This script contains no Exec step. Any CREATE/ALTER/DML attempt fails it.
		})
	}
}

func TestMigrateInspectionFailureDoesNotReportSuccess(t *testing.T) {
	t.Run("creation did not produce InnoDB", func(t *testing.T) {
		steps := migrationSteps(false)
		i := sqlStepIndex(steps, historyColumnsQuery, false) - 1
		steps[i].rows[0][1] = "MEMORY"
		steps = append(steps[:i+1], steps[len(steps)-1])
		if Migrate(context.Background(), scriptDB(t, steps)) == nil {
			t.Fatal("new history was not checked after CREATE")
		}
	})
	for _, afterCreate := range []bool{false, true} {
		name := "before creation"
		if afterCreate {
			name = "after creation"
		}
		t.Run(name, func(t *testing.T) {
			steps := migrationSteps(false)
			i := sqlStepIndex(steps, historyTableQuery, false)
			if afterCreate {
				i = sqlStepIndex(steps, historyColumnsQuery, false)
			}
			steps = failedMigrationStep(steps, i, errors.New("metadata inaccessible"))
			if Migrate(context.Background(), scriptDB(t, steps)) == nil {
				t.Fatal("uninspected history reported success")
			}
		})
	}
	t.Run("final engine changed", func(t *testing.T) {
		steps := migrationSteps(false)
		i := sqlStepIndex(steps, historyTableQuery, true)
		steps[i].rows[0][1] = "MEMORY"
		steps = append(steps[:i+1], steps[len(steps)-1])
		if Migrate(context.Background(), scriptDB(t, steps)) == nil {
			t.Fatal("final incompatible history reported success")
		}
	})
}
