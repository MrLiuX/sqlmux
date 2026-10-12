package app

import (
	"regexp"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"

	"github.com/kackerx/sqlmux/internal/db"
	"github.com/kackerx/sqlmux/internal/editor"
	"github.com/kackerx/sqlmux/internal/i18n"
	"github.com/kackerx/sqlmux/internal/sqlkit"
)

// The structure changes (F14.4, §7.8, §12): no form, a statement of the
// engine's own that opens in a new query, a placeholder in it, <name> or
// <type>, selected for the user to write over; C-e runs it, with the same
// question a dangerous one gets.

// alterKind is which template: the actions' names are its.
type alterKind string

const (
	addColumn  alterKind = "add_column"
	addIndex   alterKind = "add_index"
	renameTbl  alterKind = "rename"
	dropColumn alterKind = "drop_column"
	dropIndex  alterKind = "drop_index"
)

// alterTarget is the table the templates are about, and the column or index
// the cursor is on, if it is on one: the tree node under its cursor when the
// tree has the keys, else the focused table tab's table and its cursor's
// column.
func (a *App) alterTarget() (c *Conn, t db.Table, col, index string, ok bool) {
	if p := a.focused(); p == a.win().Tree {
		n, found := a.treeNode()
		if !found || n.table.Name == "" {
			return nil, t, "", "", false
		}
		if n.kind == nodeIndex {
			index = n.index.Name
		}
		return n.conn, n.table, n.column, index, true
	}
	d := dataOf(a.focused())
	if d == nil {
		return nil, t, "", "", false
	}
	if gs, _, _, found := a.gridOf(a.focused()); found {
		if shown := d.shownCols(); gs.col >= 0 && gs.col < len(shown) {
			col = d.page.Cols[shown[gs.col]].Name
		}
	}
	return d.conn, d.table, col, "", true
}

// alterSQL is the statement of kind for table t of an engine, a column or an index
// written in when the cursor was on one, else the placeholders <name>, <type>,
// <column>, <index>, <new_name>; "" says why not in why: a view has no column to add,
// SQLite does not rename one.
func alterSQL(c *Conn, kind alterKind, t db.Table, col, index string) (sql, why string) {
	q, d := c.eng.Quote, c.eng.Dialect()
	table := q(t.Schema, t.Name)
	name := func(known, placeholder string) string {
		if known != "" {
			return q(known)
		}
		return placeholder
	}
	if t.View() && kind != renameTbl {
		return "", i18n.T("%s 是视图", t.Name)
	}
	switch kind {
	case addColumn:
		return "alter table " + table + " add column <name> <type>;", ""
	case addIndex:
		on := table
		if d == sqlkit.SQLite { // its ON takes a table with no schema
			on = q(t.Name)
		}
		return "create index <name> on " + on + " (" + name(col, "<column>") + ");", ""
	case renameTbl:
		switch {
		case d == sqlkit.MySQL:
			return "rename table " + table + " to <new_name>;", ""
		case d == sqlkit.SQLite && t.View():
			return "", i18n.T("SQLite 不能给视图改名")
		}
		object := "table"
		switch t.Kind {
		case "v":
			object = "view"
		case "m":
			object = "materialized view"
		}
		return "alter " + object + " " + table + " rename to <new_name>;", ""
	case dropColumn:
		return "alter table " + table + " drop column " + name(col, "<column>") + ";", ""
	case dropIndex:
		switch d {
		case sqlkit.MySQL:
			return "drop index " + name(index, "<index>") + " on " + table + ";", ""
		case sqlkit.SQLite:
			return "drop index " + name(index, "<index>") + ";", ""
		case sqlkit.PG:
		}
		return "drop index " + q(t.Schema) + "." + name(index, "<index>") + ";", ""
	}
	return "", ""
}

// alterTitles are the templates' titles, in the menu and the palette.
var alterTitles = map[alterKind]string{
	addColumn: i18n.N("加列…"), addIndex: i18n.N("加索引…"), renameTbl: i18n.N("改名…"),
	dropColumn: i18n.N("删除列…"), dropIndex: i18n.N("删除索引…"),
}

// alter is the action of kind: its statement in a new query on the table's
// connection, the first placeholder selected (F14.4).
func (a *App) alter(kind alterKind) tea.Cmd {
	c, t, col, index, ok := a.alterTarget()
	if !ok {
		return a.showToast(i18n.T("先选一张表"))
	}
	if c.State != connReady {
		return a.showToast(i18n.T("连接还没连上"))
	}
	sql, why := alterSQL(c, kind, t, col, index)
	if why != "" {
		return a.showToast(why)
	}
	return a.queryWith(c, t.Schema, sql, true)
}

var placeholder = regexp.MustCompile(`<[a-z_]+>`)

// selectPlaceholder puts the cursor of query t on its first placeholder, and,
// in VISUAL, over it, so that c writes the name in its place; the modeless
// editor has no selection to put, the cursor goes to its start.
func selectPlaceholder(t *queryTab) {
	lines := t.ed.Lines()
	for i, l := range lines {
		if loc := placeholder.FindStringIndex(l); loc != nil {
			t.ed.Goto(editor.Pos{Line: i, Col: loc[0]})
			if !t.ed.Plain() {
				t.ed.Feed("v")
				for range utf8.RuneCountInString(l[loc[0]:loc[1]]) - 1 {
					t.ed.Feed("l")
				}
			}
			return
		}
	}
}

// alterItems are the table node's menu items for the templates; a view's is
// only the rename (F14.4).
func (a *App) alterItems(n node) (items []menuItem) {
	kinds := []alterKind{addColumn, addIndex, renameTbl, dropColumn, dropIndex}
	if n.table.View() {
		kinds = []alterKind{renameTbl}
	}
	for _, k := range kinds {
		if _, why := alterSQL(n.conn, k, n.table, "", ""); why == "" {
			items = append(items, menuItem{i18n.T(alterTitles[k]), func() tea.Cmd { return a.alter(k) }})
		}
	}
	return items
}
