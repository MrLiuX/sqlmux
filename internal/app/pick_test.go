package app

import (
	"image/color"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/exp/golden"

	"github.com/kackerx/sqlmux/internal/config"
)

// pickSetup is withDatabases with a connections.toml that has its connection, the server's
// databases and schemas as the catalog read them.
func pickSetup(t *testing.T) (*App, *Conn, *fakePG) {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	a, c, f := withDatabases(t)
	if err := config.AppendConnection(config.Connection{Name: c.Spec.Name, Engine: "postgres", DSN: "postgres://u@h/base"}); err != nil {
		t.Fatal(err)
	}
	f.schemas = slices.Clone(c.Schemas)
	c.all = catalogAll{}
	answered(a, a.loadCatalog(c)) // what the server has is what the load says, not set by hand
	return a, c, f
}

func connectionsText(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(config.Dir(), "connections.toml"))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// "选择库…" on a PG connection lists its databases, the DSN's on and kept so; what is
// off goes into connections.toml as databases when the list closes, and is no more in the
// tree, the connection saying how many are shown; all on again is no line (F17.13).
func TestPickDatabases(t *testing.T) {
	a, c, _ := pickSetup(t)
	if !slices.Equal(c.all.dbs, []string{"alpha", "base", "zeta"}) || len(c.all.schemas) == 0 || c.all.keep != "public" {
		t.Fatalf("what the load read: %+v", c.all)
	}
	if ns, _ := a.treeNodes(); ns[0].Note != "" { // all of them: no note
		t.Errorf("the connection's note %q before any is left out", ns[0].Note)
	}
	treeTo(t, a, c.Spec.Name)
	a.run("tree.pick", 0)
	p := a.cols.pick
	if p == nil || !p.dbs || !slices.Equal(p.names, []string{"alpha", "base", "zeta"}) || slices.Contains(p.on, false) || p.fixed != "base" {
		t.Fatalf("the list: %+v", a.cols)
	}
	feed(t, a, "<Space>")   // alpha off
	feed(t, a, "jj<Space>") // zeta off
	feed(t, a, "k<Space>")  // base: the DSN's, stays on
	if !slices.Equal(p.on, []bool{false, true, false}) {
		t.Fatalf("on: %v", p.on)
	}
	_, cmd := a.Update(teaKey("<Esc>"))
	if a.cols != nil {
		t.Fatal("esc leaves the list open")
	}
	answered(a, cmd)
	if got := connectionsText(t); !strings.Contains(got, `databases = ["base"]`) {
		t.Errorf("the file:\n%s", got)
	}
	if got := treeTexts(a); strings.Contains(got, "alpha") || strings.Contains(got, "zeta") || !strings.Contains(got, "base") {
		t.Errorf("the tree:\n%s", got)
	}
	ns, _ := a.treeNodes()
	if i := slices.IndexFunc(ns, func(n node) bool { return n.kind == nodeConn }); ns[i].Note != "1/3" {
		t.Errorf("the connection's note %q", ns[i].Note)
	}
	if c.Spec.Databases == nil || !slices.Equal(c.Spec.Databases, []string{"base"}) {
		t.Errorf("the connection's spec: %v", c.Spec.Databases)
	}
	treeTo(t, a, c.Spec.Name)
	a.run("tree.pick", 0)
	feed(t, a, "a<Esc>") // all on again
	if a.cols != nil {
		t.Fatal("still open")
	}
	if got := connectionsText(t); strings.Contains(got, "databases") {
		t.Errorf("all on is no line:\n%s", got)
	}
}

