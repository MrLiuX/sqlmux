//go:build integration

package app

import (
	"context"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"sqlmux/internal/config"
	"sqlmux/internal/db/postgres"
	"sqlmux/internal/keymap"
)

// settle delivers what connecting and the catalog bring back, and what
// those bring in turn; the rest is dropped.
func settle(a *App, cmd tea.Cmd) {
	if cmd == nil {
		return
	}
	switch m := cmd().(type) {
	case tea.BatchMsg:
		for _, c := range m {
			settle(a, c)
		}
	case connected, catalogMsg, sessionDialed:
		_, next := a.Update(m)
		settle(a, next)
	}
}

// Only the connection started on connects; another connects once opened
// in the tree, open on its current_schema(); one that cannot is marked
// failed, named in a toast, the others as they were (§7.8「连接节点」).
func TestConnectOnOpen(t *testing.T) {
	dsn := postgres.IntegrationDSN(t)
	sess, err := Open(context.Background(), []config.Connection{
		{Name: "play", Engine: "postgres", DSN: dsn},
		{Name: "seed", Engine: "postgres", DSN: dsn},
		{Name: "broken", Engine: "postgres", DSN: "postgres://nobody@127.0.0.1:1/x?connect_timeout=2"},
	}, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Close()
	cfg := config.Default()
	keys, _ := keymap.New(cfg)
	a := New(cfg, keys, sess, &config.State{}, "")
	a.Update(tea.WindowSizeMsg{Width: 160, Height: 45})
	settle(a, a.loadCatalog(sess.Conns[0]))
	play, seed, broken := sess.Conns[0], sess.Conns[1], sess.Conns[2]
	if play.State != connReady || len(play.Tables) == 0 || seed.State != connIdle || broken.State != connIdle {
		t.Fatalf("at start: %v %v %v", play.State, seed.State, broken.State)
	}
	a.win().focus(0)
	for _, c := range []*Conn{broken, seed} {
		treeTo(t, a, c.Spec.Name)
		_, cmd := a.Update(teaKey("l"))
		settle(a, cmd)
	}
	if broken.State != connFailed || !strings.HasPrefix(a.toast, "broken：") {
		t.Errorf("broken: %v, toast %q", broken.State, a.toast)
	}
	ns, _ := a.treeNodes()
	home := slices.IndexFunc(ns, func(n node) bool { return n.id == schemaNode(seed, seed.home) })
	if seed.State != connReady || len(seed.Tables) != len(play.Tables) || home < 0 || !ns[home].Open {
		t.Errorf("seed: %v, %d tables, %s at %d\n%s", seed.State, len(seed.Tables), seed.home, home, treeTexts(a))
	}
	if play.State != connReady || a.conn() != seed {
		t.Errorf("play %v; current %s", play.State, a.conn().Spec.Name)
	}
}

// password_cmd runs once for a connection however often it is dialed,
// again only after a dial failed (§13).
func TestSecretCached(t *testing.T) {
	u, err := url.Parse(postgres.IntegrationDSN(t))
	if err != nil {
		t.Fatal(err)
	}
	pw, _ := u.User.Password()
	u.User = url.User(u.User.Username())
	runs := filepath.Join(t.TempDir(), "runs")
	spec := func(pw string) config.Connection {
		return config.Connection{Name: "play", Engine: "postgres", DSN: u.String(), PasswordCmd: "echo >> " + runs + "; printf " + pw}
	}
	count := func() int { b, _ := os.ReadFile(runs); return strings.Count(string(b), "\n") }
	for range 2 {
		d, err := dial(context.Background(), spec(pw))
		if err != nil {
			t.Fatal(err)
		}
		d.main.Close()
		d.meta.Close()
	}
	if count() != 1 {
		t.Errorf("two dials ran password_cmd %d times", count())
	}
	for range 2 {
		if _, err := dial(context.Background(), spec("wrong")); err == nil {
			t.Fatal("a wrong password connects")
		}
	}
	if count() != 3 {
		t.Errorf("a failed dial kept the password: %d runs", count())
	}
}

// Two consoles on one connection share its Main, its transaction too: one
// begins, the other is in it and stays on the connection till it ends;
// then it goes to another once that connects (§5, F5.11).
func TestConsolesShareMain(t *testing.T) {
	dsn := postgres.IntegrationDSN(t)
	sess, err := Open(context.Background(), []config.Connection{
		{Name: "play", Engine: "postgres", DSN: dsn},
		{Name: "seed", Engine: "postgres", DSN: dsn},
	}, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Close()
	cfg := config.Default()
	keys, _ := keymap.New(cfg)
	a := New(cfg, keys, sess, &config.State{}, "")
	a.Update(tea.WindowSizeMsg{Width: 160, Height: 45})
	settle(a, a.loadCatalog(sess.Conns[0]))
	play, seed := sess.Conns[0], sess.Conns[1]
	c1 := consoleOf(a.win().Root.B.Pane)
	a.win().focus(a.win().Root.B.Pane.ID)
	a.run("console.new", 0)
	c2 := consoleOf(a.focused())
	run := func(c *consoleTab, sql string) {
		t.Helper()
		deliver(a, a.runSQL(c, c.conn, c.schema, "console", sql, 0, c.ver))
	}
	if run(c1, "begin"); play.tx != 'T' {
		t.Fatalf("begin: %c", play.tx)
	}
	if run(c2, "select 1"); play.tx != 'T' || c2.conn != play {
		t.Fatalf("the other console's run: %c", play.tx)
	}
	if a.pickConn(c2, seed); c2.conn != play || c2.want != nil {
		t.Error("left its connection in a transaction")
	}
	run(c2, "rollback")
	if settle(a, a.pickConn(c2, seed)); play.tx != 'I' || c2.conn != seed || seed.State != connReady {
		t.Errorf("rolled back: %c, on %s, seed %v", play.tx, c2.conn.Spec.Name, seed.State)
	}
}

// n makes a session of its own connections, the current one connected for
// it first, then attached: named 1, console_n the next free, the other
// connections not connected (§7.8「新建 session」).
func TestNewSession(t *testing.T) {
	dsn := postgres.IntegrationDSN(t)
	sess, err := Open(context.Background(), []config.Connection{
		{Name: "play", Engine: "postgres", DSN: dsn},
		{Name: "seed", Engine: "postgres", DSN: dsn},
	}, 0)
	if err != nil {
		t.Fatal(err)
	}
	cfg := config.Default()
	keys, _ := keymap.New(cfg)
	a := New(cfg, keys, sess, &config.State{}, "")
	defer a.Close()
	a.Update(tea.WindowSizeMsg{Width: 160, Height: 45})
	settle(a, a.loadCatalog(sess.Conns[0]))
	a.Update(teaKey("<Space>"))
	a.Update(teaKey("s"))
	_, cmd := a.Update(teaKey("n"))
	settle(a, cmd)
	s := a.sess
	if len(a.sessions) != 2 || s == sess || s.Name != "1" || s.Conns[0] == sess.Conns[0] || s.Conns[0].State != connReady || len(s.Conns[0].Tables) == 0 || s.Conns[1].State != connIdle {
		t.Fatalf("sessions %d, attached %s, %v / %v", len(a.sessions), s.Name, s.Conns[0].State, s.Conns[1].State)
	}
	if c := consoleOf(s.Windows[0].Root.B.Pane); c == nil || c.key() != "console_2" || c.conn != s.Conns[0] {
		t.Errorf("its console: %+v", c)
	}
}
