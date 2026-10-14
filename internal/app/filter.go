package app

import (
	"slices"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/kackerx/sqlmux/internal/db"
	"github.com/kackerx/sqlmux/internal/i18n"
	"github.com/kackerx/sqlmux/internal/sqlkit"
	"github.com/kackerx/sqlmux/internal/ui"
)

// noEquals are the types whose values a condition cannot be written for: no
// literal, or no = (PG's json, xml, point and polygon), or one compared by
// area or length, not by what it is (box, circle, lseg, line, path), or one
// that never matches (MySQL's JSON against a string; a four-byte float, PG's
// real and MySQL's float, against a literal: 0.1 is not the float 0.1, where
// the eight-byte ones are); arrays are cut by their []
// (§7.6「按单元值过滤」).
var noEquals = []string{"bytea", "blob", "tinyblob", "mediumblob", "longblob", "binary", "varbinary", "json", "xml",
	"geometry", "point", "linestring", "polygon", "multipoint", "multilinestring", "multipolygon", "geometrycollection", "geomcollection",
	"box", "circle", "path", "lseg", "line", "real", "float4", "float"}

// cellCondition is the condition that keeps the rows whose column col, of
// type typ, holds v: col = literal, col IS NULL for a NULL; ok is false for
// a type that has none. The literal follows the column's type: a number or a
// boolean is written as it is, any other value as a string.
// ponytail: the type decides, not the value; a number column holding NaN or
// Infinity is a string literal, which PG reads as one.
func cellCondition(d sqlkit.Dialect, quoted, typ string, fixed bool, v db.Val) (cond string, ok bool) {
	if fixed || strings.HasSuffix(typ, "[]") || slices.Contains(noEquals, plainType(typ)) {
		return "", false
	}
	if v.Null {
		return quoted + " IS NULL", true
	}
	if db.IsBinaryShown(v.S) { // the grid's word for bytes it does not have: no condition keeps them (a MySQL geometry's type is not always its own)
		return "", false
	}
	lit := sqlkit.Literal(v.S, d)
	switch colType(typ) {
	case ui.ColNumber:
		if isPlainNumber(v.S) {
			lit = v.S
		}
	case ui.ColBool:
		switch {
		case slices.Contains([]string{"t", "true"}, v.S):
			lit = "true"
		case slices.Contains([]string{"f", "false"}, v.S):
			lit = "false"
		case isPlainNumber(v.S): // MySQL's tinyint(1)
			lit = v.S
		}
	case ui.ColString, ui.ColTime, ui.ColJSON, ui.ColOther:
	}
	return quoted + " = " + lit, true
}

// isPlainNumber is whether s is a number written with digits, a sign, a point
// and an exponent only: what a condition can have bare.
func isPlainNumber(s string) bool {
	if strings.Trim(s, "0123456789+-.eE") != "" {
		return false
	}
	_, err := strconv.ParseFloat(s, 64)
	return err == nil
}

// filterCell is * (grid.filter.cell): the cell under the cursor as a
// condition joined to the WHERE, run from the first page and in the history
// as a typed one is; a condition that is there already is toasted, not run.
func (a *App) filterCell() tea.Cmd {
	t, ok := a.focusedGrid()
	if !ok {
		return nil
	}
	s, ok := t.cellAtCursor()
	if !ok || s.add != nil { // a row added has no value in the database
		return nil
	}
	if !t.conn.sql() { // a Redis key's filter is a MATCH pattern, not a condition of a cell
		return a.showToast(i18n.T("这一列的类型不支持按值过滤"))
	}
	d := t.conn.eng.Dialect()
	cond, ok := cellCondition(d, t.conn.eng.Quote(s.key.col), s.col.Type, s.col.Fixed, s.orig)
	if !ok {
		return a.showToast(i18n.T("这一列的类型不支持按值过滤"))
	}
	where, changed := sqlkit.AddCondition(t.applied, cond, d)
	if !changed {
		return a.showToast(i18n.T("条件已在 WHERE 里"))
	}
	t.where.Text, t.where.Pos = where, len(where)
	return a.runWhere(t)
}

// cursorColumn is the focused table's column under the cursor, by name.
func (a *App) cursorColumn() (*dataTab, string, bool) {
	t, ok := a.focusedGrid()
	if !ok || t.fieldAt(t.col) < 0 {
		return nil, "", false
	}
	return t, t.page.Cols[t.fieldAt(t.col)].Name, true
}
