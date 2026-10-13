package app

import (
	"strings"
	"testing"

	"github.com/kackerx/sqlmux/internal/config"
	"github.com/kackerx/sqlmux/internal/db"
	"github.com/kackerx/sqlmux/internal/editor"
	"github.com/kackerx/sqlmux/internal/ui"
)

// after is the labels of the candidates sqlComplete gives for sql, the cursor at
// its end or at the | in it.
func after(a *App, conn *Conn, sql string) string {
	pos := strings.Index(sql, "|")
	if pos < 0 {
		pos = len(sql)
	} else {
		sql = sql[:pos] + sql[pos+1:]
	}
	c, _ := a.sqlComplete(conn, sql, pos, "public", map[tableID]bool{}, false)
	if c == nil {
		return "-"
	}
	var ls []string
	for _, it := range c.items {
		ls = append(ls, it.label)
	}
	return strings.Join(ls, " ")
}

// INSERT INTO t and a blank offers its columns and VALUES in one, the cursor in
// the parentheses of the values as the first tabstop to fill in; a generated
// column is not in it, nor is anything offered once a word is begun (F15.11).
func TestInsertSkeleton(t *testing.T) {
	a, c := hinted(t, "")
	table, cols, _ := ordersTable(0)
	cols.Cols[4].Generated = true
	a.sess.Conns[0].cols[idOf(table)] = cols
	feed(t, a, "iinsert into t_order ")
	if got := offered(c); got != "(id, status, amount, pai…) values ()" || c.comp.sel != noSelection || c.comp.items[0].note != "全部 6 列" {
		t.Fatalf("the skeleton: %q, picked %d, %q", got, c.comp.sel, c.comp.items[0].note)
	}
	feed(t, a, "<Tab><CR>")
	want := "insert into t_order (id, status, amount, paid, note, created_at) values ()"
	if text(c) != want || c.ed.Cursor() != (editor.Pos{Col: len(want) - 1}) || c.ed.Mode() != editor.Insert {
		t.Fatalf("↵: %q, the cursor at %v in %v", text(c), c.ed.Cursor(), c.ed.Mode())
	}
	feed(t, a, "1, ")
	if got := text(c); got != "insert into t_order (id, status, amount, paid, note, created_at) values (1, )" {
		t.Fatalf("typing in the values: %q", got)
	}
	feed(t, a, "<Esc>")
	feed(t, a, "ggdGiinsert into t_order ")
	feed(t, a, "<Tab><CR>1<Tab>")
	if text(c) != want[:len(want)-1]+"1)" || c.ed.Cursor() != (editor.Pos{Col: len(want) + 1}) {
		t.Fatalf("Tab after the last tabstop: %q at %v", text(c), c.ed.Cursor())
	}
	feed(t, a, "<Esc>ggdGiinsert into t_order v")
	if got := offered(c); strings.Contains(got, "values ()") {
		t.Fatalf("a word begun: %q", got)
	}
	feed(t, a, "<Esc>ggdGiinsert into t_order (id) ")
	if got := offered(c); got != "-" {
		t.Fatalf("the columns are written: %q", got)
	}
	only := db.Table{Schema: "public", Name: "t_gen"}
	a.sess.Conns[0].Tables = append(a.sess.Conns[0].Tables, only)
	a.sess.Conns[0].cols[idOf(only)] = db.Columns{Cols: []db.Column{{Name: "id", Type: "bigint", Generated: true}}}
	feed(t, a, "<Esc>ggdGiinsert into t_gen ")
	if got := offered(c); got != "-" {
		t.Fatalf("nothing to insert: %q", got)
	}
	feed(t, a, "<Esc>ggdGiinsert into t_nope ")
	if got := offered(c); got != "-" {
		t.Fatalf("a table that is not there: %q", got)
	}
}

// The quick SQL's input has no tabstops: the cursor goes to the first.
func TestInsertSkeletonInTheInput(t *testing.T) {
	a := sized(160, 45, "nerd")
	table, cols, _ := ordersTable(0)
	conn := a.sess.Conns[0]
	conn.cols[idOf(table)] = cols
	sql := "insert into t_order "
	c, _ := a.sqlComplete(conn, sql, len(sql), "public", map[tableID]bool{}, false)
	if c == nil {
		t.Fatal("no skeleton")
	}
	c.sel = 0
	in := ui.Input{Text: sql, Pos: len(sql)}
	c.accept(&in)
	if want := "insert into t_order (id, status, amount, paid, meta, note, created_at) values ()"; in.Text != want || in.Pos != len(want)-1 {
		t.Fatalf("%q at %d", in.Text, in.Pos)
	}
}

