package app

import (
	"context"
	"net/url"
	"slices"
	"strings"
	"sync"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/exp/golden"

	"github.com/kackerx/sqlmux/internal/config"
	"github.com/kackerx/sqlmux/internal/db"
	"github.com/kackerx/sqlmux/internal/db/postgres"
	"github.com/kackerx/sqlmux/internal/ui"
)

// fakePG is PG's engine over no server: it dials the database its DSN names, as
// far as saying which, lists the databases alpha, base and zeta, and has a
// table t_x in public of each.
type fakePG struct {
	db.Engine
	mu    sync.Mutex
	dials []string // the database of each Connect
}

func (f *fakePG) Connect(_ context.Context, dsn, _ string, _ bool) (db.Conn, db.Info, error) {
	u, _ := url.Parse(dsn)
	name := strings.TrimPrefix(u.Path, "/")
	f.mu.Lock()
	f.dials = append(f.dials, name)
	f.mu.Unlock()
	return noDB{}, db.Info{Addr: "u@h:1", Database: name}, nil
}
func (*fakePG) Schemas(context.Context, db.Conn) ([]string, string, error) {
	return []string{"public"}, "public", nil
}
func (*fakePG) Tables(context.Context, db.Conn) ([]db.Table, error) {
	return []db.Table{{Schema: "public", Name: "t_x", Rows: 1}}, nil
}
func (*fakePG) Databases(context.Context, db.Conn) ([]string, error) {
	return []string{"alpha", "base", "zeta"}, nil
}

// withDatabases is a session of one connection to the database base, which
// the server has others besides, over fakePG.
func withDatabases(t *testing.T) (*App, *Conn, *fakePG) {
	t.Helper()
	f := &fakePG{Engine: postgres.Engine{}}
	was := engines["postgres"]
	engines["postgres"] = f
	t.Cleanup(func() { engines["postgres"] = was })
	a := sized(160, 45, "nerd")
	c := a.sess.Conns[0]
	c.Spec.DSN, c.DB, c.Databases = "postgres://u@h/base", "base", []string{"alpha", "base", "zeta"}
	c.eng = f
	a.win().focus(0)
	return a, c, f
}

// answered hands a what cmd, and what that brings in turn, answers: dialing
// and the catalog.
func answered(a *App, cmd tea.Cmd) {
	if cmd == nil {
		return
	}
	switch m := cmd().(type) {
	case tea.BatchMsg:
		for _, c := range m {
			answered(a, c)
		}
	case connected, catalogMsg:
		_, next := a.Update(m)
		answered(a, next)
	}
}

// A PG connection's tree has the databases between it and its schemas: the
// DSN's first and open, with the schemas the connection has, the others by name
// and closed; a connection that has no list is as it was (§7.8).
func TestDatabaseNodes(t *testing.T) {
	a, c, _ := withDatabases(t)
	got := strings.Split(treeTexts(a), "\n")
	want := []string{"▾ doraemon", "  ▾ base", "    ▸ agentable", "    ▾ public", "      ▾ Tables (14)"}
	if !slices.Equal(got[:5], want) {
		t.Fatalf("top:\n%s", strings.Join(got[:5], "\n"))
	}
	if i := slices.Index(got, "  ▸ alpha"); i < 0 || got[i+1] != "  ▸ zeta" || got[i+2] != "▾ 工作区" {
		t.Errorf("the others\n%s", strings.Join(got, "\n"))
	}
	ns, _ := a.treeNodes()
	var rows []string
	for i, n := range ns {
		if n.kind == nodeConn || n.kind == nodeDatabase {
			r := find(t, a, ui.Target{Kind: ui.KindNode, Pane: 0, I: i})
			rows = append(rows, ansi.Cut(strings.Split(a.render().String(), "\n")[r.Min.Y], r.Min.X, r.Max.X))
			if n.kind == nodeDatabase && (n.conn == c) == (n.Text != "base") {
				t.Errorf("%s is of %s", n.Text, n.conn.label())
			}
		}
	}
	golden.RequireEqual(t, strings.Join(rows, "\n")+"\n")

	c.Databases = nil
	if got := treeTexts(a); !strings.HasPrefix(got, "▾ doraemon\n  ▸ agentable\n  ▾ public") {
		t.Errorf("without a list:\n%s", got)
	}
}