// "选择 schema…" on a PG database writes `db.schema` entries for that database only, those
// of other databases as the file has them; the catalog is then read for the picked ones
// alone, and the database says how many (F17.13).
func TestPickSchemas(t *testing.T) {
	a, c, f := pickSetup(t)
	was, _ := config.ReadConnection(c.Spec.Name) // another database's entry, written meanwhile
	next := was
	next.Schemas = []string{"zeta.sales"}
	if _, err := config.UpdateConnection(was.Name, was, next); err != nil {
		t.Fatal(err)
	}
	treeTo(t, a, "base")
	a.run("tree.pick", 0)
	p := a.cols.pick
	if p == nil || p.dbs || p.conn != c || !slices.Equal(p.names, c.Schemas) {
		t.Fatalf("the list: %+v", a.cols)
	}
	feed(t, a, "A")           // none: every one off
	feed(t, a, "/public<CR>") // the filter
	feed(t, a, "<Space>")     // public on
	feed(t, a, "<Esc>")       // clears the filter
	if a.cols == nil {
		t.Fatal("esc with a filter closes the list")
	}
	_, cmd := a.Update(teaKey("<Esc>"))
	if a.cols != nil {
		t.Fatal("not closed")
	}
	answered(a, cmd)
	if got := connectionsText(t); !strings.Contains(got, `schemas = ["zeta.sales", "base.public"]`) {
		t.Errorf("the file:\n%s", got)
	}
	for name, asked := range map[string][][]string{"tables": f.only, "functions": f.fonly, "sequences": f.sonly} {
		if last := asked[len(asked)-1]; !slices.Equal(last, []string{"public"}) {
			t.Errorf("the catalog's %s were asked for %v", name, asked)
		}
	}
	if got := treeTexts(a); strings.Contains(got, "agentable") || !strings.Contains(got, "public") {
		t.Errorf("the tree:\n%s", got)
	}
	ns, _ := a.treeNodes()
	if i := slices.IndexFunc(ns, func(n node) bool { return n.kind == nodeDatabase && n.Text == "base" }); ns[i].Note != "1/"+strconv.Itoa(len(c.all.schemas)) {
		t.Errorf("the database's note %q", ns[i].Note)
	}
	// all of them again: the line goes
	treeTo(t, a, "base")
	a.run("tree.pick", 0)
	feed(t, a, "a")
	_, cmd = a.Update(teaKey("<Esc>"))
	answered(a, cmd)
	if got := connectionsText(t); strings.Contains(got, "base.public") || !strings.Contains(got, `schemas = ["zeta.sales"]`) {
		t.Errorf("all picked again:\n%s", got)
	}
	// nothing changed: nothing is written and nothing is read
	before, reads := connectionsText(t), len(f.only)
	treeTo(t, a, "base")
	a.run("tree.pick", 0)
	if _, cmd := a.Update(teaKey("<Esc>")); cmd != nil || connectionsText(t) != before || len(f.only) != reads {
		t.Errorf("a list closed as it was opened writes or reads (%v)", cmd != nil)
	}
}

// The databases and schemas the catalog is limited to: `db.schema` for the database it
// is read in, plain names for MySQL, all when the setting names none of this database's,
// the DSN's database whatever the setting says.
func TestPickedSchemasAndDatabases(t *testing.T) {
	all := []string{"a", "b", "c"}
	for _, tc := range []struct {
		setting []string
		pg      bool
		db      string
		keep    string
		want    []string
	}{
		{nil, true, "d", "a", nil},
		{[]string{"d.a", "e.b", "d.c"}, true, "d", "", []string{"a", "c"}},
		{[]string{"e.b"}, true, "d", "b", nil}, // none of d's: all of d's
		{[]string{"my.db.a"}, true, "my.db", "", []string{"a"}},
		{[]string{"b"}, false, "", "", []string{"b"}},
		{[]string{"d.c"}, true, "d", "a", []string{"a", "c"}}, // the schema names go to stays, written or not
	} {
		if got := pickedSchemas(tc.setting, tc.pg, tc.db, all, tc.keep); !slices.Equal(got, tc.want) || (got == nil) != (tc.want == nil) {
			t.Errorf("%v in %q: %v, want %v", tc.setting, tc.db, got, tc.want)
		}
	}
	if got := pickedDatabases([]string{"x"}, "dsn", []string{"dsn", "x", "y"}); !slices.Equal(got, []string{"dsn", "x"}) {
		t.Errorf("databases %v", got)
	}
	if got := pickedDatabases(nil, "dsn", []string{"dsn", "y"}); !slices.Equal(got, []string{"dsn", "y"}) {
		t.Errorf("no setting: %v", got)
	}
}

// The list over the tree, a screen of it (F17.13).
func TestGoldenPick(t *testing.T) {
	a, c, _ := pickSetup(t)
	treeTo(t, a, c.Spec.Name)
	a.run("tree.pick", 0)
	feed(t, a, "<Space>") // alpha off
	golden.RequireEqual(t, a.render().String())
}

