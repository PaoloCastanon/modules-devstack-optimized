package mysql

import (
	"context"
	"database/sql"
	"errors"
	"regexp"
	"strings"
)

const historyTableQuery = `SELECT TABLE_TYPE, ENGINE FROM information_schema.TABLES
WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME='schema_migrations'`

const historyColumnsQuery = `SELECT COLUMN_NAME, DATA_TYPE, COLUMN_TYPE, IS_NULLABLE,
CHARACTER_MAXIMUM_LENGTH, CHARACTER_SET_NAME, COLLATION_NAME, DATETIME_PRECISION,
COLUMN_DEFAULT, EXTRA, GENERATION_EXPRESSION FROM information_schema.COLUMNS
WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME='schema_migrations'`

const historyPrimaryQuery = `SELECT COLUMN_NAME, SEQ_IN_INDEX, NON_UNIQUE, SUB_PART
FROM information_schema.STATISTICS WHERE TABLE_SCHEMA=DATABASE()
AND TABLE_NAME='schema_migrations' AND INDEX_NAME='PRIMARY' ORDER BY SEQ_IN_INDEX`

// historyTable permits absence only so the explicit migrator can create it.
// Existing non-InnoDB objects must be rejected before any history DDL or DML.
func historyTable(ctx context.Context, q querier) (bool, error) {
	rows, err := q.QueryContext(ctx, historyTableQuery)
	if err != nil {
		return false, errors.New("cannot inspect migration history table")
	}
	defer rows.Close()
	if !rows.Next() {
		if rows.Err() != nil {
			return false, errors.New("cannot inspect migration history table")
		}
		return false, nil
	}
	var tableType string
	var engine sql.NullString
	if rows.Scan(&tableType, &engine) != nil {
		return false, errors.New("cannot read migration history table provider/inventory")
	}
	if rows.Next() || rows.Err() != nil {
		return false, errors.New("cannot unambiguously inspect migration history table")
	}
	if tableType != "BASE TABLE" || !engine.Valid || !strings.EqualFold(engine.String, "InnoDB") {
		return false, errors.New("incompatible migration history: an InnoDB base table is required; manual review required")
	}
	return true, nil
}

type historyColumn struct {
	name, dataType, columnType, nullable string
	length, precision                    sql.NullInt64
	charset, collation, defaultValue     sql.NullString
	extra, generated                     string
}

// MySQL versions may include deprecated integer display widths in COLUMN_TYPE.
// They do not change the signed integer domain; unsigned/zerofill are rejected.
var historyIntegerType = regexp.MustCompile(`^(bigint|tinyint)(\([0-9]+\))?$`)

func validHistoryColumn(c historyColumn) bool {
	if c.defaultValue.Valid || c.extra != "" || c.generated != "" {
		return false
	}
	dataType, columnType := strings.ToLower(c.dataType), strings.ToLower(c.columnType)
	if match := historyIntegerType.FindStringSubmatch(columnType); match != nil {
		columnType = match[1]
	}
	if c.name == "checksum" {
		return dataType == "char" && columnType == "char(64)" && c.nullable == "NO" &&
			c.length.Valid && c.length.Int64 == 64 && c.charset.Valid && c.charset.String == "ascii" &&
			c.collation.Valid && c.collation.String == "ascii_bin" && !c.precision.Valid
	}
	if c.length.Valid || c.charset.Valid || c.collation.Valid {
		return false
	}
	switch c.name {
	case "version":
		return dataType == "bigint" && columnType == "bigint" && c.nullable == "NO" && !c.precision.Valid
	case "dirty":
		return dataType == "tinyint" && columnType == "tinyint" && c.nullable == "NO" && !c.precision.Valid
	case "applied_at":
		return dataType == "datetime" && columnType == "datetime(6)" && c.nullable == "YES" && c.precision.Valid && c.precision.Int64 == 6
	default:
		return false
	}
}

func verifyHistoryStructure(ctx context.Context, q querier) error {
	exists, err := historyTable(ctx, q)
	if err != nil {
		return err
	}
	if !exists {
		return errors.New("migration history table is missing; run explicit migrations")
	}
	rows, err := q.QueryContext(ctx, historyColumnsQuery)
	if err != nil {
		return errors.New("cannot inspect migration history columns")
	}
	defer rows.Close()
	seen := make(map[string]bool, 4)
	for rows.Next() {
		var c historyColumn
		if rows.Scan(&c.name, &c.dataType, &c.columnType, &c.nullable, &c.length, &c.charset,
			&c.collation, &c.precision, &c.defaultValue, &c.extra, &c.generated) != nil {
			return errors.New("cannot read migration history column provider/inventory")
		}
		if seen[c.name] || !validHistoryColumn(c) {
			return errors.New("incompatible migration history columns; manual review required")
		}
		seen[c.name] = true
	}
	if rows.Err() != nil {
		return errors.New("cannot inspect migration history columns")
	}
	if len(seen) != 4 {
		return errors.New("incomplete migration history columns; manual review required")
	}
	// Close the provider/inventory result before issuing another query on a pinned Conn.
	if err := rows.Close(); err != nil {
		return errors.New("cannot finish migration history column inspection")
	}
	primary, err := q.QueryContext(ctx, historyPrimaryQuery)
	if err != nil {
		return errors.New("cannot inspect migration history primary key")
	}
	defer primary.Close()
	count := 0
	for primary.Next() {
		var name sql.NullString
		var sequence, nonUnique int64
		var prefix sql.NullInt64
		if primary.Scan(&name, &sequence, &nonUnique, &prefix) != nil {
			return errors.New("cannot read migration history primary key provider/inventory")
		}
		if !name.Valid || name.String != "version" || sequence != 1 || nonUnique != 0 || prefix.Valid {
			return errors.New("incompatible migration history primary key; manual review required")
		}
		count++
	}
	if primary.Err() != nil {
		return errors.New("cannot inspect migration history primary key")
	}
	if count != 1 {
		return errors.New("migration history requires a primary key on version only; manual review required")
	}
	return nil
}
