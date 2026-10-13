package app

import (
	"slices"
	"strings"
	"testing"

	"github.com/kackerx/sqlmux/internal/config"
	"github.com/kackerx/sqlmux/internal/db"
	"github.com/kackerx/sqlmux/internal/sqlkit"
)

// offered is the labels of the candidates up in query c, joined; "-" when none
// is, and ": none" or the first picked.
func offered(c *queryTab) string {
	if c.comp == nil {
		return "-"
	}
	var ls []string
	for _, it := range c.comp.items {
		ls = append(ls, it.label)
	}
	return strings.Join(ls, " ")
}

// Where a value of a column goes the candidates are its own (F15.10): an enum's
// labels quoted, then NULL; none for a column that takes no such value; a
// string typed there is matched whole, and ↵ takes the pick over its closing
// quote.
func TestValueCompletion(t *testing.T) {
	a, c := hinted(t, "")
	feed(t, a, "iinsert into t_order (id, status, paid, note) values (")
	if got := offered(c); got != "-" { // id: bigint not null, nothing to offer
		t.Fatalf("a not null number: %q", got)
	}
	feed(t, a, "1, ")
	if got := offered(c); got != "pending running done failed null" || c.comp.sel != noSelection {
		t.Fatalf("an enum: %q, picked %d", got, c.comp.sel)
	}
	feed(t, a, "<Tab><CR>")
	if text(c) != "insert into t_order (id, status, paid, note) values (1, 'pending')" || c.comp != nil {
		t.Fatalf("Tab and ↵: %q", text(c))
	}
	feed(t, a, "<Esc>ggdGiinsert into t_order (id, status, paid, note) values (1, 'ru")
	if got := offered(c); got != "running" || c.comp.sel != 0 {
		t.Fatalf("in a string: %q", got)
	}
	feed(t, a, "<CR>")
	if text(c) != "insert into t_order (id, status, paid, note) values (1, 'running')" {
		t.Fatalf("over the closing quote: %q", text(c))
	}
	feed(t, a, "<Esc>ggdGiinsert into t_order (id, status, paid, note) values (1, 'done', ")
	if got := offered(c); got != "true false null" {
		t.Fatalf("a boolean: %q", got)
	}
	feed(t, a, "t")
	if got := offered(c); !strings.HasPrefix(got, "true ") || c.comp.sel != 0 { // the values first, then what a word is anywhere
		t.Fatalf("a boolean typed: %q", got)
	}
	feed(t, a, "<Esc>ggdGiinsert into t_order (id, status, paid, note) values (1, 'done', true, ")
	if got := offered(c); got != "null" {
		t.Fatalf("a nullable text: %q", got)
	}
	feed(t, a, "<Esc>ggdGiinsert into t_order (id, status) values (1, '")
	if got := offered(c); got != "pending running done failed" || c.comp.sel != noSelection { // only literals in a string, nothing picked
		t.Fatalf("in an empty string: %q, picked %d", got, c.comp.sel)
	}
	feed(t, a, "<Esc>ggdGiinsert into t_order (id, paid) values (1, '")
	if got := offered(c); got != "-" {
		t.Fatalf("a boolean in a string: %q", got)
	}
	feed(t, a, "<Esc>ggdGiinsert into t_order (id) values (1, ")
	if got := offered(c); got != "-" {
		t.Fatalf("more values than columns: %q", got)
	}
	feed(t, a, "<Esc>ggdGiinsert into t_order (id, status) values (1 ")
	if got := offered(c); got != "-" {
		t.Fatalf("after a value: %q", got)
	}
	feed(t, a, "<Esc>ggdGiupdate t_order set status = 'done' ")
	if got := offered(c); got != "-" {
		t.Fatalf("after a string: %q", got)
	}
	feed(t, a, "<Esc>ggdGiinsert into t_order (id, status) values (1, 5")
	if got := offered(c); got != "-" {
		t.Fatalf("after a number: %q", got)
	}
	feed(t, a, "<Esc>ggdGiinsert into t_order (id, status) values (1, lower(")
	if got := offered(c); got != "-" {
		t.Fatalf("a call's argument: %q", got)
	}
	feed(t, a, "<Esc>ggdGiinsert into t_order (id, status) values (1, (")
	if got := offered(c); got != "-" {
		t.Fatalf("a parenthesis in the row: %q", got)
	}
	feed(t, a, "<Esc>ggdGiinsert into t_order (id, status) select 1, ")
	if got := offered(c); strings.Contains(got, "pending") { // the select list's columns come, no values
		t.Fatalf("INSERT … SELECT: %q", got)
	}
}

