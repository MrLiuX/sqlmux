package app

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"sqlmux/internal/db"
	"sqlmux/internal/editor"
)

// ran is what the console sent the database, the reads without the LIMIT
// they get and the search_path sets left out.
func ran(d *execDB) []string {
	var out []string
	for _, s := range d.ran {
		if !strings.HasPrefix(s, "search_path") {
			s, _, _ = strings.Cut(s, "\nLIMIT ")
			out = append(out, s)
		}
	}
	return out
}

// What runs is the selection exactly, over lines and across statements,
// else the statement at the cursor (§6.9).
func TestPlainConsoleRunsSelection(t *testing.T) {
	d := &execDB{}
	a, c := inConsole(t, "vim = false", "")
	a.sess.Conns[0].Main = db.NewWorker(d)
	c.schema = ""
	feed(t, a, "1;<CR>select 2;")
	feed(t, a, "<Up><C-a>")
	shiftArrow(a, tea.KeyRight, 10) // "1;\nselect " ends in the middle of the second statement
	if from, to, _ := c.ed.SelectionSpan(); from != (editor.Pos{}) || to != (editor.Pos{Line: 1, Col: 7}) {
		t.Fatalf("selected %v to %v", from, to)
	}
	deliver(a, a.run("console.run", 0))
	if got := ran(d); !slices.Equal(got, []string{"1", "select"}) || c.ed.HasSelection() {
		t.Errorf("the selection: ran %q, still selected %v", got, c.ed.HasSelection())
	}
	d.ran = nil
	c.ed.Goto(editor.Pos{Line: 1, Col: 3})
	deliver(a, a.run("console.run", 0))
	if got := ran(d); !slices.Equal(got, []string{"select 2"}) {
		t.Errorf("none selected: the statement at the cursor, ran %q", got)
	}
}

// ↵ of the WHERE runs it, C-r is its history, a Ctrl key goes on to its
// binding with the candidate list open, and the mouse selects (§6.9).
func TestPlainWhereKeys(t *testing.T) {
	a := sizedWith(160, 45, userConfig(t, "vim = false", ""))
	a.win().focus(1)
	tab := loadOrders(t, a, 3)
	feed(t, a, "/id = 1<CR>")
	if tab.applied != "id = 1" || tab.typing != "" {
		t.Fatalf("↵: applied %q, typing %q", tab.applied, tab.typing)
	}
	feed(t, a, "/<C-r>")
	if tab.hist == nil {
		t.Error("C-r is the history")
	}
	feed(t, a, "<Esc><Esc>")
	feed(t, a, "/ and st")
	if tab.comp == nil {
		t.Fatal("no list")
	}
	if feed(t, a, "<C-l>"); a.win().Focus == 1 {
		t.Error("C-l with the list open keeps the focus")
	}

	// the pointer: down on the input, dragged right
	a.win().focus(1)
	feed(t, a, "/id = 1")
	a.View()
	pn := a.focused()
	r := a.queryBar(pn, tab).InputRect(bodyRect(a.layout()[pn.ID]))
	a.Update(tea.MouseClickMsg{X: r.Min.X + 1, Y: r.Min.Y, Button: tea.MouseLeft})
	a.Update(tea.MouseMotionMsg{X: r.Min.X + 4, Y: r.Min.Y, Button: tea.MouseLeft})
	from, to, ok := tab.ed.SelectionSpan()
	a.Update(tea.MouseReleaseMsg{X: r.Min.X + 4, Y: r.Min.Y, Button: tea.MouseLeft})
	if !ok || from.Col != 1 || to.Col != 4 {
		t.Errorf("drag: %v to %v, selecting %v", from, to, ok)
	}
	a.Update(tea.MouseMotionMsg{X: r.Min.X + 6, Y: r.Min.Y, Button: tea.MouseLeft})
	if _, to, _ := tab.ed.SelectionSpan(); to.Col != 4 {
		t.Errorf("the button is up: the selection stays, %v", to)
	}
}

