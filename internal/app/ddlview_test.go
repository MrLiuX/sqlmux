package app

import (
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/exp/golden"

	"sqlmux/internal/db"
)

// A column's line of a table's DDL, then the lines that make an index or a key
// of it, then its comment, as PG and MySQL write them (F9.8).
func TestDDLColumnLines(t *testing.T) {
	pg := `create table public.t_sku (
  code text not null,
  "B c" integer,
  title text not null default 'x'::text,
  price numeric(10,2),
  constraint t_sku_pkey PRIMARY KEY (code),
  constraint t_sku_price_check CHECK ((price > 0))
);
CREATE UNIQUE INDEX t_sku_lower_title ON public.t_sku USING btree (lower(title));
CREATE INDEX t_sku_title_code ON public.t_sku USING btree (title, code) INCLUDE (price);
CREATE INDEX other_idx ON public.t_sku USING btree (code_x);
comment on table public.t_sku is 'the title is not here';
comment on column public.t_sku.title is 'what it is called';
comment on column public.t_sku."B c" is 'quoted';`
	my := "CREATE TABLE `t_sku` (\n  `id` bigint NOT NULL AUTO_INCREMENT COMMENT 'the id',\n  `code` varchar(20) NOT NULL,\n  `title` varchar(80) DEFAULT NULL,\n" +
		"  PRIMARY KEY (`id`),\n  UNIQUE KEY `t_sku_code` (`code`),\n  KEY `t_sku_title` (`title`,`code`),\n  CONSTRAINT `fk` FOREIGN KEY (`code`) REFERENCES `x` (`code`)\n) ENGINE=InnoDB;"
	for _, c := range []struct {
		name, ddl, col string
		cat            db.Column
		want           []string
	}{
		{"pg title", pg, "title", db.Column{}, []string{
			"title text not null default 'x'::text",
			"CREATE UNIQUE INDEX t_sku_lower_title ON public.t_sku USING btree (lower(title));",
			"CREATE INDEX t_sku_title_code ON public.t_sku USING btree (title, code) INCLUDE (price);",
			"comment on column public.t_sku.title is 'what it is called';"}},
		{"pg code, a primary key", pg, "code", db.Column{}, []string{
			"code text not null",
			"constraint t_sku_pkey PRIMARY KEY (code)",
			"CREATE INDEX t_sku_title_code ON public.t_sku USING btree (title, code) INCLUDE (price);"}}, // not code_x's
		{"pg quoted", pg, "B c", db.Column{}, []string{`"B c" integer`, `comment on column public.t_sku."B c" is 'quoted';`}},
		{"pg alone", pg, "price", db.Column{}, []string{"price numeric(10,2)", "CREATE INDEX t_sku_title_code ON public.t_sku USING btree (title, code) INCLUDE (price);"}},
		{"mysql", my, "code", db.Column{}, []string{"`code` varchar(20) NOT NULL", "UNIQUE KEY `t_sku_code` (`code`)", "KEY `t_sku_title` (`title`,`code`)"}},
		{"mysql id", my, "id", db.Column{}, []string{"`id` bigint NOT NULL AUTO_INCREMENT COMMENT 'the id'", "PRIMARY KEY (`id`)"}},
		{"a view's column", "create view public.v as\n SELECT id\n   FROM t;", "id", db.Column{Type: "bigint", NotNull: true, Default: "1"}, []string{"id bigint not null default 1"}},
	} {
		if got := ddlColumnLines(c.ddl, c.col, c.cat); !slices.Equal(got, c.want) {
			t.Errorf("%s:\n got %q\nwant %q", c.name, got, c.want)
		}
	}
}

// ddlApp is a table of t_order open with its DDL cached, fh and fH bound to the
// popups; the DDL has an index and a comment for a column.
func ddlApp(t *testing.T, ddl string) (*App, *dataTab) {
	a := configured(t, 160, 45, "[keys.grid]\nfh = \"grid.ddl.column\"\nfH = \"grid.ddl.table\"")
	a.removePane(2)
	tab := loadOrders(t, a, 5)
	if ddl != "" {
		a.sess.Conns[0].ddl[idOf(tab.table)] = ddlText{text: ddl, got: true}
	}
	return a, tab
}

