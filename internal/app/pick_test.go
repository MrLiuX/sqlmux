package app

import (
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

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
	c.all = catalogAll{schemas: slices.Clone(c.Schemas), dbs: slices.Clone(c.Databases)}
	f.schemas = slices.Clone(c.Schemas)
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
	if last := f.only[len(f.only)-1]; !slices.Equal(last, []string{"public"}) {
		t.Errorf("the catalog was asked for %v", f.only)
	}
	if got := treeTexts(a); strings.Contains(got, "agentable") || !strings.Contains(got, "public") {
		t.Errorf("the tree:\n%s", got)
	}
	ns, _ := a.treeNodes()
	if i := slices.IndexFunc(ns, func(n node) bool { return n.kind == nodeDatabase && n.Text == "base" }); ns[i].Note != "1/"+strconv.Itoa(len(c.all.schemas)) {
		t.Errorf("the database's note %q", ns[i].Note)
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
		want    []string
	}{
		{nil, true, "d", nil},
		{[]string{"d.a", "e.b", "d.c"}, true, "d", []string{"a", "c"}},
		{[]string{"e.b"}, true, "d", nil}, // none of d's: all of d's
		{[]string{"my.db.a"}, true, "my.db", []string{"a"}},
		{[]string{"b"}, false, "", []string{"b"}},
	} {
		if got := pickedSchemas(tc.setting, tc.pg, tc.db, all); !slices.Equal(got, tc.want) || (got == nil) != (tc.want == nil) {
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
// names (F17.13).
func TestPickMySQLSchemas(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	a, c, f := withDatabases(t)
	c.Spec.Engine, c.Databases = "mysql", nil
	if err := config.AppendConnection(config.Connection{Name: c.Spec.Name, Engine: "mysql", DSN: "u:p@tcp(h:3306)/base"}); err != nil {
		t.Fatal(err)
	}
	c.all = catalogAll{schemas: slices.Clone(c.Schemas)}
	f.schemas = slices.Clone(c.Schemas)
	treeTo(t, a, c.Spec.Name)
	a.run("tree.pick", 0)
	if p := a.cols.pick; p == nil || p.dbs || !slices.Equal(p.names, c.Schemas) {
		t.Fatalf("the list: %+v", a.cols)
	}
	feed(t, a, "A/public<CR><Space><Esc>")
	_, cmd := a.Update(teaKey("<Esc>"))
	answered(a, cmd)
	if got := connectionsText(t); !strings.Contains(got, `schemas = ["public"]`) {
		t.Errorf("the file:\n%s", got)
	}
	if last := f.only[len(f.only)-1]; !slices.Equal(last, []string{"public"}) {
		t.Errorf("the catalog was asked for %v", f.only)
	}
}
