package mysql

import (
	"context"
	"database/sql/driver"
	"errors"
	"strings"
	"testing"
)

func transferStructureSteps() []sqlStep {
	return []sqlStep{
		query(transferTableQuery, "BASE TABLE", "InnoDB"),
		{query: transferColumnsQuery, rows: [][]driver.Value{
			{"image_id", "char(36)", "NO", "ascii_bin", nil, "", ""},
			{"attempt_id", "char(36)", "NO", "ascii_bin", nil, "", ""},
			{"phase", "varchar(16)", "NO", "ascii_bin", nil, "", ""},
			{"expected_size", "bigint", "YES", nil, nil, "", ""},
			{"size", "bigint", "YES", nil, nil, "", ""},
			{"virtual_size", "bigint", "YES", nil, nil, "", ""},
			{"checksum", "char(32)", "YES", "ascii_bin", nil, "", ""},
			{"os_hash_value", "char(128)", "YES", "ascii_bin", nil, "", ""},
			{"cleanup_target", "varchar(8)", "YES", "ascii_bin", nil, "", ""},
			{"last_error_code", "varchar(32)", "YES", "ascii_bin", nil, "", ""},
			{"created_at", "datetime(6)", "NO", nil, nil, "", ""},
			{"updated_at", "datetime(6)", "NO", nil, nil, "", ""},
		}},
		{query: transferIndexesQuery, rows: [][]driver.Value{
			{"data_operation_attempt", "attempt_id", int64(1), int64(0), nil},
			{"data_operation_reconcile", "phase", int64(1), int64(1), nil},
			{"data_operation_reconcile", "updated_at", int64(2), int64(1), nil},
			{"data_operation_reconcile", "image_id", int64(3), int64(1), nil},
			{"PRIMARY", "image_id", int64(1), int64(0), nil},
		}},
		query(transferForeignQuery, "image_id", "images", "id", "NO ACTION", "NO ACTION"),
		{query: transferChecksQuery, rows: [][]driver.Value{
			{"data_operation_cleanup", "YES"}, {"data_operation_error", "YES"}, {"data_operation_md5", "YES"},
			{"data_operation_phase", "YES"}, {"data_operation_receipt", "YES"}, {"data_operation_sha512", "YES"}, {"data_operation_sizes", "YES"},
		}},
	}
}

func TestTransferStructureFailsClosed(t *testing.T) {
	for _, tc := range []struct {
		name string
		at   int
		edit func(*sqlStep)
	}{
		{"valid", -1, nil},
		{"memory", 0, func(s *sqlStep) { s.rows[0][1] = "MEMORY" }},
		{"missing", 0, func(s *sqlStep) { s.rows = nil }},
		{"nullable image", 1, func(s *sqlStep) { s.rows[0][2] = "YES" }},
		{"wrong uuid collation", 1, func(s *sqlStep) { s.rows[0][3] = "ascii_general_ci" }},
		{"missing column", 1, func(s *sqlStep) { s.rows = s.rows[:11] }},
		{"nonunique attempt", 2, func(s *sqlStep) { s.rows[0][3] = int64(1) }},
		{"cascade", 3, func(s *sqlStep) { s.rows[0][3] = "CASCADE" }},
		{"unenforced check", 4, func(s *sqlStep) { s.rows[0][1] = "NO" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			steps := transferStructureSteps()
			if tc.at >= 0 {
				tc.edit(&steps[tc.at])
				steps = steps[:tc.at+1]
			}
			err := verifyTransferStructure(context.Background(), scriptDB(t, steps))
			if (err == nil) != (tc.at < 0) {
				t.Fatal(err)
			}
		})
	}
	for at := 0; at < 5; at++ {
		t.Run([]string{"table", "columns", "indexes", "foreign", "checks"}[at], func(t *testing.T) {
			steps := transferStructureSteps()[:at+1]
			steps[at].err = errors.New("private sql secret")
			if err := verifyTransferStructure(context.Background(), scriptDB(t, steps)); err == nil || strings.Contains(err.Error(), "private sql secret") {
				t.Fatal(err)
			}
		})
	}
}
