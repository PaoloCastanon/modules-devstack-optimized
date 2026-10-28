package mysql

import (
	"context"
	"database/sql"
	"errors"
	"reflect"
)

const transferTableQuery = "SELECT TABLE_TYPE, ENGINE FROM information_schema.TABLES WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME='image_data_operations'"
const transferColumnsQuery = "SELECT COLUMN_NAME, COLUMN_TYPE, IS_NULLABLE, COLLATION_NAME, COLUMN_DEFAULT, EXTRA, GENERATION_EXPRESSION FROM information_schema.COLUMNS WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME='image_data_operations' ORDER BY ORDINAL_POSITION"
const transferIndexesQuery = "SELECT INDEX_NAME, COLUMN_NAME, SEQ_IN_INDEX, NON_UNIQUE, SUB_PART FROM information_schema.STATISTICS WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME='image_data_operations' ORDER BY INDEX_NAME, SEQ_IN_INDEX"
const transferForeignQuery = "SELECT k.COLUMN_NAME, k.REFERENCED_TABLE_NAME, k.REFERENCED_COLUMN_NAME, r.DELETE_RULE, r.UPDATE_RULE FROM information_schema.KEY_COLUMN_USAGE k JOIN information_schema.REFERENTIAL_CONSTRAINTS r ON r.CONSTRAINT_SCHEMA=k.CONSTRAINT_SCHEMA AND r.CONSTRAINT_NAME=k.CONSTRAINT_NAME AND r.TABLE_NAME=k.TABLE_NAME WHERE k.TABLE_SCHEMA=DATABASE() AND k.TABLE_NAME='image_data_operations' AND k.REFERENCED_TABLE_SCHEMA=DATABASE() AND k.REFERENCED_TABLE_NAME IS NOT NULL ORDER BY k.ORDINAL_POSITION"
const transferChecksQuery = "SELECT CONSTRAINT_NAME, ENFORCED FROM information_schema.TABLE_CONSTRAINTS WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME='image_data_operations' AND CONSTRAINT_TYPE='CHECK' ORDER BY CONSTRAINT_NAME"

// Verify essential table/column/key/enforcement metadata, not a SQL expression
// equivalence proof. Migration checksums bind the approved CHECK definitions.
func verifyTransferStructure(ctx context.Context, q querier) error {
	specs := []struct {
		query    string
		expected [][]string
	}{
		{transferTableQuery, [][]string{{"BASE TABLE", "InnoDB"}}},
		{transferColumnsQuery, [][]string{
			{"image_id", "char(36)", "NO", "ascii_bin", "<NULL>", "", ""},
			{"attempt_id", "char(36)", "NO", "ascii_bin", "<NULL>", "", ""},
			{"phase", "varchar(16)", "NO", "ascii_bin", "<NULL>", "", ""},
			{"expected_size", "bigint", "YES", "<NULL>", "<NULL>", "", ""},
			{"size", "bigint", "YES", "<NULL>", "<NULL>", "", ""},
			{"virtual_size", "bigint", "YES", "<NULL>", "<NULL>", "", ""},
			{"checksum", "char(32)", "YES", "ascii_bin", "<NULL>", "", ""},
			{"os_hash_value", "char(128)", "YES", "ascii_bin", "<NULL>", "", ""},
			{"cleanup_target", "varchar(8)", "YES", "ascii_bin", "<NULL>", "", ""},
			{"last_error_code", "varchar(32)", "YES", "ascii_bin", "<NULL>", "", ""},
			{"created_at", "datetime(6)", "NO", "<NULL>", "<NULL>", "", ""},
			{"updated_at", "datetime(6)", "NO", "<NULL>", "<NULL>", "", ""},
		}},
		{transferIndexesQuery, [][]string{
			{"data_operation_attempt", "attempt_id", "1", "0", "<NULL>"},
			{"data_operation_reconcile", "phase", "1", "1", "<NULL>"},
			{"data_operation_reconcile", "updated_at", "2", "1", "<NULL>"},
			{"data_operation_reconcile", "image_id", "3", "1", "<NULL>"},
			{"PRIMARY", "image_id", "1", "0", "<NULL>"},
		}},
		{transferForeignQuery, [][]string{{"image_id", "images", "id", "NO ACTION", "NO ACTION"}}},
		{transferChecksQuery, [][]string{
			{"data_operation_cleanup", "YES"}, {"data_operation_error", "YES"}, {"data_operation_md5", "YES"},
			{"data_operation_phase", "YES"}, {"data_operation_receipt", "YES"}, {"data_operation_sha512", "YES"}, {"data_operation_sizes", "YES"},
		}},
	}
	for _, spec := range specs {
		rows, err := q.QueryContext(ctx, spec.query)
		if err != nil {
			return errors.New("cannot inspect image data schema")
		}
		var got [][]string
		for rows.Next() {
			values := make([]sql.NullString, len(spec.expected[0]))
			args := make([]any, len(values))
			for n := range values {
				args[n] = &values[n]
			}
			if rows.Scan(args...) != nil {
				_ = rows.Close()
				return errors.New("cannot read image data schema")
			}
			record := make([]string, len(values))
			for n, v := range values {
				record[n] = "<NULL>"
				if v.Valid {
					record[n] = v.String
				}
			}
			// MySQL may use RESTRICT rather than its synonym NO ACTION.
			if spec.query == transferForeignQuery {
				for n := 3; n < 5; n++ {
					if record[n] == "RESTRICT" {
						record[n] = "NO ACTION"
					}
				}
			}
			if spec.query == transferColumnsQuery && (record[0] == "size" || record[0] == "expected_size" || record[0] == "virtual_size") && record[1] == "bigint(20)" {
				record[1] = "bigint"
			}
			got = append(got, record)
		}
		rowErr := rows.Err()
		closeErr := rows.Close()
		if rowErr != nil || closeErr != nil {
			return errors.New("cannot finish image data schema inspection")
		}
		if !reflect.DeepEqual(got, spec.expected) {
			return errors.New("incompatible image data schema; manual review required")
		}
	}
	return nil
}