const ddlOrderWith = ddlOrder + "\nCREATE INDEX t_order_status ON public.t_order USING btree (status, id);\ncomment on column public.t_order.status is 'where the order is';"

// fh shows what the DDL says of the cell's column in a small box under it, esc
// closes it, and any other key closes it and is pressed as it is (F9.8).
func TestDDLColumnPopup(t *testing.T) {
	a, tab := ddlApp(t, ddlOrderWith)
	feed(t, a, "l") // status
	keys(t, a, "fh")
	if a.ddl == nil || a.ddl.col != "status" {
		t.Fatalf("popup %+v", a.ddl)
	}
	screen := a.render().String()
	for _, want := range []string{"status order_status not null default 'pending'::order_status", "CREATE INDEX t_order_status", "where the order is"} {
		if !strings.Contains(screen, want) {
			t.Errorf("the box lacks %q", want)
		}
	}
	golden.RequireEqual(t, screen)
	a.Update(tea.MouseWheelMsg{X: 80, Y: 20, Button: tea.MouseWheelDown}) // the column's box is cut, not scrolled
	if a.ddl == nil || a.ddl.top != 0 {
		t.Errorf("wheel: popup %+v", a.ddl)
	}
	feed(t, a, "<Esc>")
	if a.ddl != nil || tab.col != 1 {
		t.Fatalf("esc: popup %v, column %d", a.ddl != nil, tab.col)
	}
	keys(t, a, "fh")
	feed(t, a, "l")
	if a.ddl != nil || tab.col != 2 {
		t.Errorf("l: popup %v, column %d: closed, and moved", a.ddl != nil, tab.col)
	}
}

// A column in many indexes shows what fits and a …, does not scroll, and goes
// with a resize (F9.8).
func TestDDLColumnPopupCut(t *testing.T) {
	ddl := ddlOrder
	for i := range 12 {
		ddl += "\nCREATE INDEX t_order_id_" + strings.Repeat("x", i) + " ON public.t_order USING btree (id);"
	}
	a, _ := ddlApp(t, ddl)
	keys(t, a, "fh")
	text, _, _ := a.ddlContent()
	if strings.Count(text, "\n") < ddlColumnRows {
		t.Fatalf("not many lines: %q", text)
	}
	screen := a.render().String()
	if !strings.Contains(screen, "t_order_id_xxxx ON") || strings.Contains(screen, "t_order_id_xxxxx ON") || !strings.Contains(screen, "│ …   ") {
		t.Errorf("cut at %d rows:\n%s", ddlColumnRows, screen)
	}
	a.Update(tea.MouseWheelMsg{X: 80, Y: 20, Button: tea.MouseWheelDown})
	if a.ddl.top != 0 {
		t.Errorf("wheel: %d", a.ddl.top)
	}
	a.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	if a.ddl != nil {
		t.Error("a resize: the box is under a cell that has moved")
	}
}

