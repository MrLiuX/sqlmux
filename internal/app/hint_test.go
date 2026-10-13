package app

import (
	"slices"
	"strings"
	"testing"

	"github.com/kackerx/sqlmux/internal/db"
	"github.com/kackerx/sqlmux/internal/ui"
)

// hinted is a query tab on the orders table, whose columns are in the cache,
// with a function of the catalog.
func hinted(t *testing.T, conf string) (*App, *queryTab) {
	t.Helper()
	a, c := inQuery(t, conf, "")
	table, cols, _ := ordersTable(0)
	conn := a.sess.Conns[0]
	for i, tb := range conn.Tables {
		if tb.Name == "t_order" {
			conn.Tables[i] = table
		}
	}
	conn.cols[idOf(table)] = cols
	conn.cols[tableID{"public", "t_user"}] = db.Columns{Cols: []db.Column{{Name: "id", Type: "uuid"}}}
	conn.funcs = []db.Function{
		{Schema: "public", Name: "add_tax", Args: "price numeric(10,2), rate numeric(4,2)"},
		{Schema: "other", Name: "add_tax", Args: "amount integer"},
	}
	return a, c
}

// shown is the fields of the signature at the cursor and the one lit.
func shown(a *App, c *queryTab) string {
	s := a.signature(c)
	if len(s.Fields) == 0 {
		return ""
	}
	return strings.Join(s.Fields, ", ") + " #" + string(rune('0'+s.Cur))
}

// The line follows the cursor: the INSERT's columns with the one the value
// goes in, by the commas of the row; none out of the row (F15.3).
func TestSignatureInsert(t *testing.T) {
	a, c := hinted(t, "")
	feed(t, a, "iinsert into t_order (id, status, paid) values (")
	if got := shown(a, c); got != "id bigint, status order_status, paid boolean #0" {
		t.Fatalf("the first value: %q", got)
	}
	feed(t, a, "1, ")
	if got := shown(a, c); got != "id bigint, status order_status, paid boolean #1" {
		t.Fatalf("the second: %q", got)
	}
	feed(t, a, "'done', true, ")
	if got := shown(a, c); got != "" {
		t.Fatalf("a fourth value has no column: %q", got)
	}
	feed(t, a, "1), (")
	if got := shown(a, c); !strings.HasSuffix(got, "#0") {
		t.Fatalf("the next row counts again: %q", got)
	}
	feed(t, a, "<Esc>0")
	if got := shown(a, c); got != "" {
		t.Fatalf("moved out of the row: %q", got)
	}
	feed(t, a, "$")
	if got := shown(a, c); !strings.HasSuffix(got, "#0") {
		t.Fatalf("moved back in, in NORMAL: %q", got)
	}
}

// Without a column list the values go to the table's columns in its order; a
// table whose columns the cache lacks gives the names that are written, with
// no types.
func TestSignatureInsertColumns(t *testing.T) {
	a, c := hinted(t, "")
	feed(t, a, "iinsert into t_order values (1, 'done', ")
	if got := shown(a, c); got != "id bigint, status order_status, amount numeric(10,2), paid boolean, meta jsonb, note text, created_at timestamp with time zone #2" {
		t.Fatalf("no list: %q", got)
	}
	feed(t, a, "<Esc>ggdGiinsert into t_order (id, status) values (1, ")
	delete(a.sess.Conns[0].cols, idOf(db.Table{Schema: "public", Name: "t_order"}))
	if got := shown(a, c); got != "id, status #1" {
		t.Fatalf("columns not in: %q", got)
	}
	a.sess.Conns[0].cols[idOf(db.Table{Schema: "public", Name: "t_order"})] = db.Columns{Cols: []db.Column{{Name: "bare"}}}
	feed(t, a, "<Esc>ggdGiinsert into t_order (bare) values (")
	if got := shown(a, c); got != "bare #0" {
		t.Fatalf("a column with no type: %q", got)
	}
	feed(t, a, "<Esc>ggdGiinsert into t_order (id, status) select 1, ")
	if got := shown(a, c); got != "" {
		t.Fatalf("INSERT … SELECT: %q", got)
	}
}

