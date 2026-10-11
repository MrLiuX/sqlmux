package app

import (
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/exp/golden"

	"github.com/kackerx/sqlmux/internal/editor"
	"github.com/kackerx/sqlmux/internal/keymap"
	"github.com/kackerx/sqlmux/internal/ui"
)

// helpItems are the ? help's items as "key title [table]".
func helpItems(a *App) []string {
	var out []string
	for _, it := range a.keyHelpView().Items {
		out = append(out, it.Key+" "+it.Title+" ["+it.Group+"]")
	}
	return out
}

// ? lists the grid's keys by the table each comes from, user maps first;
// a prefix goes down a level and BS back; a key listed closes it and runs
// as typed there; esc, C-c and a global key close it (§6.5).
func TestKeyHelp(t *testing.T) {
	a := configured(t, 160, 45, "[map.grid.normal]\nJ = \"5j\"")
	tab := loadOrders(t, a, 20)
	feed(t, a, "?")
	items := helpItems(a)
	for _, want := range []string{"J 5j [map.grid.normal]", "j " + title("grid.down") + " [keys.grid]", "g … [keys.grid]", "C-h " + title("pane.focus.left") + " [keys.normal]", "C-p 命令面板 [keys.global]"} {
		if !slices.Contains(items, want) {
			t.Errorf("lacks %q: %v", want, items)
		}
	}
	if a.mode() != keymap.Command || !strings.Contains(a.render().String(), "[map.grid.normal]") {
		t.Fatalf("COMMAND, headings drawn: %v", a.mode())
	}
	if feed(t, a, "g"); a.keyHelpView().Prefix != "g" || !slices.Contains(helpItems(a), "o "+title("grid.order")+" [keys.grid]") || !slices.Contains(helpItems(a), "t "+title("tab.next")+" [keys.normal]") {
		t.Fatalf("g: %v", helpItems(a))
	}
	if feed(t, a, "<BS>"); a.keyHelpView().Prefix != "?" {
		t.Fatal("BS: back to the top")
	}
	if feed(t, a, "Z"); a.keyHelp == nil {
		t.Fatal("a key it doesn't list does nothing")
	}
	if feed(t, a, "J"); a.keyHelp != nil || tab.row != 5 {
		t.Fatalf("J runs the mapping: row %d", tab.row)
	}
	if feed(t, a, "?go"); a.keyHelp != nil || a.cols == nil || !a.cols.order {
		t.Fatal("g o opens ORDER")
	}
	feed(t, a, "<Esc>?<Esc>")
	if a.keyHelp != nil {
		t.Fatal("esc closes it")
	}
	if feed(t, a, "?<C-c>"); a.keyHelp != nil || a.toast != "" {
		t.Fatalf("C-c is esc: toast %q", a.toast)
	}
	if feed(t, a, "?<C-p>"); a.keyHelp != nil || a.palette == nil {
		t.Fatal("C-p opens the palette over it")
	}
}

// ? opens it in every pane in NORMAL, a query's too, whose VISUAL ?
// stays vim's backward search; <leader>? anywhere.
func TestKeyHelpWhere(t *testing.T) {
	a := inTree(45)
	if feed(t, a, "?"); a.keyHelp == nil || !slices.Contains(helpItems(a), "t "+title("tree.open.tab")+" [keys.tree]") {
		t.Fatalf("tree: %v", helpItems(a))
	}
	a, c := inQuery(t, "", "")
	if feed(t, a, "?"); a.keyHelp == nil || !slices.Contains(helpItems(a), "↵ "+title("query.run")+" [keys.query]") {
		t.Fatalf("a query's ? in NORMAL: %v", helpItems(a))
	}
	if feed(t, a, "<Esc>v?"); a.keyHelp != nil || c.ed.Mode() != editor.Command {
		t.Fatal("a query's ? in VISUAL is vim's")
	}
	feed(t, a, "<Esc><Esc><Space>?")
	if a.keyHelp == nil || !slices.Contains(helpItems(a), "↵ "+title("query.run")+" [keys.query]") {
		t.Fatalf("SPC ? in a query: %v", helpItems(a))
	}
	feed(t, a, "<Esc><Space>x") // the query goes: ② is a landing pane
	if feed(t, a, "<C-l>?"); a.keyHelp == nil || !slices.Contains(helpItems(a), "c "+title("query.new")+" [keys.landing]") {
		t.Errorf("landing: %v", helpItems(a))
	}
}