// fH shows the table's DDL in a box in the middle; j / k, C-d / C-u, gg / G and
// the wheel scroll it, esc and a click outside close it, no other key does
// anything; a DDL not read yet shows … and is fetched (F9.8).
func TestDDLTablePopup(t *testing.T) {
	long := ddlOrderWith + "\n" + strings.Repeat("CREATE INDEX i ON public.t_order USING btree (id);\n", 30) + "CREATE INDEX the_last ON public.t_order USING btree (note);"
	a, _ := ddlApp(t, long)
	keys(t, a, "fH")
	if a.ddl == nil || a.ddl.col != "" {
		t.Fatalf("popup %+v", a.ddl)
	}
	box := a.ddlRect(long)
	rows := box.Dy() - 2
	if box.Dx() != 160*2/3 || rows != 44*2/3-2 {
		t.Errorf("box %v", box)
	}
	golden.RequireEqual(t, a.render().String())
	feed(t, a, "jj")
	if a.ddl.top != 2 {
		t.Errorf("jj: %d", a.ddl.top)
	}
	feed(t, a, "k<C-d>")
	if a.ddl.top != 1+rows/2 {
		t.Errorf("k C-d: %d", a.ddl.top)
	}
	feed(t, a, "G")
	lines := strings.Count(long, "\n") + 1
	if a.ddl.top != lines-rows || !strings.Contains(a.render().String(), "the_last") {
		t.Errorf("G: %d of %d", a.ddl.top, lines)
	}
	feed(t, a, "j<C-d>")
	if a.ddl.top != lines-rows {
		t.Errorf("past the end: %d", a.ddl.top)
	}
	feed(t, a, "gg")
	if a.ddl.top != 0 {
		t.Errorf("gg: %d", a.ddl.top)
	}
	feed(t, a, "<C-u>")
	if a.ddl.top != 0 {
		t.Errorf("above the top: %d", a.ddl.top)
	}
	a.Update(tea.MouseWheelMsg{X: 80, Y: 20, Button: tea.MouseWheelDown})
	if a.ddl.top != wheelStep {
		t.Errorf("wheel: %d", a.ddl.top)
	}
	// nothing else works while it is up
	feed(t, a, "<C-p>:?<Space>c")
	if a.palette != nil || a.keyHelp != nil || a.ddl == nil || len(a.sess.Windows) != 1 {
		t.Fatalf("a key got through: palette %v, help %v", a.palette != nil, a.keyHelp != nil)
	}
	a.run("palette.open", 0)
	if a.palette != nil {
		t.Error("an action got through")
	}
	feed(t, a, "<Esc>")
	if a.ddl != nil {
		t.Error("esc")
	}
	keys(t, a, "fH")
	a.View()
	click(a, a.window().Min)
	if a.ddl != nil {
		t.Error("a click outside")
	}
}

// A result's grid has no table: neither popup opens (F9.8).
func TestDDLPopupNeedsATable(t *testing.T) {
	r := db.Result{Cols: []db.Col{{Name: "n", Type: "int4"}}, Rows: [][]db.Val{{{S: "1"}}}}
	b, _ := inRun(t, &execDB{res: map[string]db.Result{"select 1": r}}, "select 1;")
	press(t, b, "<CR>")
	b.win().focus(b.win().Result.ID)
	for _, act := range []string{"grid.ddl.table", "grid.ddl.column"} {
		if cmd := b.run(act, 0); cmd != nil || b.ddl != nil {
			t.Errorf("%s on a result: %v, popup %v", act, cmd != nil, b.ddl != nil)
		}
	}
}

// A DDL not read yet is fetched, shown as … till it is in; one cached is not
// fetched again (F9.8).
func TestDDLPopupFetches(t *testing.T) {
	a, tab := ddlApp(t, "")
	c := a.sess.Conns[0]
	cmd := keys(t, a, "fH")
	if _, asked := c.ddl[idOf(tab.table)]; !asked || cmd == nil {
		t.Fatalf("not asked for: %v", cmd != nil)
	}
	if text, _, _ := a.ddlContent(); text != "…" {
		t.Errorf("before it is in: %q", text)
	}
	a.Update(ddlMsg{conn: c, table: tab.table, text: ddlOrderWith})
	if text, _, _ := a.ddlContent(); !strings.HasPrefix(text, "create table public.t_order") {
		t.Errorf("after: %q", text)
	}
	feed(t, a, "<Esc>")
	if cmd := a.openDDL(false); cmd != nil {
		t.Error("asked again")
	}
	// a failure is its message
	c.ddl[idOf(tab.table)] = ddlText{got: true, err: "boom"}
	if text, colors, _ := a.ddlContent(); text != "boom" || colors != nil {
		t.Errorf("an error: %q", text)
	}
}