// A database opens when it is opened in the tree, not before: its Main and Meta
// are dialed to it, its catalog read, its schemas below it and its tables
// opening as the DSN's do; the connection's databases are listed once. R on it
// reloads its catalog and no other's (F15.1).
func TestDatabaseDialsOnOpen(t *testing.T) {
	a, c, f := withDatabases(t)
	if len(f.dials) != 0 {
		t.Fatalf("dialed %v before any was opened", f.dials)
	}
	treeTo(t, a, "alpha")
	_, cmd := a.Update(teaKey("l"))
	answered(a, cmd)
	alpha := c.inDB("alpha")
	if !slices.Equal(f.dials, []string{"alpha", "alpha"}) || alpha.State != connReady || alpha.DB != "alpha" || alpha.Databases != nil {
		t.Fatalf("dials %v, %v in %q with databases %v", f.dials, alpha.State, alpha.DB, alpha.Databases)
	}
	if alpha.Addr != "u@h:1/alpha" || c.Addr != "pg@localhost:5432" {
		t.Errorf("addresses %q, %q", alpha.Addr, c.Addr)
	}
	got := treeTexts(a)
	if !strings.Contains(got, "  ▾ alpha\n    ▾ public\n      ▾ Tables (1)\n        ▸ t_x\n        Views (0)\n  ▸ zeta") {
		t.Fatalf("under alpha:\n%s", got)
	}
	treeTo(t, a, "t_x")
	if n, _ := a.treeNode(); n.conn != alpha || a.conn() != alpha {
		t.Errorf("t_x is of %s", n.conn.label())
	}
	_, cmd = a.Update(teaKey("R"))
	if m, ok := cmd().(catalogMsg); !ok || m.conn != alpha {
		t.Errorf("R reloads %T %v", m, m.conn.label())
	}
	treeTo(t, a, "zeta")
	if _, cmd = a.Update(teaKey("R")); cmd == nil {
		t.Fatal("R on a database not connected does nothing")
	}
	answered(a, cmd)
	if !slices.Equal(f.dials, []string{"alpha", "alpha", "zeta", "zeta"}) {
		t.Errorf("R on zeta dialed %v", f.dials)
	}
}

// With the tree filtering, a database that has no table kept is not shown, nor
// is the connection when none of its databases has one (§7.8).
func TestDatabaseFilter(t *testing.T) {
	a, c, _ := withDatabases(t)
	treeTo(t, a, "alpha")
	_, cmd := a.Update(teaKey("l"))
	answered(a, cmd)
	c.inDB("alpha").Tables = []db.Table{{Schema: "public", Name: "only_alpha"}}
	a.treeFilter()
	feed(t, a, "only_alpha")
	got := treeTexts(a)
	if !strings.Contains(got, "▾ doraemon\n  ▾ alpha\n    ▾ public\n      ▾ Tables (1)\n        ▸ only_alpha") || strings.Contains(got, "base") || strings.Contains(got, "zeta") {
		t.Errorf("filtered:\n%s", got)
	}
}

// The query's schema dropdown lists the databases first: another is where the
// query goes, once it is dialed; the one it is in goes on to its schemas. With
// no other database it is the schemas at once (F15.1).
func TestQueryDatabaseDrop(t *testing.T) {
	a, c, f := withDatabases(t)
	a.win().focus(2)
	q := queryOf(a.focused())
	a.run("query.schema", 0)
	items, current := a.dropItems()
	if a.drop.kind != dropDatabase || !slices.Equal(items, []string{"base", "alpha", "zeta"}) || current != 0 {
		t.Fatalf("%v %v of %v", a.drop.kind, current, items)
	}
	feed(t, a, "<CR>")
	if a.drop == nil || a.drop.kind != dropSchema {
		t.Fatalf("the database it is in: %+v", a.drop)
	}
	feed(t, a, "<Esc>")

	a.run("query.schema", 0)
	feed(t, a, "alpha")
	_, cmd := a.Update(teaKey("<CR>"))
	if a.drop != nil || q.conn != c || q.want == nil || q.want.Spec.Database != "alpha" {
		t.Fatalf("on %s, waiting for %v", q.conn.label(), q.want)
	}
	answered(a, cmd)
	if q.conn.Spec.Database != "alpha" || q.conn.State != connReady || q.want != nil || q.schema != "public" || !slices.Equal(f.dials, []string{"alpha", "alpha"}) {
		t.Fatalf("on %s (%v), schema %q, dials %v", q.conn.label(), q.conn.State, q.schema, f.dials)
	}
	if got := a.state.Queries[q.name]; got.Conn != "doraemon" || got.DB != "alpha" {
		t.Errorf("kept %+v", got)
	}
	a.run("query.schema", 0)
	if items, current = a.dropItems(); items[current] != "alpha" {
		t.Errorf("current %q of %v", items[current], items)
	}
	feed(t, a, "<Esc>")
	a.run("query.schema", 0)
	feed(t, a, "base<CR>") // back where it was: the connection itself
	if q.conn != c || a.state.Queries[q.name].DB != "" {
		t.Errorf("back on %s, kept %+v", q.conn.label(), a.state.Queries[q.name])
	}

	c.Databases = []string{"base"}
	a.run("query.schema", 0)
	if a.drop.kind != dropSchema {
		t.Errorf("one database: %v", a.drop.kind)
	}
}