// Too tall for the window, C-d and C-u scroll half of what shows, the
// wheel a row; a click on an item presses it, outside closes (§6.5).
func TestKeyHelpScrollAndMouse(t *testing.T) {
	a := sized(60, 12, "nerd")
	tab := loadOrders(t, a, 20)
	feed(t, a, "?")
	_, shown := a.keyHelpView().Fit(a.overlayArea(), 0)
	if feed(t, a, "<C-d>"); a.keyHelp.top != shown/2 {
		t.Fatalf("C-d: top %d of %d shown", a.keyHelp.top, shown)
	}
	a.Update(tea.MouseWheelMsg{X: 5, Y: 5, Button: tea.MouseWheelDown})
	if a.keyHelp.top != shown/2+1 {
		t.Fatalf("the wheel: top %d", a.keyHelp.top)
	}
	for range 20 {
		feed(t, a, "<C-d>")
	}
	if top, _ := a.keyHelpView().Fit(a.overlayArea(), 99); a.keyHelp.top != top {
		t.Fatalf("no further than the end: %d", a.keyHelp.top)
	}
	for range 20 {
		feed(t, a, "<C-u>")
	}
	if a.keyHelp.top != 0 {
		t.Fatalf("C-u back to the top: %d", a.keyHelp.top)
	}
	i := slices.Index(helpItems(a), "j "+title("grid.down")+" [keys.grid]")
	click(a, find(t, a, ui.Target{Kind: ui.KindItem, I: i}).Min)
	if a.keyHelp != nil || tab.row != 1 {
		t.Fatalf("a click on j: row %d", tab.row)
	}
	feed(t, a, "?")
	if click(a, uv.Pos(2, 1)); a.keyHelp == nil { // its first heading
		t.Fatal("a click in it off an item does nothing")
	}
	click(a, uv.Pos(0, a.h-1))
	if a.keyHelp != nil {
		t.Error("a click outside closes it")
	}
}

// A Ctrl leader works in the help too: <C-a>? goes back to its top, not
// to a help of the help's own keys; <C-a> alone shows which-key over it.
func TestKeyHelpCtrlLeader(t *testing.T) {
	a := configured(t, 160, 45, "[keys]\nleader = \"<C-a>\"")
	loadOrders(t, a, 3)
	feed(t, a, "?g<C-a>?")
	if h := a.keyHelp; h == nil || h.prefix != nil || !slices.Contains(helpItems(a), "j "+title("grid.down")+" [keys.grid]") {
		t.Fatalf("C-a ?: %v", helpItems(a))
	}
	feed(t, a, "<C-a>")
	due(a)
	if f := a.render().String(); !a.whichKey || !strings.Contains(f, "C-a") || !strings.Contains(f, title("session.list")) {
		t.Errorf("which-key over the help:\n%s", f)
	}
	feed(t, a, "<Esc><Esc>/<C-a>?") // over a WHERE: its keys have titles too (§6.7「标题」)
	for _, it := range a.keyHelpView().Items {
		if strings.Contains(it.Title, ".") {
			t.Errorf("a raw ID: %+v", it)
		}
	}
}

// The top level is named by the key that opens the help there (§6.5).
func TestKeyHelpTitle(t *testing.T) {
	a := configured(t, 160, 45, "[keys.grid]\n\"g?\" = \"keyhelp.open\"")
	loadOrders(t, a, 3)
	if feed(t, a, "g?"); a.keyHelp == nil || a.keyHelpView().Prefix != "g?" {
		t.Fatalf("g?: %+v", a.keyHelp)
	}
	a, c := inQuery(t, "", "")
	if feed(t, a, "<Space>?"); a.keyHelp == nil || a.keyHelpView().Prefix != "?" || c == nil {
		t.Errorf("a query's: %q", a.keyHelpView().Prefix)
	}
}

// The ? help over a grid: its tables' headings in scope order, columns
// under each, resting on the status bar (§6.5).
func TestGoldenKeyHelp160x45(t *testing.T) {
	a := configured(t, 160, 45, "[map.grid.normal]\nJ = \"5j\"")
	loadOrders(t, a, 20)
	feed(t, a, "?")
	golden.RequireEqual(t, a.render().String())
}

// The ? help opened over an overlay, which a Ctrl leader allows, is drawn over
// it and has the keys: what is typed is not in the palette's input, the title
// is the key that opened it, and no terminal cursor is left in what it covers
// (F13.14).
func TestKeyHelpOverOverlays(t *testing.T) {
	for name, open := range map[string]func(a *App){
		"palette":  func(a *App) { feed(t, a, "<C-p>") },
		"form":     func(a *App) { a.openConnForm() },
		"sessions": func(a *App) { a.run("session.list", 0) },
		"cols":     func(a *App) { a.run("grid.cols", 0) },
	} {
		a := configured(t, 100, 24, "[keys]\nleader = \"<C-a>\"") // the two boxes overlap
		loadOrders(t, a, 5)
		open(a)
		feed(t, a, "<C-a>?")
		if a.keyHelp == nil || a.topOverlay() != "keyhelp" {
			t.Errorf("%s: top %q", name, a.topOverlay())
			continue
		}
		f := a.render()
		if got := a.keyHelpView().Prefix; got != "C-a ?" || !strings.Contains(f.String(), got) { // the key that opens it there
			t.Errorf("%s: the title is %q", name, got)
		}
		if f.Cursor != nil {
			t.Errorf("%s: the terminal cursor is at %v, under the help", name, *f.Cursor)
		}
		if feed(t, a, "xyz"); a.palette != nil && a.palette.input.Text != "" {
			t.Errorf("%s: typed into the palette under it: %q", name, a.palette.input.Text)
		}
	}
}

// A question over the value editor has the keys, and no cursor is left in the
// text under it (F13.14).
func TestNoCursorUnderAQuestion(t *testing.T) {
	a, _ := valueTab(t)
	if a.render().Cursor == nil {
		t.Fatal("the value editor has no cursor")
	}
	feed(t, a, "ix")
	a.valueClose(false)
	if a.confirm == nil {
		t.Fatal("no question")
	}
	if f := a.render(); f.Cursor != nil {
		t.Errorf("the cursor is at %v, under the question", *f.Cursor)
	}
}