// An UPDATE's SET names its column, a condition's right side its column, a
// qualified one by its alias; a call of the catalog its arguments, a call it
// does not know leaves the place it is an argument of.
func TestSignatureOtherPlaces(t *testing.T) {
	a, c := hinted(t, "")
	for _, k := range []struct{ sql, want string }{
		{"update t_order set note = 'x', status = ", "status order_status #0"},
		{"update t_order o set o.paid = ", "paid boolean #0"},
		{"select * from t_order where status = ", "status order_status #0"},
		{"select * from t_order o where o.amount > 1 and o.id in (1, ", "id bigint #0"},
		{"select * from t_order o join t_user u on u.id = ", "id uuid #0"}, // the table the qualifier names has it
		{"select * from t_order o join t_user u on o.id = ", "id bigint #0"},
		{"select * from t_order o join t_user u on x.id = ", ""},
		{"select * from t_order where nope = ", ""},
		{"select other.add_tax(", "amount integer #0"},
		{"select other.add_tax(1, ", ""},
		{"select add_tax(1, ", "price numeric(10,2), rate numeric(4,2) #1"}, // the first of the schemas that has the argument
		{"select public.add_tax(", "price numeric(10,2), rate numeric(4,2) #0"},
		{"select add_tax(1, 2, ", ""}, // an argument it has none for
		{"insert into t_order (id, status) values (1, lower(", "id bigint, status order_status #1"},
		{"insert into t_order (id, status) values (1, add_tax(1, ", "price numeric(10,2), rate numeric(4,2) #1"},
		{"select now(", ""},
		{"update t_order set paid = true where ", ""},
	} {
		feed(t, a, "<Esc>ggdGi"+k.sql)
		if got := shown(a, c); got != k.want {
			t.Errorf("%q: %q, want %q", k.sql, got, k.want)
		}
	}
}

// `[query] hints = false` has none.
func TestSignatureOff(t *testing.T) {
	a, c := hinted(t, "[query]\nhints = false\n")
	feed(t, a, "iinsert into t_order (id, status) values (")
	if got := shown(a, c); got != "" {
		t.Fatalf("off: %q", got)
	}
	if strings.Contains(a.render().String(), "id bigint") {
		t.Error("drawn though it is off")
	}
}

// The line is drawn over the cursor's, the field being filled in lit, and on
// the line under it when the cursor is in the first row of the view.
func TestSignatureDrawn(t *testing.T) {
	a, _ := hinted(t, "")
	feed(t, a, "iselect 1;<CR>insert into t_order (id, status) values (1, ")
	rows := strings.Split(a.render().String(), "\n")
	at := slices.IndexFunc(rows, func(r string) bool { return strings.Contains(r, "id bigint, status order_status") })
	if at < 0 {
		t.Fatalf("no line:\n%s", strings.Join(rows, "\n"))
	}
	cur := slices.IndexFunc(rows, func(r string) bool { return strings.Contains(r, "values (1, ") })
	if cur != at+1 {
		t.Errorf("the line is at %d, the cursor's at %d", at, cur)
	}
	// the one lit is the second: its first cell, a column after the first one's end
	f, th := a.render(), ui.TokyonightStorm
	col := func(s string) int {
		before, _, _ := strings.Cut(rows[at], s)
		return ui.Width(before)
	}
	if got := f.Buf.CellAt(col("status order_status"), at).Style.Fg; got != th.Focus {
		t.Errorf("status is %v, not lit", got)
	}
	if got := f.Buf.CellAt(col("id bigint"), at).Style.Fg; got != th.Dim {
		t.Errorf("id is %v, not dim", got)
	}
	feed(t, a, "<Esc>ggdGiinsert into t_order (id, status) values (1, ")
	rows = strings.Split(a.render().String(), "\n")
	at = slices.IndexFunc(rows, func(r string) bool { return strings.Contains(r, "id bigint, status order_status") })
	cur = slices.IndexFunc(rows, func(r string) bool { return strings.Contains(r, "values (1, ") })
	if cur < 0 || at != cur+1 {
		t.Errorf("on the first row the line is under it: %d and %d", at, cur)
	}
}

// The arguments of a function split at the commas outside parentheses.
func TestSplitArgs(t *testing.T) {
	for in, want := range map[string]string{
		"": "", "integer": "integer", "a integer, b numeric(10,2), c text": "a integer|b numeric(10,2)|c text",
		"a numeric(10, 2), b text": "a numeric(10, 2)|b text", "a int,b int,c int": "a int|b int|c int",
	} {
		if got := strings.Join(splitArgs(in), "|"); got != want {
			t.Errorf("%q: %q, want %q", in, got, want)
		}
	}
}

// Only the focused query has the line; the others show none though their
// cursors are in a place for one.
func TestSignatureFocused(t *testing.T) {
	a, _ := hinted(t, "")
	feed(t, a, "iinsert into t_order (id, status) values (1, <Esc>")
	if !strings.Contains(a.render().String(), "id bigint") {
		t.Fatal("not drawn in the focused query")
	}
	feed(t, a, "<C-h>")
	if strings.Contains(a.render().String(), "id bigint") {
		t.Error("drawn in a query that has not the focus")
	}
}
