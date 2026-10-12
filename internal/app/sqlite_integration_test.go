//go:build integration

package app

import (
	"slices"
	"strings"
	"testing"

	"github.com/kackerx/sqlmux/internal/config"
	"github.com/kackerx/sqlmux/internal/db"
	"github.com/kackerx/sqlmux/internal/db/sqlite"
)

// A SQLite table with no key but its rowid reads, edits and exports with the
// rowid in front: select rowid, *; the rowid's own cell takes no edit; a
// change goes to the row by the rowid (F14.1).
func TestSQLiteRowidTable(t *testing.T) {
	a := opened(t, config.Connection{Name: "lite", Engine: "sqlite", DSN: sqlite.OwnDB(t)})
	lite := a.sess.Conns[0]
	settle(a, a.openTable(lite, lite.Tables[slices.IndexFunc(lite.Tables, func(tb db.Table) bool { return tb.Name == "t_log" })]))
	tab := dataOf(a.focused())
	if tab == nil || tab.page.Cols[0].Name != "rowid" || tab.cols.Lead != "rowid, " || len(tab.page.Rows) != 3 {
		t.Fatalf("t_log: %+v", tab)
	}
	tab.col = 0
	if s, ok := tab.cellAtCursor(); !ok || s.why == "" {
		t.Errorf("the rowid's cell takes an edit: %+v", s)
	}
	line := slices.IndexFunc(tab.page.Cols, func(c db.Col) bool { return c.Name == "line" })
	tab.edits = map[editKey]edit{{tab.rowKey(1), "line"}: {val: db.Val{S: "mine"}, orig: tab.page.Rows[1][line]}}
	settle(a, a.save())
	if got := tab.page.Rows[1][line].S; got != "mine" || tab.bar != nil {
		t.Fatalf("saved: %q, bar %v", got, tab.bar)
	}
	r, err := lite.Meta.Query(t.Context(), "select group_concat(line, ' ') from t_log")
	if err != nil || r.Rows[0][0].S != "started mine stopped" {
		t.Errorf("in the file: %v %v", r.Rows, err)
	}
	src := a.exportSource(&dropdown{kind: dropExport, tab: tab, pane: a.focused()})
	if !strings.HasPrefix(src.all.sql, "select rowid, * from") {
		t.Errorf("an export asks %q", src.all.sql)
	}
}