// "选择 schema…" on a MySQL connection lists its schemas (its databases) and writes plain
// names; A leaves the DSN's database, which cannot go off, and the connection says 1/N
// (F17.13).
func TestPickMySQLSchemas(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	a, c, f := withDatabases(t)
	c.Spec.Engine, c.Databases = "mysql", nil
	if err := config.AppendConnection(config.Connection{Name: c.Spec.Name, Engine: "mysql", DSN: "u:p@tcp(h:3306)/base"}); err != nil {
		t.Fatal(err)
	}
	f.schemas = slices.Clone(c.Schemas)
	answered(a, a.loadCatalog(c))
	treeTo(t, a, c.Spec.Name)
	a.run("tree.pick", 0)
	p := a.cols.pick
	if p == nil || p.dbs || !slices.Equal(p.names, c.Schemas) || p.fixed != "public" {
		t.Fatalf("the list: %+v", a.cols)
	}
	feed(t, a, "A")
	if on := slices.Index(p.on, true); on != slices.Index(p.names, "public") || slices.Contains(p.on[on+1:], true) {
		t.Fatalf("A leaves %v", p.on)
	}
	_, cmd := a.Update(teaKey("<Esc>"))
	answered(a, cmd)
	if got := connectionsText(t); !strings.Contains(got, `schemas = ["public"]`) {
		t.Errorf("the file:\n%s", got)
	}
	if last := f.only[len(f.only)-1]; !slices.Equal(last, []string{"public"}) {
		t.Errorf("the catalog was asked for %v", f.only)
	}
	if ns, _ := a.treeNodes(); ns[0].Note != "1/"+strconv.Itoa(len(c.all.schemas)) {
		t.Errorf("the connection's note %q", ns[0].Note)
	}
}

// The schema names go to is always loaded, whatever the setting names; and a list that
// is left with none, which only happens with no such schema, writes nothing (F17.13).
func TestPickKeepsTheDefaultSchema(t *testing.T) {
	a, c, f := pickSetup(t)
	c.Spec.Schemas = []string{"base.agentable"}
	answered(a, a.loadCatalog(c))
	if last := f.only[len(f.only)-1]; !slices.Equal(last, []string{"agentable", "public"}) {
		t.Errorf("the setting names agentable: asked for %v", last)
	}
	before := connectionsText(t)
	treeTo(t, a, "base")
	a.run("tree.pick", 0)
	p := a.cols.pick
	p.fixed, p.on = "", make([]bool, len(p.names)) // nothing to keep, nothing on
	if _, cmd := a.Update(teaKey("<Esc>")); cmd != nil || connectionsText(t) != before {
		t.Errorf("a list with none on writes (%v)", cmd != nil)
	}
}

// PG with no schema of the search_path there keeps public.
func TestPickKeepsPublicWhenThereIsNoCurrentSchema(t *testing.T) {
	a, c, f := pickSetup(t)
	f.noCurrent = true
	answered(a, a.loadCatalog(c))
	if c.all.keep != "public" {
		t.Errorf("keep %q", c.all.keep)
	}
}

// Every session's connection of the name takes the pick, not the one it was made in alone.
func TestPickReachesEverySession(t *testing.T) {
	a, c, _ := pickSetup(t)
	other := newConn(c.Spec)
	a.sessions = append(a.sessions, bareSession("s2", []*Conn{other}, other))
	treeTo(t, a, c.Spec.Name)
	a.run("tree.pick", 0)
	feed(t, a, "<Space>")
	_, cmd := a.Update(teaKey("<Esc>"))
	answered(a, cmd)
	if !slices.Equal(other.Spec.Databases, []string{"base", "zeta"}) || !slices.Equal(c.Spec.Databases, []string{"base", "zeta"}) {
		t.Errorf("the other session's: %v, this one's: %v", other.Spec.Databases, c.Spec.Databases)
	}
}

// A connection of the command line is in no file: the pick is in memory and writes none.
func TestPickOfATempConnection(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	a, c, _ := withDatabases(t)
	c.Spec.Temp = true
	answered(a, a.loadCatalog(c))
	treeTo(t, a, c.Spec.Name)
	a.run("tree.pick", 0)
	feed(t, a, "<Space>")
	_, cmd := a.Update(teaKey("<Esc>"))
	answered(a, cmd)
	if _, err := os.Stat(filepath.Join(config.Dir(), "connections.toml")); err == nil {
		t.Error("a file was written")
	}
	if !slices.Equal(c.Spec.Databases, []string{"base", "zeta"}) {
		t.Errorf("the connection: %v", c.Spec.Databases)
	}
}

