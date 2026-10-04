package app

import (
	"slices"
	"testing"

	tea "charm.land/bubbletea/v2"

	"sqlmux/internal/db"
	"sqlmux/internal/editor"
)

// pane.focus.next goes clockwise round the middle of the screen, the
// status bar in it, and the sidebar when it is open (§6.8, F5.16).
func TestFocusNext(t *testing.T) {
	a := sized(160, 45, "nerd")
	win := a.win()
	order := func(from, n int) (ids []int) {
		win.focus(from)
		for range n {
			a.run("pane.focus.next", 0)
			ids = append(ids, win.Focus)
		}
		return ids
	}
	// a row of three full-height panes, the sidebar a fourth: 树 → 1 → 2 → 3 → 树
	win.focus(2)
	a.splitPane(Horiz) // 3 at the right
	if got := order(0, 5); !slices.Equal(got, []int{1, 2, 3, 0, 1}) {
		t.Errorf("a row: %v", got)
	}
	a.toggleTree()
	if got := order(1, 4); !slices.Equal(got, []int{2, 3, 1, 2}) {
		t.Errorf("sidebar folded: %v", got)
	}
	a.toggleTree()
	// the default layout and the result area below the panes: 树 → data → console → 结果区 → 树
	b := sized(160, 45, "nerd")
	bw := b.win()
	res := b.resultPane(bw)
	step := func(from int) int {
		bw.focus(from)
		b.run("pane.focus.next", 0)
		return bw.Focus
	}
	if got := []int{step(0), step(1), step(2), step(res.ID)}; !slices.Equal(got, []int{1, 2, res.ID, 0}) {
		t.Errorf("with the result area: %v, area %d", got, res.ID)
	}
	bw.resultHidden = false
	b.toggleResult() // hidden: it counts no more
	if got := []int{step(0), step(1), step(2)}; !slices.Equal(got, []int{1, 2, 0}) {
		t.Errorf("result area hidden: %v", got)
	}
	// a pane split, the sidebar and the result area: all five, round once
	b.toggleResult()
	bw.focus(1)
	b.splitPane(Vert)
	start, seen := bw.Focus, map[int]bool{}
	for range 5 {
		b.run("pane.focus.next", 0)
		seen[bw.Focus] = true
	}
	if len(seen) != 5 || bw.Focus != start {
		t.Errorf("five panes: visited %v, back on %d, began on %d", seen, bw.Focus, start)
	}
}

// console.format lays out every statement, as one undo step, the cursor at
// the start of the statement it was in, in bytes whatever comes before
// it; with the statements no more as many, where gq puts it; not in
// INSERT (F5.16).
func TestConsoleFormatAll(t *testing.T) {
	a, c := inConsole(t, "", "")
	format := func() {
		t.Helper()
		cmd := a.run("console.format", 0)
		if cmd == nil {
			t.Fatal("formats nothing")
		}
		a.Update(cmd())
	}
	a.consoleDid(c, c.ed.Load("select 1;\nselect a,b from t;"))
	feed(t, a, "j")
	format()
	if got := text(c); got != "select\n  1;\n\nselect\n  a,\n  b\nfrom\n  t;" || c.ed.Cursor() != (editor.Pos{Line: 3, Col: 0}) {
		t.Fatalf("%q, cursor %v", got, c.ed.Cursor())
	}
	if feed(t, a, "u"); text(c) != "select 1;\nselect a,b from t;" {
		t.Errorf("u: %q", text(c))
	}
	for _, w := range []struct {
		in   string
		want editor.Pos
	}{
		{"\tselect 1;", editor.Pos{Line: 0, Col: 1}},
		{"/* 中文 */ select 1;", editor.Pos{Line: 0, Col: 13}},
	} {
		a.consoleDid(c, c.ed.Load(w.in))
		format()
		if c.ed.Cursor() != w.want {
			t.Errorf("%q: cursor %v, want %v", w.in, c.ed.Cursor(), w.want)
		}
	}
	a.formatPrg = "printf 'select 1;\\nselect 2;\\nselect 3;'" // one more statement than went in
	a.consoleDid(c, c.ed.Load("select 1;\nselect 2;"))
	feed(t, a, "j")
	format()
	if got := c.ed.Cursor(); got != (editor.Pos{Line: 2, Col: 0}) {
		t.Errorf("a different count: cursor %v, want where gq puts it", got)
	}
	feed(t, a, "i")
	if a.run("console.format", 0) != nil {
		t.Error("formats in INSERT")
	}
}

