package app

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"sqlmux/internal/editor"
)

// pane.focus.next goes clockwise round the middle of the screen, the
// sidebar when it is open (F5.16).
func TestFocusNext(t *testing.T) {
	a := sized(160, 45, "nerd")
	win := a.win()
	order := func(from int, n int) (ids []int) {
		win.focus(from)
		for range n {
			a.run("pane.focus.next", 0)
			ids = append(ids, win.Focus)
		}
		return ids
	}
	// the console on the right at 3 o'clock, then the data pane and the
	// sidebar both at 9, the nearer first
	if got := order(2, 4); !equalInts(got, []int{1, 0, 2, 1}) {
		t.Errorf("default layout: %v", got)
	}
	a.toggleTree()
	if got := order(2, 3); !equalInts(got, []int{1, 2, 1}) {
		t.Errorf("sidebar folded: %v", got)
	}
	a.toggleTree()
	win.focus(1)
	a.splitPane(Vert) // 3 below 1: at 7, the sidebar at 9, 1 at 11
	if got := order(2, 5); !equalInts(got, []int{3, 0, 1, 2, 3}) {
		t.Errorf("a pane below: %v", got)
	}
}

func equalInts(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// console.format lays out every statement, as one undo step, the cursor at
// the start of the statement it was in (F5.16).
func TestConsoleFormatAll(t *testing.T) {
	a, c := inConsole(t, "", "")
	a.consoleDid(c, c.ed.Load("select 1;\nselect a,b from t;"))
	feed(t, a, "j")
	cmd := a.run("console.format", 0)
	if cmd == nil {
		t.Fatal("formats nothing")
	}
	a.Update(cmd())
	if got := text(c); got != "select\n  1;\n\nselect\n  a,\n  b\nfrom\n  t;" || c.ed.Cursor() != (editor.Pos{Line: 3, Col: 0}) {
		t.Fatalf("%q, cursor %v", got, c.ed.Cursor())
	}
	if feed(t, a, "u"); text(c) != "select 1;\nselect a,b from t;" {
		t.Errorf("u: %q", text(c))
	}
	feed(t, a, "i")
	if a.run("console.format", 0) != nil {
		t.Error("formats in INSERT")
	}
}

// tab.quit closes the tab, then the pane when it is left empty, then the
// window when it is its only pane; the session's last window stays
// (F5.16).
func TestTabQuit(t *testing.T) {
	a := sized(160, 45, "nerd")
	feed(t, a, "<Space>c")
	win := a.win()
	tab := loadOrders(t, a, 3)
	win.focus(2)
	a.run("tab.quit", 0)
	if len(a.sess.Windows) != 2 || win.pane(2) != nil || len(win.Root.Leaves()) != 1 {
		t.Fatalf("a console's tab, its pane left empty: %d windows, panes %d", len(a.sess.Windows), len(win.Root.Leaves()))
	}
	win.focus(1)
	if dataOf(a.focused()) != tab {
		t.Fatal("the table is not in the pane")
	}
	a.run("tab.quit", 0)
	if len(a.sess.Windows) != 1 {
		t.Fatalf("the only pane emptied: %d windows", len(a.sess.Windows))
	}
	if a.run("tab.quit", 0); len(a.sess.Windows) != 1 {
		t.Error("the last window closed")
	}
	feed(t, a, "<Space>c")
	a.run("tab.quit", 0) // the empty data pane, the console's beside it
	if got := len(a.win().Root.Leaves()); got != 1 || len(a.sess.Windows) != 2 {
		t.Errorf("an empty pane: %d panes, %d windows", got, len(a.sess.Windows))
	}
}

// tab.reopen brings back the tabs closed last, newest first, a table with
// its WHERE, ORDER and LIMIT, a console on its file; a table open already
// is only shown (F5.16).
func TestTabReopen(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	a := sized(160, 45, "nerd")
	if a.run("tab.reopen", 0) != nil || len(a.sess.closed) != 0 {
		t.Fatal("nothing closed, something reopened")
	}
	tab := loadOrders(t, a, 3)
	tab.applied, tab.order, tab.desc, tab.limit = "id > 1", "amount", true, 500
	win := a.win()
	win.focus(2)
	a.run("tab.close", 0) // the console, its pane gone
	win.focus(1)
	a.run("tab.close", 0) // the table, the pane the window's only one
	if win.pane(2) != nil || len(a.sess.closed) != 2 {
		t.Fatalf("closed: pane %v, %d recorded", win.pane(2), len(a.sess.closed))
	}
	a.run("tab.reopen", 0) // the table, in its own pane
	d := dataOf(a.focused())
	if d == nil || d.table.Name != "t_order" || d.applied != "id > 1" || d.order != "amount" || !d.desc || d.limit != 500 || d.where.Text != "id > 1" {
		t.Fatalf("table: %+v", d)
	}
	a.run("tab.reopen", 0) // the console: its pane is gone, so the current one
	p := a.focused()
	if c := consoleOf(p); c == nil || c.n != 1 || c.schema != "public" || len(p.Tabs) != 2 {
		t.Fatalf("console: %+v in pane %d", c, p.ID)
	}
	selectTab(p, 0)
	a.run("tab.close", 0) // the table
	loadOrders(t, a, 3)
	a.run("tab.reopen", 0) // already open: shown, not twice
	if n := len(p.Tabs); n != 2 || dataOf(p) == nil {
		t.Errorf("a table open already: %d tabs", n)
	}
}

// A key held with Cmd types nothing and does nothing the plain key would,
// in NORMAL or INSERT; bound in [keys.global] it runs (F5.17).
func TestSuperKeys(t *testing.T) {
	a, c := inConsole(t, "", "[keys.global]\n\"<D-w>\" = \"tree.toggle\"\n")
	feed(t, a, "iselect abc<Esc>")
	open := a.win().TreeOpen
	press := func(code rune) { a.Update(tea.KeyPressMsg{Code: code, Mod: tea.ModSuper}) }
	press('x')
	feed(t, a, "i")
	press('y')
	feed(t, a, "<Esc>")
	if text(c) != "select abc" {
		t.Errorf("typed: %q", text(c))
	}
	if press('w'); a.win().TreeOpen == open {
		t.Error("<D-w> did not run")
	}
}
