package app

import (
	"slices"
	"strings"
	"testing"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/exp/golden"

	"sqlmux/internal/db"
	"sqlmux/internal/sqlkit"
	"sqlmux/internal/ui"
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
	// names that are alike: PG's key and index are no reserved words, "ID" and
	// id are two columns, "x.y" is one name, a string is no name, a comment runs
	// over lines, a constraint may be named unique_
	tricky := `create table public.k (
  key integer not null generated always as identity,
  index text,
  "ID" integer,
  id integer,
  y integer,
  "x.y" integer,
  a integer,
  data jsonb,
  constraint unique_a CHECK ((a > 0)),
  constraint k_pkey PRIMARY KEY (key),
  constraint k_id_key UNIQUE (id)
);
CREATE INDEX k_expr ON public.k USING btree (((data ->> 'id'::text)));
CREATE INDEX k_data ON public.k USING btree (data);
CREATE INDEX id ON public.k USING btree (index);
CREATE INDEX k_xy ON public.k USING btree ("x.y");
comment on column public.k.y is 'first line
  key (y) second line';
comment on column public.k."x.y" is 'the dotted one';
comment on column public.k."ID" is 'the upper one';`
	my := "CREATE TABLE `t_sku` (\n  `id` bigint NOT NULL AUTO_INCREMENT COMMENT 'the id',\n  `code` varchar(20) NOT NULL,\n  `title` varchar(80) DEFAULT NULL,\n  `key` int DEFAULT NULL,\n" +
		"  PRIMARY KEY (`id`),\n  UNIQUE KEY `t_sku_code` (`code`),\n  KEY `t_sku_title` (`title`(10),`code`),\n  CONSTRAINT `fk` FOREIGN KEY (`code`) REFERENCES `x` (`code`)\n) ENGINE=InnoDB;"
	view := "create view public.v as\n SELECT id,\n    status\n   FROM t;"
	for _, c := range []struct {
		name, ddl, col string
		d              sqlkit.Dialect
		cat            db.Column
		want           []string
	}{
		{"pg title", pg, "title", sqlkit.PG, db.Column{}, []string{
			"title text not null default 'x'::text",
			"CREATE UNIQUE INDEX t_sku_lower_title ON public.t_sku USING btree (lower(title));",
			"CREATE INDEX t_sku_title_code ON public.t_sku USING btree (title, code) INCLUDE (price);",
			"comment on column public.t_sku.title is 'what it is called';"}},
		{"pg code, a primary key", pg, "code", sqlkit.PG, db.Column{}, []string{
			"code text not null",
			"constraint t_sku_pkey PRIMARY KEY (code)",
			"CREATE INDEX t_sku_title_code ON public.t_sku USING btree (title, code) INCLUDE (price);"}}, // not code_x's
		{"pg quoted", pg, "B c", sqlkit.PG, db.Column{}, []string{`"B c" integer`, `comment on column public.t_sku."B c" is 'quoted';`}},
		{"pg alone", pg, "price", sqlkit.PG, db.Column{}, []string{"price numeric(10,2)", "CREATE INDEX t_sku_title_code ON public.t_sku USING btree (title, code) INCLUDE (price);"}},
		{"pg column key, not reserved", tricky, "key", sqlkit.PG, db.Column{}, []string{"key integer not null generated always as identity", "constraint k_pkey PRIMARY KEY (key)"}},
		{"pg column index", tricky, "index", sqlkit.PG, db.Column{}, []string{"index text", "CREATE INDEX id ON public.k USING btree (index);"}}, // not the index called id's
		{"pg id, not ID", tricky, "id", sqlkit.PG, db.Column{}, []string{"id integer", "constraint k_id_key UNIQUE (id)"}},                       // not 'id' in a string
		{"pg ID, not id", tricky, "ID", sqlkit.PG, db.Column{}, []string{`"ID" integer`, `comment on column public.k."ID" is 'the upper one';`}},
		{"pg y, not x.y; a comment over lines", tricky, "y", sqlkit.PG, db.Column{}, []string{"y integer", "comment on column public.k.y is 'first line\n  key (y) second line';"}},
		{"pg x.y", tricky, "x.y", sqlkit.PG, db.Column{}, []string{`"x.y" integer`, `CREATE INDEX k_xy ON public.k USING btree ("x.y");`, `comment on column public.k."x.y" is 'the dotted one';`}},
		{"pg a, a check called unique_ is no key", tricky, "a", sqlkit.PG, db.Column{}, []string{"a integer"}},
		{"pg data, a function of it", tricky, "data", sqlkit.PG, db.Column{}, []string{"data jsonb", "CREATE INDEX k_expr ON public.k USING btree (((data ->> 'id'::text)));", "CREATE INDEX k_data ON public.k USING btree (data);"}},
		{"mysql", my, "code", sqlkit.MySQL, db.Column{}, []string{"`code` varchar(20) NOT NULL", "UNIQUE KEY `t_sku_code` (`code`)", "KEY `t_sku_title` (`title`(10),`code`)"}},
		{"mysql id", my, "id", sqlkit.MySQL, db.Column{}, []string{"`id` bigint NOT NULL AUTO_INCREMENT COMMENT 'the id'", "PRIMARY KEY (`id`)"}},
		{"mysql title, a prefix length", my, "title", sqlkit.MySQL, db.Column{}, []string{"`title` varchar(80) DEFAULT NULL", "KEY `t_sku_title` (`title`(10),`code`)"}},
		{"mysql column key, quoted", my, "key", sqlkit.MySQL, db.Column{}, []string{"`key` int DEFAULT NULL"}},
		{"a view's column: the catalog's", view, "status", sqlkit.PG, db.Column{Type: "text", NotNull: true, Default: "'x'"}, []string{"status text not null default 'x'"}},
		{"a view's column that is an identity's", view, "id", sqlkit.PG, db.Column{Type: "integer", NotNull: true, Default: "generated always as identity"}, []string{"id integer not null generated always as identity"}},
		{"an index on a table called as the column", "create table public.t (\n  code integer\n);\nCREATE INDEX t_code ON public.id USING btree (code);", "id", sqlkit.PG, db.Column{Type: "integer"}, []string{"id integer"}},
	} {
		if got := ddlColumnLines(c.ddl, c.col, c.cat, c.d); !slices.Equal(got, c.want) {
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
	v, _ := a.ddlContent()
	if v.Lines() <= ddlColumnRows {
		t.Fatalf("not many lines: %q", v.Text)
	}
	screen := a.render().String()
	if !strings.Contains(screen, "t_order_id_xxxx ON") || strings.Contains(screen, "t_order_id_xxxxx ON") || !strings.Contains(screen, "│ …   ") {
		t.Errorf("cut at %d rows:\n%s", ddlColumnRows, screen)
	}
	// the … is the dim of the view's, not a color of the SQL it stands for
	f := a.render()
	for y, l := range strings.Split(f.String(), "\n") {
		if strings.Contains(l, "│ …   ") {
			x := strings.Index(l, "│ …   ")
			if got := f.Buf.CellAt(len([]rune(l[:x]))+2, y).Style.Fg; got != a.theme.Dim {
				t.Errorf("the …: %v, want dim", got)
			}
			break
		}
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
	box := a.ddlRect(ui.TextView{Text: long})
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

// R drops the DDL cache: a popup that is open asks for it again with the next
// message, and shows it once it is in, however often that happens (F9.8).
func TestDDLPopupAsksAgainAfterTheCacheGoes(t *testing.T) {
	a, tab := ddlApp(t, ddlOrderWith)
	c := a.sess.Conns[0]
	keys(t, a, "fh")
	c.dropCols()
	if _, asked := c.ddl[idOf(tab.table)]; asked {
		t.Fatal("the cache is still there")
	}
	if _, cmd := a.Update(tea.MouseMotionMsg{X: 1, Y: 1}); cmd == nil {
		t.Fatal("no request")
	}
	if _, asked := c.ddl[idOf(tab.table)]; !asked {
		t.Error("not asked for")
	}
	if v, _ := a.ddlContent(); v.Text != "…" {
		t.Errorf("before it is in: %q", v.Text)
	}
	a.Update(ddlMsg{conn: c, table: tab.table, text: ddlOrderWith})
	if v, _ := a.ddlContent(); !strings.Contains(v.Text, "id bigint not null") {
		t.Errorf("after: %q", v.Text)
	}
	// and again, as often as it goes
	c.dropCols()
	if _, cmd := a.Update(tea.MouseMotionMsg{X: 2, Y: 1}); cmd == nil {
		t.Error("not asked the second time")
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
	if v, _ := a.ddlContent(); v.Text != "…" || v.Fg != a.theme.Dim {
		t.Errorf("before it is in: %q in %v, want dim", v.Text, v.Fg)
	}
	a.Update(ddlMsg{conn: c, table: tab.table, text: ddlOrderWith})
	if v, _ := a.ddlContent(); !strings.HasPrefix(v.Text, "create table public.t_order") {
		t.Errorf("after: %q", v.Text)
	}
	feed(t, a, "<Esc>")
	if cmd := a.openDDL(false); cmd != nil {
		t.Error("asked again")
	}
	// a failure is its message
	c.ddl[idOf(tab.table)] = ddlText{got: true, err: "boom"}
	if v, _ := a.ddlContent(); v.Text != "boom" || v.Colors != nil {
		t.Errorf("an error: %q", v.Text)
	}
}

// The column's box is under the current cell, or over it when there is no room
// below, edited or not (F9.8).
func TestDDLColumnPopupPlace(t *testing.T) {
	for _, h := range []int{45, 14} {
		a, tab := ddlApp(t, ddlOrderWith)
		a.Update(tea.WindowSizeMsg{Width: 160, Height: h})
		feed(t, a, "jjjl")
		p := a.focused()
		_, g, area, _ := a.gridOf(p)
		cell := g.CellRect(area)
		if cell.Empty() || tab.row != 3 {
			t.Fatalf("h %d: cell %v at row %d", h, cell, tab.row)
		}
		keys(t, a, "fh")
		v, _ := a.ddlContent()
		box := a.ddlRect(v)
		below := cell.Max.Y+1+min(v.Lines(), ddlColumnRows)+1 <= a.window().Max.Y
		switch {
		case below && (box.Min.X != cell.Min.X || box.Min.Y != cell.Max.Y):
			t.Errorf("h %d: box %v, cell %v: not right under it", h, box, cell)
		case !below && box.Max.Y != cell.Min.Y:
			t.Errorf("h %d: box %v, cell %v: not right over it", h, box, cell)
		}
		if h == 14 && below {
			t.Error("the case with no room below is not a case")
		}
	}
}

// The DDL is in SQL's colors in the table's box and the column's, and a view
// scrolled past some lines is in them still (F9.8, F9.7).
func TestDDLPopupColors(t *testing.T) {
	long := ddlOrderWith + "\n" + strings.Repeat("CREATE INDEX i ON public.t_order USING btree (id);\n", 30) + "CREATE INDEX the_last ON public.t_order USING btree (status);"
	a, _ := ddlApp(t, long)
	keys(t, a, "fH")
	fg := func(word string) any { // the first cell of the first row that has word
		f := a.render()
		first, _ := utf8.DecodeRuneInString(word)
		for y, l := range strings.Split(f.String(), "\n") {
			if x := strings.Index(l, word); x >= 0 {
				if c := f.Buf.CellAt(len([]rune(l[:x])), y); c.String() == string(first) {
					return c.Style.Fg
				}
			}
		}
		return nil
	}
	if got := fg("create table"); got != a.theme.Keyword {
		t.Errorf("the keyword: %v", got)
	}
	if got := fg("status order_status"); got != a.theme.SQLColumn { // a column of the table, the catalog knows it
		t.Errorf("a column's name: %v, want %v", got, a.theme.SQLColumn)
	}
	feed(t, a, "G") // the lines scrolled past have their bytes: the colors are of these
	if got := fg("CREATE INDEX the_last"); got != a.theme.Keyword {
		t.Errorf("scrolled, the keyword: %v", got)
	}
	feed(t, a, "<Esc>")
	keys(t, a, "fh") // id
	if got := fg("CREATE INDEX i ON"); got != a.theme.Keyword {
		t.Errorf("the column's: %v", got)
	}
}

// C-c on a column's box only closes it, the wheel and a paste close it and go
// on as they would have (F9.8).
func TestDDLColumnPopupClosesWithTheNextThing(t *testing.T) {
	a, tab := ddlApp(t, ddlOrderWith)
	keys(t, a, "fh")
	if feed(t, a, "<C-c>"); a.ddl != nil || a.toast != "" {
		t.Errorf("C-c: popup %v, toast %q", a.ddl != nil, a.toast)
	}
	keys(t, a, "fh")
	row := tab.top
	a.Update(tea.MouseWheelMsg{X: 90, Y: 20, Button: tea.MouseWheelDown})
	if a.ddl != nil {
		t.Error("the wheel left it")
	}
	if tab.top < row {
		t.Errorf("the wheel was lost: top %d", tab.top)
	}
	keys(t, a, "fh")
	a.paste("77")
	if a.ddl != nil || tab.cell == nil || tab.cell.in.Text != "77" {
		t.Errorf("a paste: popup %v, cell %+v", a.ddl != nil, tab.cell)
	}
	// esc is only the closing: an error bar stays
	b, btab := ddlApp(t, ddlOrderWith)
	btab.bar = &errorBar{ui.ErrorBar{First: ui.Note{Head: "failed"}}, "fetch"}
	keys(t, b, "fh")
	feed(t, b, "<Esc>")
	if b.ddl != nil || btab.bar == nil {
		t.Errorf("esc: popup %v, bar %v", b.ddl != nil, btab.bar != nil)
	}
}

// The table's DDL needs a table tab and no page read; the column's needs a
// column to stand on (F9.8).
func TestDDLPopupNeedsNoPage(t *testing.T) {
	a := configured(t, 160, 45, "[keys.grid]\nfh = \"grid.ddl.column\"\nfH = \"grid.ddl.table\"")
	a.removePane(2)
	table, _, _ := ordersTable(1)
	a.openTable(a.sess.Conns[0], table) // the page is not in
	tab := dataOf(a.focused())
	if tab == nil || tab.page.Cols != nil {
		t.Fatalf("tab %+v", tab)
	}
	if keys(t, a, "fh"); a.ddl != nil {
		t.Error("a column's with no page")
	}
	if keys(t, a, "fH"); a.ddl == nil || a.ddl.col != "" {
		t.Error("the table's needs no page")
	}
}

// What the table's box does not take it must not leak to: a click inside it is
// not outside, the keys of an input under it are none of its, and its box is 40
// by 8 where the window is small (F9.8).
func TestDDLTablePopupHolds(t *testing.T) {
	a, tab := ddlApp(t, ddlOrderWith)
	feed(t, a, "/id") // the WHERE has the keys
	if tab.typing != "where" {
		t.Fatal("not typing")
	}
	a.run("grid.ddl.table", 0) // from the palette, say
	if a.ddl == nil {
		t.Fatal("no popup")
	}
	feed(t, a, "xyz")
	if tab.where.Text != "id" {
		t.Errorf("the WHERE: %q", tab.where.Text)
	}
	a.View()
	v, _ := a.ddlContent()
	box := a.ddlRect(v)
	click(a, box.Min.Add(uv.Pos(5, 3)))
	if a.ddl == nil {
		t.Error("a click inside closed it")
	}
	a.Update(tea.WindowSizeMsg{Width: 45, Height: 11})
	if box := a.ddlRect(v); box.Dx() != 40 || box.Dy() != 8 {
		t.Errorf("a small window: %v", box)
	}
	// an error with lines is one
	a.sess.Conns[0].ddl[idOf(tab.table)] = ddlText{got: true, err: "a\nb"}
	if v, _ := a.ddlContent(); v.Text != "a b" {
		t.Errorf("an error over lines: %q", v.Text)
	}
}

// The top kept is held to the window it is in now: after the window grew, the
// first k moves the view (F9.8).
func TestDDLTopAfterResize(t *testing.T) {
	long := ddlOrderWith + "\n" + strings.Repeat("CREATE INDEX i ON public.t_order USING btree (id);\n", 30) + "CREATE INDEX the_last ON public.t_order USING btree (note);"
	a, _ := ddlApp(t, long)
	a.Update(tea.WindowSizeMsg{Width: 160, Height: 20})
	keys(t, a, "fH")
	feed(t, a, "G")
	big := a.ddl.top
	a.Update(tea.WindowSizeMsg{Width: 160, Height: 45})
	v, _ := a.ddlContent()
	rows := ui.TextBox{}.Inner(a.ddlRect(v)).Dy()
	want := v.Lines() - rows // the whole of what is below the first rows
	feed(t, a, "k")
	if a.ddl.top != want-1 || big <= want {
		t.Errorf("k after a resize: top %d, want %d (it was %d)", a.ddl.top, want-1, big)
	}
}