// C-c in a plain console ends the selection, else closes the list, else is
// the first press of quit; C-h is Backspace there and C-l still goes to the
// next pane (§6.9).
func TestPlainConsoleKeys(t *testing.T) {
	a, c := inConsole(t, "vim = false", "")
	feed(t, a, "sele")
	if c.comp == nil {
		t.Fatal("no list")
	}
	if feed(t, a, "<C-c>"); c.comp != nil || a.toast != "" || text(c) != "sele" {
		t.Errorf("C-c closes the list only: %v %q %q", c.comp, a.toast, text(c))
	}
	win := a.win()
	feed(t, a, "<C-h>")
	if win.Focus != 2 || text(c) != "sel" {
		t.Errorf("C-h is Backspace: focus %d, %q", win.Focus, text(c))
	}
	feed(t, a, "<C-l>")
	if win.Focus != 2 {
		t.Error("C-l from the rightmost pane stays")
	}
	feed(t, a, "<C-j>")
	if win.Focus != 2 {
		t.Error("C-j with nothing below stays")
	}
}

// A console made after the program began has no modes before its first
// key: a ? is the text, not the help (§6.9).
func TestPlainFreshConsole(t *testing.T) {
	a := sizedWith(160, 45, userConfig(t, "vim = false", ""))
	feed(t, a, "<Space>c") // made in an Update: what is drawn next is modeless already
	if c := a.win().pane(2); consoleOf(c) == nil || !consoleOf(c).ed.Plain() {
		t.Fatal("a console made by a command is not modeless as it is drawn")
	}
	feed(t, a, "<C-l>")
	c := a.focusedConsole()
	if c == nil {
		t.Fatal("no console")
	}
	feed(t, a, "?")
	if a.keyHelp != nil || text(c) != "?" || !c.ed.Plain() {
		t.Errorf("a ? typed at once: help %v, %q", a.keyHelp != nil, text(c))
	}
}

// After the $EDITOR the text is read back, and the console is still typed
// into (§6.9).
func TestPlainExternalEditor(t *testing.T) {
	a, c := inConsole(t, "vim = false", "")
	feed(t, a, "select 1")
	os.MkdirAll(filepath.Dir(c.path), 0o700)
	if err := os.WriteFile(c.path, []byte("select 2\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	a.gotExternal(externalDone{c, nil})
	a.View()
	feed(t, a, "x")
	if !strings.Contains(text(c), "x") || strings.Contains(text(c), "select 1") || c.ed.Mode() != editor.Insert {
		t.Errorf("typed after it: %q in %v", text(c), c.ed.Mode())
	}
}

// console.format over a selection lays out that, none the statements; both
// are steps of their own (§6.9, F5.16).
func TestPlainConsoleFormat(t *testing.T) {
	a, c := inConsole(t, "vim = false\nformatprg = \"tr a-z A-Z\"", "")
	format := func() {
		t.Helper()
		cmd := a.run("console.format", 0)
		if cmd == nil {
			t.Fatal("formats nothing")
		}
		a.Update(cmd())
	}
	a.consoleDid(c, c.ed.Load("select a;\nselect b;"))
	c.ed.Goto(editor.Pos{})
	shiftArrow(a, tea.KeyRight, 6)
	format()
	if text(c) != "SELECT a;\nselect b;" || c.ed.HasSelection() {
		t.Fatalf("a selection: %q", text(c))
	}
	format()
	if text(c) != "SELECT A;\nSELECT B;" {
		t.Errorf("none: %q", text(c))
	}
	if feed(t, a, "<C-z>"); text(c) != "SELECT a;\nselect b;" {
		t.Errorf("undo is the formatting only: %q", text(c))
	}
}

// A hint names the keys the layer gave, and a plain console has no tabs to
// walk with gt (§6.9).
func TestNoVimHints(t *testing.T) {
	vim := sizedWith(160, 45, userConfig(t, "", ""))
	novim := sizedWith(160, 45, userConfig(t, "vim = false", ""))
	for _, a := range []*App{vim, novim} {
		a.win().focus(1)
		loadOrders(t, a, 3)
	}
	if s := vim.render().String(); !strings.Contains(s, "hjkl") || !strings.Contains(s, "gt/gT") {
		t.Errorf("vim: no hjkl or gt/gT in the grid's hints")
	}
	if s := novim.render().String(); !strings.Contains(s, "←↓↑→") || strings.Contains(s, "hjkl") {
		t.Errorf("vim = false: the grid's hint is the arrows")
	}
	a, c := inConsole(t, "vim = false", "")
	if s := a.render().String(); strings.Contains(s, "gt/gT") || !strings.Contains(s, "C-e") {
		t.Errorf("a plain console's bar: %q has gt/gT or no C-e", c.path)
	}
	a, _ = inConsole(t, "", "")
	if s := a.render().String(); !strings.Contains(s, "gt/gT") {
		t.Error("a vim console's bar has gt/gT")
	}
}