// An UPDATE's SET and a condition offer the column's values too, the SET its
// DEFAULT, the condition neither that nor NULL; a time column its now.
func TestValueCompletionPlaces(t *testing.T) {
	a, c := hinted(t, "")
	table, cols, _ := ordersTable(0)
	cols.Cols[6].Default = "now()"
	cols.Cols[0].Default = "nextval('t_order_id_seq'::regclass)"
	a.sess.Conns[0].cols[idOf(table)] = cols
	for _, k := range []struct{ sql, want string }{
		{"update t_order set created_at = ", "now() current_timestamp null default"},
		{"update t_order set id = ", "default"},
		{"update t_order set status = ", "pending running done failed null"},
		{"update t_order set status = (", "-"},
		{"select * from t_order where created_at = ", "now() current_timestamp"}, // no DEFAULT nor NULL in a condition: = null is never true
		{"select * from t_order o where o.status = ", "pending running done failed"},
		{"select * from t_order where status in ('done', ", "pending running done failed"},
		{"select * from t_order where status in (", "pending running done failed"},
		{"select * from t_order where status <> ", "pending running done failed"},
		{"insert into t_order (created_at) values (", "now() current_timestamp null default"},
		{"insert into t_order (id) values (", "default"},
		{"update t_order set status = 'done', note = 'x', ", "id status amount paid meta note created_at"}, // a column, which is not a value's place
	} {
		feed(t, a, "<Esc>ggdGi"+k.sql)
		if got := offered(c); got != k.want {
			t.Errorf("%q: %q, want %q", k.sql, got, k.want)
		}
	}
}

// What a column offers by the engine's own types: SQLite has no now(), and an
// auto_increment of MySQL's is a DEFAULT to a row added alone.
func TestValueCandidates(t *testing.T) {
	for _, c := range []struct {
		col  db.Column
		kind sqlkit.ValueKind
		d    sqlkit.Dialect
		want string
	}{
		{db.Column{Type: "timestamp", Default: "x"}, sqlkit.ValueInsert, sqlkit.SQLite, "current_timestamp null default"},
		{db.Column{Type: "datetime", NotNull: true}, sqlkit.ValueInsert, sqlkit.MySQL, "now() current_timestamp"},
		{db.Column{Type: "tinyint(1)", NotNull: true}, sqlkit.ValueWhere, sqlkit.MySQL, "true false"},
		{db.Column{Type: "int", Default: "auto_increment", NotNull: true}, sqlkit.ValueInsert, sqlkit.MySQL, "default"},
		{db.Column{Type: "int", Default: "auto_increment", NotNull: true}, sqlkit.ValueUpdate, sqlkit.MySQL, ""},
		{db.Column{Type: "integer", Default: "1", AddOnly: true, NotNull: true}, sqlkit.ValueUpdate, sqlkit.SQLite, ""},
		{db.Column{Type: "e", Enum: []string{"it's"}, NotNull: true}, sqlkit.ValueInsert, sqlkit.PG, "it's"},
		{db.Column{Type: "boolean", NotNull: true}, sqlkit.ValueInsert, sqlkit.PG, "true false"},
	} {
		var ls []string
		for _, cd := range valueCandidates(c.col, c.kind, c.d) {
			ls = append(ls, cd.label)
		}
		if got := strings.Join(ls, " "); got != c.want {
			t.Errorf("%+v: %q, want %q", c.col, got, c.want)
		}
	}
	if got := valueCandidates(db.Column{Type: "boolean", NotNull: true}, sqlkit.ValueInsert, sqlkit.PG); got[0].insert != "true" {
		t.Errorf("a boolean is no literal: %+v", got)
	}
	if got := valueCandidates(db.Column{Type: "e", Enum: []string{"it's"}}, sqlkit.ValueInsert, sqlkit.PG); !slices.ContainsFunc(got, func(cd candidate) bool { return cd.insert == "'it''s'" }) {
		t.Errorf("an enum label is a literal: %+v", got)
	}
}