// An UPDATE's SET lists the columns of the tables it names before the SET and
// nothing else, typed or not, a table in a FROM after it or a CTE before it
// not among them (F15.11).
func TestUpdateSetColumns(t *testing.T) {
	a, _ := hinted(t, "")
	conn := a.sess.Conns[0]
	conn.cols[tableID{"public", "t_user"}] = db.Columns{Cols: []db.Column{{Name: "email", Type: "text"}}}
	const own = "id status amount paid meta note created_at"
	for sql, want := range map[string]string{
		"update t_order set ":                                                 own,
		"update t_order set st":                                               "status",
		"update t_order set status = 'x', ":                                   own,
		"update t_order set status = 'x', no":                                 "note",
		"update t_order set | from t_user":                                    own,
		"update t_order o set |, id = 1 from t_user u":                        own,
		"with c as (select 1) update t_order set ":                            own,
		"with c as (select * from t_user) update t_order set ":                own,
		"update public.t_order set ":                                          own,
		"insert into t_order (id) values (1) on conflict (id) do update set ": own,
		"update t_order set status = ":                                        "pending running done failed null",
		"update t_order set status = 'x' where ":                              "id status amount paid meta note created_at",
		"update t_order join t_user on t_user.id = t_order.id set ":           own + " email", // MySQL's, both tables, in turn
		"update t_order o, t_user u set u.email = 'x', ":                      own + " email",
		"update t_order join t_user on 1 set st":                              "status",
		"update t_order join (select * from t_user) u on 1 set ":              own,  // a derived table's FROM is another level
		"update t_order join t_order x on 1 set id":                           "id", // a table once
	} {
		if got := after(a, conn, sql); got != want {
			t.Errorf("%q: %q, want %q", sql, got, want)
		}
	}
}

// A name that has to be quoted goes in quoted, whichever the list it is in: a
// column, a table, all the columns at once, a CTE, the WHERE's (F15.11).
func TestCompletionQuotes(t *testing.T) {
	a, _ := hinted(t, "")
	conn := a.sess.Conns[0]
	q := db.Table{Schema: "public", Name: "Order"}
	conn.Tables = append(conn.Tables, q)
	conn.cols[idOf(q)] = db.Columns{Cols: []db.Column{{Name: "Status", Type: "text"}, {Name: "user", Type: "text"}, {Name: "created at", Type: "text"}, {Name: "plain", Type: "text"}}}
	insert := func(sql, label string) string {
		pos := len(sql)
		c, _ := a.sqlComplete(conn, sql, pos, "public", map[tableID]bool{}, false)
		if c != nil {
			for _, it := range c.items {
				if it.label == label {
					return it.insert
				}
			}
		}
		return "-"
	}
	for _, k := range []struct{ sql, label, want string }{
		{`select * from "Order" where `, "Status", `"Status"`},
		{`select * from "Order" where `, "user", `"user"`},
		{`select * from "Order" where `, "created at", `"created at"`},
		{`select * from "Order" where `, "plain", "plain"},
		{"select * from Ord", "Order", `"Order"`},
		{"select * from t_o", "t_order", "t_order"},
		{`insert into "Order" (`, "Status, user, created at, plain", `"Status", "user", "created at", plain`},
		{`insert into "Order" (`, "user", `"user"`},
		{`insert into "Order" `, "(Status, user, created a…) values ()", `("Status", "user", "created at", plain) values ()`},
		{`with "My Cte" as (select 1) select * from My`, "My Cte", `"My Cte"`},
		{`update "Order" set `, "Status", `"Status"`},
		{`select "Order".`, "Status", `"Status"`},
	} {
		if got := insert(k.sql, k.label); got != k.want {
			t.Errorf("%s | %s: %s, want %s", k.sql, k.label, got, k.want)
		}
	}
	// MySQL's backticks, and a keyword
	my := newConn(config.Connection{Name: "m", Engine: "mysql", DSN: "u:p@tcp(m:3306)/s"})
	tb := db.Table{Schema: "s", Name: "t"}
	my.Tables = []db.Table{tb}
	my.cols[idOf(tb)] = db.Columns{Cols: []db.Column{{Name: "Key", Type: "int"}, {Name: "a b", Type: "int"}, {Name: "Mixed", Type: "int"}}}
	c, _ := a.sqlComplete(my, "select * from t where ", len("select * from t where "), "s", map[tableID]bool{}, false)
	var ins []string
	for _, it := range c.items {
		ins = append(ins, it.insert)
	}
	if got := strings.Join(ins, " "); got != "`Key` `a b` Mixed" {
		t.Errorf("MySQL: %q", got)
	}
}

// The WHERE's columns are quoted as the query's are.
func TestWhereCompletionQuotes(t *testing.T) {
	a, tab, _ := withRecorder(t)
	for i := range tab.cols.Cols {
		if tab.cols.Cols[i].Name == "created_at" {
			tab.cols.Cols[i].Name = "Created At"
		}
	}
	feed(t, a, "/Cre")
	if tab.comp == nil || tab.comp.items[0].label != "Created At" || tab.comp.items[0].insert != `"Created At"` {
		t.Fatalf("%+v", tab.comp)
	}
}