// What the formatter brings back for a console closed meanwhile is dropped,
// the file's text is the one reopened (F5.16).
func TestFormatAllClosedMeanwhile(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	a, c := inConsole(t, "", "")
	a.consoleDid(c, c.ed.Load("select 1;"))
	cmd := a.run("console.format", 0)
	a.win().focus(2)
	a.run("tab.close", 0)
	a.run("tab.reopen", 0)
	a.Update(cmd())
	if c2 := a.focusedConsole(); c2 == nil || c2 == c || text(c) != "select 1;" {
		t.Errorf("the closed console %q was formatted", text(c))
	}
}

// tab.quit closes the tab, then the pane when it is left empty, then the
// window when it is its only pane; the session's last window stays; what
// would be lost is asked about first (F5.16).
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
	tab.edits = map[editKey]edit{{"1", "note"}: {val: db.Val{S: "x"}}}
	if a.run("tab.quit", 0); a.confirm == nil || len(a.sess.Windows) != 2 || len(a.focused().Tabs) != 1 {
		t.Fatalf("with changes: %+v", a.confirm)
	}
	if feed(t, a, "n"); len(a.sess.Windows) != 2 || len(a.focused().Tabs) != 1 {
		t.Fatalf("n: %d windows, %d tabs", len(a.sess.Windows), len(a.focused().Tabs))
	}
	a.run("tab.quit", 0)
	if feed(t, a, "y"); len(a.sess.Windows) != 1 {
		t.Fatalf("y: the table and the window go together: %d windows", len(a.sess.Windows))
	}
	// the last window stays: its one pane, no tab left, is the landing page
	a.win().focus(2)
	a.run("tab.quit", 0)
	a.win().focus(1)
	if a.run("tab.quit", 0); len(a.sess.Windows) != 1 || a.win().pane(1) == nil || len(a.win().pane(1).Tabs) != 0 {
		t.Errorf("the last window: %d windows", len(a.sess.Windows))
	}
	// a pane with another tab only loses the one
	feed(t, a, "<Space>c")
	win = a.win()
	loadOrders(t, a, 3)
	a.newTab()
	a.run("tab.quit", 0)
	if p := win.pane(1); len(a.sess.Windows) != 2 || len(win.Root.Leaves()) != 2 || len(p.Tabs) != 1 {
		t.Errorf("a pane that has a tab left: %d windows, %d panes, %d tabs", len(a.sess.Windows), len(win.Root.Leaves()), len(p.Tabs))
	}
}

// tab.reopen brings back the tabs closed last, newest first, a table with
// its WHERE, ORDER and LIMIT, a console on its file and schema; a table
// open already is only shown; 20 are kept; landing tabs and results are
// not recorded (F5.16).
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
	consoleOf(a.focused()).schema = "agentable"
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
	if c := consoleOf(p); c == nil || c.n != 1 || c.schema != "agentable" || len(p.Tabs) != 2 {
		t.Fatalf("console: %+v in pane %d", c, p.ID)
	}
	selectTab(p, 0)
	a.run("tab.close", 0) // the table
	loadOrders(t, a, 3)
	a.run("tab.reopen", 0) // already open: shown, not twice
	if n := len(p.Tabs); n != 2 || dataOf(p) == nil {
		t.Errorf("a table open already: %d tabs", n)
	}

	s := a.sess
	s.closed = nil
	table := db.Table{Schema: "public", Name: "t_order"}
	for i := range 21 {
		s.recordClosed(1, 1, Tab{Name: "t", Data: &dataTab{conn: s.Conns[0], table: table, request: request{limit: 100 + i}}})
	}
	s.recordClosed(1, 1, landing())
	s.recordClosed(1, 1, Tab{Name: "log", Result: &resultTab{}})
	if len(s.closed) != closedKept || s.closed[0].req.limit != 101 || s.closed[len(s.closed)-1].req.limit != 120 {
		t.Errorf("kept %d, the first limit %d: the 21st takes the oldest out, a landing tab and a result are none", len(s.closed), s.closed[0].req.limit)
	}
}

// A console open in another session is shown there: that session
// attached, its tab current, the record gone (§6.8, F5.16).
func TestTabReopenConsoleElsewhere(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	a := sized(160, 45, "nerd")
	first := a.sess
	second := newSession("2", []*Conn{first.Conns[0]}, first.Conns[0], 2)
	a.sessions = append(a.sessions, second)
	first.recordClosed(1, 2, Tab{Name: "console_2", Console: consoleOf(second.Windows[0].pane(2))})
	if a.run("tab.reopen", 0); a.sess != second || a.focused().ID != 2 || len(first.closed) != 0 {
		t.Errorf("session %q, focus %d, %d recorded", a.sess.Name, a.focused().ID, len(first.closed))
	}
	if n := len(first.consoles()); n != 1 {
		t.Errorf("opened again in the first: %d consoles", n)
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
