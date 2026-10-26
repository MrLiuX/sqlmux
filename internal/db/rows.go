package db

import (
	"database/sql"
	"strings"
)

// What the drivers of database/sql share in reading a result (F14.1): MySQL's
// and SQLite's; PG's has its own protocol.

// SQLArgs are args as database/sql takes them: the text, NULL for a NULL.
func SQLArgs(args []Val) []any {
	vals := make([]any, len(args))
	for i, a := range args {
		if !a.Null {
			vals[i] = a.S
		}
	}
	return vals
}

// Capped is the want of EachRow, and of PG's reading, for a result of which at
// most maxRows rows (0: all) are kept: past them, r is Truncated and the rows
// are read and dropped.
func Capped(r *Result, maxRows int) func() bool {
	return func() bool {
		if maxRows > 0 && len(r.Rows) == maxRows {
			r.Truncated = true
			return false
		}
		return true
	}
}

// EachRow reads the result in rows: head with its columns, typed as the driver
// names their types, in lower case, then row with each of the rows want says
// to keep, the others read and dropped; cell makes a value of what the driver
// gave for a column, whose type tells the engine how the text goes — a
// datetime's scale say (§8.1, F18.6).
func EachRow(rows *sql.Rows, cell func(v any, col *sql.ColumnType) Val, head func([]Col) error, want func() bool, row func([]Val) error) error {
	types, err := rows.ColumnTypes()
	if err != nil {
		return err
	}
	raw, dest, cols := make([]any, len(types)), make([]any, len(types)), make([]Col, len(types))
	for i, t := range types {
		cols[i], dest[i] = Col{Name: t.Name(), Type: strings.ToLower(t.DatabaseTypeName())}, &raw[i]
	}
	if err := head(cols); err != nil {
		return err
	}
	for rows.Next() {
		if !want() {
			continue
		}
		if err := rows.Scan(dest...); err != nil {
			return err
		}
		vals := make([]Val, len(raw))
		for i, v := range raw {
			vals[i] = cell(v, types[i])
		}
		if err := row(vals); err != nil {
			return err
		}
	}
	return rows.Err()
}