// The columns that are not in the cache are asked for, and the list comes up
// when they are in, the cursor where it was.
func TestValueCompletionColumnsComeLate(t *testing.T) {
	a, c := hinted(t, "")
	table, cols, _ := ordersTable(0)
	delete(a.sess.Conns[0].cols, idOf(table))
	feed(t, a, "iinsert into t_order (id, status) values (1, ")
	if got := offered(c); got != "-" || !c.asked[idOf(table)] {
		t.Fatalf("columns asked for: list %q, asked %v", got, c.asked)
	}
	a.Update(colsMsg{a.sess.Conns[0], table, cols, nil})
	if got := offered(c); got != "pending running done failed null" {
		t.Fatalf("columns in: %q", got)
	}
	delete(a.sess.Conns[0].cols, idOf(table))
	feed(t, a, "<Esc>ggdGiinsert into t_order (id, status) values (1, <Esc>a") // asked for, and forgotten with INSERT
	feed(t, a, "'p")
	if !c.asked[idOf(table)] {
		t.Fatalf("columns asked for in a string: %v", c.asked)
	}
}

// An UPDATE's SET whose table's columns are not in the cache has no list, not
// the tables and keywords a word is anywhere, until they are in (F15.11).
func TestUpdateSetColumnsComeLate(t *testing.T) {
	a, c := hinted(t, "")
	table, cols, _ := ordersTable(0)
	delete(a.sess.Conns[0].cols, idOf(table))
	feed(t, a, "iupdate t_order set st")
	if got := offered(c); got != "-" || !c.asked[idOf(table)] {
		t.Fatalf("columns asked for: list %q, asked %v", got, c.asked)
	}
	a.Update(colsMsg{a.sess.Conns[0], table, cols, nil})
	if got := offered(c); got != "status" {
		t.Fatalf("columns in: %q", got)
	}
}

// C-n with nothing typed lists the value's candidates and what a word is anywhere.
func TestValueCompletionManual(t *testing.T) {
	a, c := hinted(t, "")
	feed(t, a, "iinsert into t_order (id, status) values (1, ")
	feed(t, a, "<Esc>a")
	a.queryComplete(c, true)
	if got := offered(c); !strings.HasPrefix(got, "pending running done failed null ") || !strings.Contains(got, " id status amount ") {
		t.Fatalf("C-n: %q", got)
	}
}

// MySQL's "…" is a string, and what is taken for it is a 'literal', which the
// quote that is there would be left over: no candidate in one, the literal in a '…'.
func TestValueCompletionMySQLString(t *testing.T) {
	conn := newConn(config.Connection{Name: "m", Engine: "mysql", DSN: "u:p@tcp(m:3306)/s"})
	tb := db.Table{Schema: "s", Name: "t"}
	conn.Tables = []db.Table{tb}
	conn.cols[idOf(tb)] = db.Columns{Cols: []db.Column{{Name: "st", Type: "enum('a','b')", Enum: []string{"a", "b"}, NotNull: true}}}
	a := sized(160, 45, "nerd")
	for sql, want := range map[string]int{`insert into t (st) values (`: 2, `insert into t (st) values ('`: 2, `insert into t (st) values ("`: 0} {
		c, _ := a.sqlComplete(conn, sql, len(sql), "s", map[tableID]bool{}, false)
		got := 0
		if c != nil {
			got = len(c.items)
		}
		if got != want {
			t.Errorf("%s: %d candidates, want %d", sql, got, want)
		}
	}
}