// What a session keeps of where a tab is says the database when it is not the
// DSN's: the layout's tabs, a query file's place, a table's WHERE history;
// none of it is there for the DSN's. A layout, or a state, that says one
// puts the tab back on a connection to it, dialed as it needs (F15.1).
func TestDatabaseKept(t *testing.T) {
	a, c, _ := withDatabases(t)
	alpha := c.inDB("alpha")
	alpha.Schemas = []string{"public"}
	data := a.win().Root.A.Pane
	data.Tabs = []Tab{{Name: "t_x", Data: newDataTab(alpha, db.Table{Schema: "public", Name: "t_x"})}, {Name: "t_y", Data: newDataTab(c, db.Table{Schema: "public", Name: "t_y"})}}
	q := queryOf(a.win().Root.B.Pane)
	q.conn = alpha
	a.keepQuery(q)
	l := paneLayout(data)
	if l.Tabs[0].DB != "alpha" || l.Tabs[1].DB != "" {
		t.Errorf("layout tabs %+v", l.Tabs)
	}
	if lq := paneLayout(a.win().Root.B.Pane).Tabs[0]; lq.DB != "alpha" || lq.Connection != "doraemon" {
		t.Errorf("query tab %+v", lq)
	}
	if k := a.tableState(data.Tabs[0].Data); k == a.tableState(data.Tabs[1].Data) || a.state.Tables["doraemon/alpha/public.t_x"] != k || a.state.Tables["doraemon/public.t_y"] == nil {
		t.Errorf("table keys %v", slices.Collect(func(yield func(string) bool) {
			for k := range a.state.Tables {
				if !yield(k) {
					return
				}
			}
		}))
	}

	r := Restore([]config.Connection{c.Spec}, &config.Layout{Sessions: []config.LayoutSession{{Name: "s", Connection: "doraemon", Windows: []config.LayoutWindow{{
		Name: "w", Layout: &config.LayoutNode{Pane: 1}, Panes: []config.LayoutPane{{ID: 1, Tabs: []config.LayoutTab{
			{Kind: "table", Connection: "doraemon", DB: "alpha", Schema: "public", Table: "t_x"},
			{Kind: "table", Connection: "doraemon", Schema: "public", Table: "t_y"},
			{Kind: "query", Connection: "doraemon", DB: "zeta", Name: "query_7"},
		}}},
	}}}}})
	s := r.Sessions[0]
	tabs := s.Windows[0].Root.Pane.Tabs
	if len(tabs) != 3 {
		t.Fatalf("restored %d tabs: %v", len(tabs), r.Warnings)
	}
	if got := tabs[0].Data.conn; got.Spec.Database != "alpha" || got != s.Conns[0].inDB("alpha") || got.parent != s.Conns[0] {
		t.Errorf("t_x on %s", got.label())
	}
	if tabs[1].Data.conn != s.Conns[0] {
		t.Errorf("t_y on %s", tabs[1].Data.conn.label())
	}
	if w := tabs[2].Query.want; w == nil || w.Spec.Database != "zeta" || tabs[2].Query.conn != s.Conns[0] {
		t.Errorf("query_7 waits for %v", w)
	}
	if !slices.ContainsFunc(s.conns(), func(c *Conn) bool { return c.Spec.Database == "alpha" }) {
		t.Error("the session does not have the connection to alpha: its answers would be dropped")
	}

	a.state.Queries["query_9"] = config.QueryState{Conn: "doraemon", DB: "alpha", Schema: "public"}
	if cn, schema := a.queryPlace("query_9", c); cn != alpha || schema != "public" {
		t.Errorf("query_9 opens on %s in %q", cn.label(), schema)
	}
	other := &queryTab{name: "query_9", conn: c}
	if a.restoreQuery(other); other.want != alpha || other.conn != c {
		t.Errorf("restoring query_9: %v", other.want)
	}
}