// A schema row ends in how many of its tables are loaded, which is for a schema that was
// loaded when the list opened, not one that is only on now.
func TestPickNoteIsOfWhatIsLoaded(t *testing.T) {
	a, c, _ := pickSetup(t)
	treeTo(t, a, "base")
	a.run("tree.pick", 0)
	p := a.cols.pick
	pub, ag := slices.Index(p.names, "public"), slices.Index(p.names, "agentable")
	if p.note(pub) != "1" || p.note(ag) != "1" && p.note(ag) != "0" {
		t.Fatalf("loaded: %q %q", p.note(pub), p.note(ag))
	}
	feed(t, a, "A") // only public stays: written, loaded again
	_, cmd := a.Update(teaKey("<Esc>"))
	answered(a, cmd)
	treeTo(t, a, "base")
	a.run("tree.pick", 0)
	p = a.cols.pick
	p.set(ag, true) // agentable is on now, not loaded
	if p.note(ag) != "" || p.note(pub) != "1" {
		t.Errorf("notes %q %q", p.note(ag), p.note(pub))
	}
	_ = c
}

// A click outside the list closes it as esc does: the change is written.
func TestPickClosesOnAClickOutside(t *testing.T) {
	a, c, _ := pickSetup(t)
	treeTo(t, a, c.Spec.Name)
	a.run("tree.pick", 0)
	feed(t, a, "<Space>") // alpha off
	a.View()
	_, cmd := a.Update(tea.MouseClickMsg{X: a.w - 1, Y: a.h - 1, Button: tea.MouseLeft})
	if a.cols != nil {
		t.Fatal("the list stays")
	}
	answered(a, cmd)
	if got := connectionsText(t); !strings.Contains(got, `databases = ["base", "zeta"]`) {
		t.Errorf("the file:\n%s", got)
	}
}

// COLS's keys that move a column mean nothing in a pick list: no table behind it, no
// panic, nothing moves.
func TestPickIgnoresColumnMoves(t *testing.T) {
	a, c, _ := pickSetup(t)
	treeTo(t, a, c.Spec.Name)
	a.run("tree.pick", 0)
	names := slices.Clone(a.cols.pick.names)
	feed(t, a, "J")
	feed(t, a, "K")
	if a.cols == nil || !slices.Equal(a.cols.pick.names, names) {
		t.Errorf("after J K: %+v", a.cols)
	}
}

// What has not read its catalog has nothing to pick from: it says so.
func TestPickNeedsTheCatalog(t *testing.T) {
	a, c, _ := pickSetup(t)
	c.State = connIdle
	treeTo(t, a, c.Spec.Name)
	a.run("tree.pick", 0)
	if a.cols != nil || a.toast == "" {
		t.Errorf("list %v, toast %q", a.cols != nil, a.toast)
	}
}

// The note of a database is dim: the border color of a Redis db's size would be another.
func TestPickNoteColor(t *testing.T) {
	a, c, _ := pickSetup(t)
	th := *a.theme
	th.Dim, th.Border = color.RGBA{R: 1, G: 2, B: 3, A: 255}, color.RGBA{R: 9, G: 8, B: 7, A: 255}
	a.theme = &th
	c.Schemas = c.Schemas[:1] // narrowed: the database says 1/N
	ns, _ := a.treeNodes()
	i := slices.IndexFunc(ns, func(n node) bool { return n.kind == nodeDatabase && n.Text == "base" })
	if ns[i].Note == "" || ns[i].NoteFg != th.Dim {
		t.Errorf("note %q in %v, want dim %v", ns[i].Note, ns[i].NoteFg, th.Dim)
	}
}

// The last schemas line of a block goes when all are picked again: the one that was
// written is the one that is taken out.
func TestPickSchemasThenAll(t *testing.T) {
	a, _, _ := pickSetup(t)
	for _, keys := range []string{"A/public<CR><Space><Esc>", "a"} {
		treeTo(t, a, "base")
		a.run("tree.pick", 0)
		feed(t, a, keys)
		_, cmd := a.Update(teaKey("<Esc>"))
		answered(a, cmd)
	}
	if got := connectionsText(t); strings.Contains(got, "schemas") {
		t.Errorf("the file:\n%s", got)
	}
	if got := treeTexts(a); !strings.Contains(got, "agentable") {
		t.Errorf("the tree:\n%s", got)
	}
}
