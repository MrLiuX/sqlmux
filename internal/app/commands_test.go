package app

import (
	"errors"
	"fmt"
	"image/color"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/exp/golden"

	"sqlmux/internal/config"
	"sqlmux/internal/db"
	"sqlmux/internal/editor"
	"sqlmux/internal/i18n"
	"sqlmux/internal/keymap"
	"sqlmux/internal/ui"
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
	// the data pane split, 1 above 3: 树 → 1 → 2 → 3 → 树, a clockwise turn, not ⟨n⟩ order
	c := sized(160, 45, "nerd")
	cw := c.win()
	cw.focus(1)
	c.splitPane(Vert) // the new pane, 3, is below 1
	cstep := func(from int) int {
		cw.focus(from)
		c.run("pane.focus.next", 0)
		return cw.Focus
	}
	if got := []int{cstep(0), cstep(1), cstep(2), cstep(3)}; !slices.Equal(got, []int{1, 2, 3, 0}) {
		t.Errorf("1 above 3: %v", got)
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

// Lines of keymaps.toml that did not take are listed in a notice at the
// start, one a line, closed by ↵; the program is as usual after it (§6.7,
// F6.1).
func TestKeyProblemsNotice(t *testing.T) {
	c := userConfig(t, "", "[keys.normal]\n\"g\" = \"tab.next\"\n\"gz\" = \"tab.prev\"\n\"<C-p>\" = \"tab.next\"\n\"<c-p>\" = \"tab.prev\"\n")
	keys, problems := keymap.New(c)
	if len(problems) < 2 {
		t.Fatalf("problems: %v", problems)
	}
	a := New(c, keys, testSession(), &config.State{}, "")
	a.Update(tea.WindowSizeMsg{Width: 160, Height: 45})
	a.KeyProblems(problems)
	if a.confirm == nil || !strings.HasPrefix(a.confirm.text, "keymaps.toml 有 2 处问题") || strings.Count(a.confirm.text, "\n") != len(problems) {
		t.Fatalf("notice: %+v", a.confirm)
	}
	screen := a.render().String()
	for _, p := range problems {
		if !strings.Contains(screen, p.String()) {
			t.Errorf("%q is not on screen", p)
		}
	}
	if !strings.Contains(screen, "知道了") || strings.Contains(screen, "取消") {
		t.Error("the one button: 知道了")
	}
	if feed(t, a, "<CR>"); a.confirm != nil {
		t.Error("↵ keeps it")
	}
	a.KeyProblems(nil)
	if a.confirm != nil {
		t.Error("none: a notice")
	}
	if feed(t, a, "<Space>c"); len(a.sess.Windows) != 2 {
		t.Error("the program is as usual")
	}
}

// theme.switch lists the themes there are, the one in use marked; the pick
// is on the screen at once, a token the file leaves out as tokyonight-storm
// has it, the icons chosen kept with the theme's [icon] on them; a theme
// that cannot be read says why and changes nothing; nothing is written
// (F6.2).
func TestThemeSwitch(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	themes := filepath.Join(dir, "sqlmux", "themes")
	os.MkdirAll(themes, 0o755)
	os.WriteFile(filepath.Join(themes, "mine.toml"), []byte("bg = \"#101010\"\n[icon.table]\ntext = \"T\"\n"), 0o644)
	os.WriteFile(filepath.Join(themes, "bad.toml"), []byte("bg = \"red\"\n"), 0o644)
	a := sized(160, 45, "ascii")
	a.run("theme.switch", 0)
	items, current := a.dropItems()
	if a.drop == nil || a.drop.kind != dropTheme || !slices.Equal(items, []string{"bad", "mine", "tokyonight-storm"}) || current != 2 {
		t.Fatalf("list: %+v %v current %d", a.drop, items, current)
	}
	feed(t, a, "mi<CR>")
	if a.drop != nil || a.themeName != "mine" || ansiHex(a.theme.Bg) != "#101010" || a.theme.Fg != ui.TokyonightStorm.Fg || a.icons.Table.Text != "T" || a.icons.Schema.Text != "#" {
		t.Fatalf("mine: bg %v fg %v table %q schema %q", a.theme.Bg, a.theme.Fg, a.icons.Table.Text, a.icons.Schema.Text)
	}
	if !strings.Contains(a.toast, "config.toml") {
		t.Errorf("toast %q", a.toast)
	}
	a.run("theme.switch", 0)
	feed(t, a, "bad<CR>")
	if a.themeName != "mine" || !strings.Contains(a.toast, "bad.toml") {
		t.Errorf("a theme that cannot be read: %q, toast %q", a.themeName, a.toast)
	}
	a.run("theme.switch", 0)
	feed(t, a, "tokyo<CR>")
	if a.theme != ui.TokyonightStorm || a.icons.Table.Text != "+" {
		t.Error("back to tokyonight-storm and the icons chosen")
	}
}

func ansiHex(c color.Color) string {
	r, g, b, _ := c.RGBA()
	return fmt.Sprintf("#%02x%02x%02x", r>>8, g>>8, b>>8)
}

// With language = "en" the screen is in English: the landing page, the
// palette, the confirm box over a table with changes, the query bar, the
// result area's log, a table's saved note (§14「界面语言」, F6.4).
func TestGoldenEnglish(t *testing.T) {
	if err := i18n.Set("en"); err != nil {
		t.Fatal(err)
	}
	defer i18n.Set("zh")
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	scenes := map[string]func() *App{
		"landing": func() *App { return wide(160, 45) },
		"palette": func() *App {
			a := wide(160, 45)
			feed(t, a, "<C-p>")
			return a
		},
		"confirm": func() *App {
			a := wide(160, 45)
			tab := loadOrders(t, a, 60)
			tab.edits = map[editKey]edit{{joinKey([]string{"1"}), "status"}: {val: db.Val{S: "done"}, orig: db.Val{S: "running"}}}
			feed(t, a, "x")
			return a
		},
		"table": func() *App {
			a := wide(160, 45)
			tab := loadOrders(t, a, 60)
			tab.note = ui.Note{Head: i18n.T("已保存 %d 行%s · %s", 2, "", "3ms")}
			return a
		},
		"result": func() *App {
			a, _ := inRun(t, &execDB{res: map[string]db.Result{"select 1": rows(3)}}, "select 1;")
			press(t, a, "<CR>")
			return a
		},
	}
	for name, scene := range scenes {
		t.Run(name, func(t *testing.T) {
			golden.RequireEqual(t, scene().render().String())
		})
	}
}

// layoutApp is an app over a session of a layout with the first session's
// connection ready; the others wait.
func layoutRestored(t *testing.T, l *config.Layout) *Restored {
	t.Helper()
	specs := []config.Connection{{Name: "doraemon", Engine: "postgres"}, {Name: "play", Engine: "postgres"}}
	r, err := restore(specs, l, func(c *Conn) error {
		c.State, c.Main, c.Meta = connReady, db.NewWorker(noDB{}), db.NewWorker(noDB{})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return r
}

// A layout is the sessions as they were: the windows, their split trees,
// the focus, the tabs with a table's request and a console's schema; and
// what the file gives comes back as it was given (§14「布局文件」, F6.5).
func TestLayoutSnapshotAndBuild(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	a := sized(160, 45, "nerd")
	win := a.win()
	data := win.pane(1)
	data.Tabs = []Tab{{Name: "t_order", Data: newDataTab(a.sess.Conns[0], db.Table{Schema: "public", Name: "t_order"})}, landing()}
	d := data.Tabs[0].Data
	d.request.applied, d.request.order, d.request.desc, d.request.limit, d.request.pageNo = "id > 1", "amount", true, 500, 2
	d.hidden["note"], d.transpose = true, true
	data.Cur = 0
	consoleOf(win.pane(2)).schema = "agentable"
	win.focus(2)
	a.resultPane(win) // the area is not saved, its height is
	win.TreeW = 40
	a.run("window.new", 0)
	a.run("window.select 0", 0)

	l := a.layoutOf()
	w0 := l.Sessions[0].Windows[0]
	if len(l.Sessions) != 1 || len(l.Sessions[0].Windows) != 2 || w0.Focus != 2 || w0.SidebarWidth != 40 || w0.ResultHeight == 0 {
		t.Fatalf("window: %+v", w0)
	}
	if len(w0.Panes) != 2 || w0.Layout.Split != "h" || w0.Layout.A.Pane != 1 || w0.Layout.B.Pane != 2 {
		t.Fatalf("tree: %+v panes %+v", w0.Layout, w0.Panes)
	}
	tab := w0.Panes[0].Tabs
	if len(tab) != 1 || tab[0].Kind != "table" || tab[0].Table != "t_order" || tab[0].Where != "id > 1" || tab[0].Order != "amount" || !tab[0].Desc || tab[0].Limit != 500 || tab[0].Page != 2 || !tab[0].Transpose || len(tab[0].Cols) != 1 {
		t.Fatalf("table tab (no landing tab): %+v", tab)
	}
	if c := w0.Panes[1].Tabs[0]; c.Kind != "console" || c.N != 1 || c.Schema != "agentable" {
		t.Fatalf("console tab: %+v", c)
	}

	r := layoutRestored(t, l)
	s := r.Sessions[0]
	if len(r.Warnings) != 0 || len(s.Windows) != 2 || s.Active != 0 {
		t.Fatalf("restored: %v, %d windows", r.Warnings, len(s.Windows))
	}
	w := s.Windows[0]
	got := w.pane(1).Tabs[0].Data
	if w.Root.Split != Horiz || w.Focus != 2 || w.TreeW != 40 || w.resultRatio == 0 || got == nil || !got.pending ||
		got.applied != "id > 1" || got.where.Text != "id > 1" || got.order != "amount" || !got.desc || got.limit != 500 || got.pageNo != 2 || !got.hidden["note"] || !got.transpose {
		t.Fatalf("rebuilt: %+v", got)
	}
	if c := consoleOf(w.pane(2)); c == nil || c.n != 1 || c.schema != "agentable" {
		t.Fatalf("console: %+v", c)
	}
}

// What cannot be built is left out and said; the rest is built: a console
// number another session has goes to the next free one, a connection that
// is not there takes its tabs with it, a pane the tree names is made, a
// layout of nothing has the default window (§14「恢复」).
func TestLayoutRestoreGaps(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	console := func(n int) config.LayoutTab { return config.LayoutTab{Kind: "console", N: n, Connection: "doraemon"} }
	l := &config.Layout{Version: 1, Sessions: []config.LayoutSession{
		{Name: "1", Connection: "doraemon", Windows: []config.LayoutWindow{{
			Name: "a", Layout: &config.LayoutNode{Split: "h", Ratio: 0.5, A: &config.LayoutNode{Pane: 1}, B: &config.LayoutNode{Pane: 7}},
			Panes: []config.LayoutPane{{ID: 1, Tabs: []config.LayoutTab{console(1), {Kind: "table", Connection: "gone", Table: "t"}}}},
		}}},
		{Name: "2", Connection: "nowhere", Windows: []config.LayoutWindow{{Name: "b", Panes: []config.LayoutPane{{ID: 1, Tabs: []config.LayoutTab{console(1)}}}}}},
		{Name: "3", Connection: "play"},
	}}
	r := layoutRestored(t, l)
	if len(r.Sessions) != 3 {
		t.Fatalf("%d sessions", len(r.Sessions))
	}
	w := r.Sessions[0].Windows[0]
	if len(w.Root.Leaves()) != 2 || w.pane(7) == nil || len(w.pane(1).Tabs) != 1 || w.lastID != 7 {
		t.Errorf("panes: %d, 7 %v, tabs %d, last %d", len(w.Root.Leaves()), w.pane(7), len(w.pane(1).Tabs), w.lastID)
	}
	if c := consoleOf(r.Sessions[1].Windows[0].pane(1)); c == nil || c.n != 2 {
		t.Errorf("the second session's console_1 is taken: %+v", c)
	}
	if r.Sessions[1].last.Spec.Name != "doraemon" || len(r.Sessions[2].Windows) != 1 || consoleOf(r.Sessions[2].Windows[0].pane(2)).n != 3 {
		t.Errorf("a connection that is not there starts on the first; no window is the default")
	}
	if got := strings.Join(r.Warnings, "|"); !strings.Contains(got, "gone") || !strings.Contains(got, "nowhere") {
		t.Errorf("warnings: %q", got)
	}
	if r.Sessions[2].last.State == connReady || r.Sessions[1].last.State == connReady {
		t.Error("only the first session's start is dialed now")
	}
}

// A table tab of a layout waits for its connection's catalog: with the table
// there it fetches, without it the tab is closed and a toast says so; a
// connection that fails takes its waiting tabs, the toast being its own
// (§14「恢复」, F6.5).
func TestLayoutPendingTabs(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	tab := func(conn, table string) config.LayoutTab {
		return config.LayoutTab{Kind: "table", Connection: conn, Schema: "public", Table: table}
	}
	l := &config.Layout{Version: 1, Sessions: []config.LayoutSession{{Name: "1", Connection: "doraemon", Windows: []config.LayoutWindow{{
		Name: "a", Layout: &config.LayoutNode{Pane: 1},
		Panes: []config.LayoutPane{{ID: 1, Tabs: []config.LayoutTab{tab("doraemon", "t_order"), tab("doraemon", "dropped"), tab("play", "x")}}},
	}}}}}
	r := layoutRestored(t, l)
	a := New(config.Default(), mustKeys(t), r.Sessions[0], &config.State{}, "")
	a.Adopt(r)
	a.Update(tea.WindowSizeMsg{Width: 160, Height: 45})
	cmds := a.connectsNow()
	if len(cmds) != 2 { // the first's catalog, play's connect
		t.Errorf("at start: %d commands", len(cmds))
	}
	doraemon, play := r.Sessions[0].Conns[0], r.Sessions[0].Conns[1]
	p := r.Sessions[0].Windows[0].pane(1)
	if len(p.Tabs) != 3 {
		t.Fatalf("tabs %d", len(p.Tabs))
	}
	a.Update(catalogMsg{conn: doraemon, schemas: []string{"public"}, current: "public", tables: []db.Table{{Schema: "public", Name: "t_order", Rows: 5}}})
	if len(p.Tabs) != 2 || p.Tabs[0].Data.pending || p.Tabs[0].Data.table.Rows != 5 || p.Tabs[0].Data.seq == 0 || !strings.Contains(a.toast, "dropped") {
		t.Fatalf("after the catalog: %d tabs, %+v, toast %q", len(p.Tabs), p.Tabs[0].Data, a.toast)
	}
	if !p.Tabs[1].Data.pending {
		t.Error("play's waits")
	}
	a.Update(connected{conn: play, err: errors.New("refused")})
	if len(p.Tabs) != 1 || !strings.Contains(a.toast, "refused") {
		t.Errorf("play failed: %d tabs, toast %q", len(p.Tabs), a.toast)
	}
}

func mustKeys(t *testing.T) *keymap.Map {
	t.Helper()
	k, _ := keymap.New(config.Default())
	return k
}

// The working set is written as the program quits, once it is told where:
// not by the tests, which are never told (F6.5).
func TestLayoutSavedOnQuit(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	a := sized(160, 45, "nerd")
	path := filepath.Join(t.TempDir(), "last.toml")
	if feed(t, a, ":qa<CR>"); !quits(a.quit()) {
		t.Fatal("quit")
	}
	if _, err := os.Stat(path); err == nil {
		t.Fatal("written with nowhere to write")
	}
	a.SaveLayoutOnExit(path)
	a.quit()
	l, err := config.LoadLayout(path)
	if err != nil || len(l.Sessions) != 1 || len(l.Sessions[0].Windows[0].Panes) != 2 {
		t.Fatalf("last: %+v %v", l, err)
	}
}

// layout.save asks the name in the status bar: ↵ writes layouts/<name>.toml,
// esc and a click away drop it, a name that cannot be one is said; layout.open
// lists them, last first when there is one, and a pick replaces the sessions
// of the layout it loaded (F6.5).
func TestLayoutSaveAndOpen(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	a := sized(160, 45, "nerd")
	a.run("layout.save", 0)
	if a.layoutName == nil || a.mode() != keymap.Insert || a.render().Cursor == nil {
		t.Fatal("no prompt")
	}
	if feed(t, a, "my/x<CR>"); a.layoutName != nil || !strings.Contains(a.toast, "/") {
		t.Errorf("a bad name: %q", a.toast)
	}
	a.run("layout.save", 0)
	feed(t, a, "<Esc>")
	if a.layoutName != nil || len(config.LayoutNames()) != 0 {
		t.Error("esc")
	}
	a.run("layout.save", 0)
	feed(t, a, "work<CR>")
	if l, err := config.LoadLayout("work"); err != nil || len(l.Sessions) != 1 || !strings.Contains(a.toast, "work") {
		t.Fatalf("saved: %v, toast %q", err, a.toast)
	}
	config.SaveLayout(config.LastPath(), a.layoutOf())
	a.run("layout.open", 0)
	if items, _ := a.dropItems(); !slices.Equal(items, []string{"last", "work"}) {
		t.Fatalf("layouts: %v", items)
	}
	// what a pick brings: the sessions are the layout's, the old ones closed
	r := layoutRestored(t, &config.Layout{Version: 1, Sessions: []config.LayoutSession{{Name: "x", Connection: "doraemon"}, {Name: "y", Connection: "play"}}, Active: 1})
	a.drop = nil
	a.Update(layoutOpened{r: r})
	if len(a.sessions) != 2 || a.sess.Name != "y" || !a.restored {
		t.Errorf("replaced: %d sessions, on %q", len(a.sessions), a.sess.Name)
	}
	a.Update(layoutOpened{err: errors.New("no such layout")})
	if len(a.sessions) != 2 || !strings.Contains(a.toast, "no such layout") {
		t.Error("an error changes nothing and is said")
	}
}

// shiftArrow presses an arrow with Shift, as a terminal sends it.
func shiftArrow(a *App, code rune, n int) {
	for range n {
		a.Update(tea.KeyPressMsg{Code: code, Mod: tea.ModShift})
	}
}

// vim = false: a console is typed into at once, ↵ makes a line, Esc and the
// vim keys are text or nothing, there is no mode to name, and the pane focus
// keys still go (§6.9, F6.6a).
func TestPlainConsole(t *testing.T) {
	a, c := inConsole(t, "vim = false", "")
	feed(t, a, "select 1<CR>gt<Space>x<Esc>")
	if text(c) != "select 1\ngt x" || c.ed.Mode() != editor.Insert {
		t.Fatalf("typed: %q in %v", text(c), c.ed.Mode())
	}
	if row := statusRow(a); strings.Contains(row, "INSERT") || strings.Contains(row, "NORMAL") {
		t.Errorf("a mode block: %q", row)
	}
	win := a.win()
	feed(t, a, "<C-h>")
	if win.Focus == 2 {
		t.Error("C-h does not move the focus")
	}
	feed(t, a, "<C-l>")
	if win.Focus != 2 {
		t.Error("C-l does not move it back")
	}
	feed(t, a, "ab<C-a>X<C-e>Y")
	if got := c.ed.Lines()[1]; got != "gt xabXY"[:0]+"Xgt xabY" && got != "Xgt xabY" {
		t.Errorf("C-a, C-e: %q", got)
	}
}

// A selection made with Shift and the arrows is what runs, else the
// statement at the cursor; it is replaced by what is typed (§6.9).
func TestPlainConsoleSelectionRuns(t *testing.T) {
	d := &execDB{res: map[string]db.Result{"select 1": rows(1), "select 2": rows(1)}}
	a, c := inRun(t, d, "")
	a.plain = true
	feed(t, a, "select 1;<CR>select 2;")
	feed(t, a, "<C-a>")
	shiftArrow(a, tea.KeyRight, 9)
	if got := c.ed.SelectedText(); got != "select 2;" {
		t.Fatalf("selected %q", got)
	}
	deliver(a, a.run("console.run", 0))
	if len(d.ran) != 1 || !strings.HasPrefix(d.ran[0], "select 2") || c.ed.HasSelection() {
		t.Errorf("a selection: ran %q, still selected %v", d.ran, c.ed.HasSelection())
	}
	d.ran = nil
	deliver(a, a.run("console.run", 0)) // none: the statement at the cursor
	if len(d.ran) != 1 {
		t.Errorf("none selected: ran %q", d.ran)
	}
	shiftArrow(a, tea.KeyLeft, 3)
	if feed(t, a, "<C-c>"); c.ed.HasSelection() || a.toast != "" {
		t.Errorf("C-c with a selection only ends it: toast %q", a.toast)
	}
	if feed(t, a, "<C-c>"); !strings.Contains(a.toast, "退出") {
		t.Errorf("C-c with nothing: the first press of quit: %q", a.toast)
	}
}

// vim = false: the WHERE is typed into, ↵ runs it, Esc closes the list and
// then leaves it; C-r is still its history (§6.9, F6.6a).
func TestPlainWhere(t *testing.T) {
	c := userConfig(t, "vim = false", "")
	a := sizedWith(160, 45, c)
	a.Update(tea.WindowSizeMsg{Width: 160, Height: 45})
	a.win().focus(1)
	tab := loadOrders(t, a, 3)
	feed(t, a, "/")
	if tab.typing != "where" || tab.ed == nil || !tab.ed.Plain() || tab.ed.Mode() != editor.Insert {
		t.Fatalf("typing %q, %+v", tab.typing, tab.ed)
	}
	feed(t, a, "id = 1")
	if tab.where.Text != "id = 1" {
		t.Fatalf("where %q", tab.where.Text)
	}
	feed(t, a, "<C-a>x<Esc>")
	if tab.where.Text != "" || tab.typing != "" { // what was not run goes, as the vim's second esc
		t.Errorf("esc leaves: %q, typing %q", tab.where.Text, tab.typing)
	}
	feed(t, a, "/ and st")
	if tab.comp == nil {
		t.Fatalf("no list")
	}
	feed(t, a, "<Esc>")
	if tab.comp != nil || tab.typing != "where" {
		t.Errorf("the first esc closes the list only: %v %q", tab.comp, tab.typing)
	}
}