// A database of a connection is the connection's to the end: what the session
// holds of the connections, its workers to close and its transactions to roll
// back, holds it; an edit of the connection reaches it, its database the same
// (F15.1).
func TestDatabaseIsTheConnections(t *testing.T) {
	a, c, _ := withDatabases(t)
	treeTo(t, a, "alpha")
	_, cmd := a.Update(teaKey("l"))
	answered(a, cmd)
	alpha := c.inDB("alpha")
	if got := len(a.sess.workers()); got != 4 {
		t.Errorf("%d workers", got)
	}
	alpha.tx = 'T'
	if got := txConns(a.sess); len(got) != 1 || got[0] != alpha || got[0].label() != "doraemon/alpha" {
		t.Errorf("transactions %v", got)
	}
	if a.sessionOf(alpha) != a.sess {
		t.Error("an answer for alpha would be dropped")
	}
}

// Edited in the manager, a connection's databases go over to what the edit
// says, each of them still the database it is (F15.1).
func TestDatabaseEditedWithTheConnection(t *testing.T) {
	a := managed(t)
	second := a.sess.Conns[1]
	second.Spec.Engine = "postgres"
	child := second.inDB("x")
	a.run("conn.manage", 0)
	feed(t, a, "j<Tab>")
	a.form.fields[fieldDSN].Input = ui.Input{Text: "postgres://u@m/s"}
	feed(t, a, "<C-s>")
	if second.Spec.DSN != "postgres://u@m/s" || child.Spec.DSN != second.Spec.DSN || child.Spec.Database != "x" || child.Spec.Name != "second" {
		t.Errorf("the connection %+v, its database %+v", second.Spec, child.Spec)
	}
}

// A table dropped in a database closes its tabs there, not those of a table of
// the same name in another database of the connection, nor the DSN's (F15.1).
func TestDropTableInDatabase(t *testing.T) {
	a, c, _ := withDatabases(t)
	alpha := c.inDB("alpha")
	x := db.Table{Schema: "public", Name: "t_x"}
	data := a.win().Root.A.Pane
	data.Tabs = []Tab{{Name: "t_x", Data: newDataTab(alpha, x)}, {Name: "t_x", Data: newDataTab(c, x)}}
	a.Update(dropped{alpha, x, nil})
	if len(data.Tabs) != 1 || data.Tabs[0].Data.conn != c {
		t.Errorf("%d tabs left, on %v", len(data.Tabs), data.Tabs[0].Data.conn.label())
	}
}

// The tree shows where a table's tab is, opening what hangs above the table:
// the database too, one of its own or the DSN's (F3.29, F15.1).
func TestRevealTabInDatabase(t *testing.T) {
	a, c, _ := withDatabases(t)
	treeTo(t, a, "alpha")
	_, cmd := a.Update(teaKey("l"))
	answered(a, cmd)
	alpha := c.inDB("alpha")
	data := a.win().Root.A.Pane
	for _, conn := range []*Conn{alpha, c} {
		for _, id := range []string{"conn:doraemon", dbNode(c, conn.DB), schemaNode(conn, "public"), groupNode(conn, "public", false)} { // all of it closed
			a.sess.open[id] = false
		}
		tb := alpha.Tables[0]
		if conn == c {
			tb = c.Tables[len(c.Tables)-1]
		}
		data.Tabs, data.Cur = []Tab{{Name: tb.Name, Data: newDataTab(conn, tb)}}, 0
		a.revealTab(data)
		if n, ok := a.treeNode(); !ok || n.kind != nodeTable || n.conn != conn || n.table.Name != tb.Name {
			t.Errorf("on %s the cursor is on %+v\n%s", conn.label(), n.TreeNode, treeTexts(a))
		}
	}
}
