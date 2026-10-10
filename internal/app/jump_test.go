package app

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/exp/golden"

	"github.com/kackerx/sqlmux/internal/keymap"
	"github.com/kackerx/sqlmux/internal/ui"
)

// matchLabels are the labels of a's matches, nearest first, with the cell or
// node each is in.
func matchLabels(a *App) (labels, actions []string) {
	for _, m := range a.jump.matches {
		labels, actions = append(labels, m.label), append(actions, m.target.Action)
	}
	return labels, actions
}

// s begins a jump on a table in NORMAL: the mode block says JUMP and the keys
// are its pattern; a pattern is found in the cells, smartcase, once to a
// cell, the nearest to the cursor first, and the labels leave out the
// letter that follows a match; a label goes there and ends the jump; ↵ goes to
// the nearest; backspace to nothing stays in the jump; esc leaves it with the
// cursor where it was (F10.8).
func TestJumpInTable(t *testing.T) {
	a := wide(160, 45)
	tab := loadOrders(t, a, 6)
	feed(t, a, "s")
	if a.jump == nil || a.mode() != keymap.Insert || a.context().Overlay != "jump" || !strings.Contains(statusRow(a), "JUMP") {
		t.Fatalf("not in a jump: %+v, mode %v, status %q", a.jump, a.mode(), statusRow(a))
	}
	feed(t, a, "run") // running in the first row and the fifth; n follows it
	labels, actions := matchLabels(a)
	if strings.Join(labels, "") != "as" || strings.Join(actions, "|") != "grid.goto 0 1|grid.goto 4 1" {
		t.Fatalf("matches of run: %v in %v, the nearest first and no n", labels, actions)
	}
	feed(t, a, "<BS><BS><BS>") // one more than there is
	if a.jump == nil || a.jump.pattern != "" || len(a.jump.matches) != 0 {
		t.Errorf("backspace to nothing: %+v", a.jump)
	}
	feed(t, a, "Run")
	if len(a.jump.matches) != 0 {
		t.Errorf("a capital makes it exact: %d matches of Run", len(a.jump.matches))
	}
	feed(t, a, "<BS><BS><BS>run<Esc>")
	if a.jump != nil || tab.row != 0 || tab.col != 0 || a.mode() != keymap.Normal {
		t.Errorf("esc leaves, the cursor stays: jump %v at %d, %d", a.jump, tab.row, tab.col)
	}
	feed(t, a, "srun<CR>") // the nearest
	if a.jump != nil || tab.row != 0 || tab.col != 1 {
		t.Errorf("↵: jump %v at %d, %d", a.jump, tab.row, tab.col)
	}
	feed(t, a, "srun")
	feed(t, a, "s")
	if a.jump != nil || tab.row != 4 || tab.col != 1 {
		t.Errorf("the label s: jump %v at %d, %d", a.jump, tab.row, tab.col)
	}
}

// The labels go to the matches of the focused pane first, the nearest to the
// cursor first, the others after as the screen goes; a cell has one match,
// its first, whatever its text holds (F10.8).
func TestJumpOrder(t *testing.T) {
	a := wide(160, 45)
	loadOrders(t, a, 6)
	feed(t, a, "4jsrun") // the cursor on the fifth row: its running is nearer than the first's
	if labels, actions := matchLabels(a); strings.Join(labels, "") != "as" || strings.Join(actions, "|") != "grid.goto 4 1|grid.goto 0 1" {
		t.Errorf("nearest first: %v in %v", labels, actions)
	}
	feed(t, a, "<Esc>sdo") // done in the table, doraemon in the tree, which comes first on the screen
	var kinds []ui.Kind
	for _, m := range a.jump.matches {
		kinds = append(kinds, m.target.Kind)
	}
	if len(kinds) != 3 || kinds[0] != ui.KindCell || kinds[1] != ui.KindCell || kinds[2] != ui.KindNode {
		t.Errorf("the focused pane's first: %v", kinds)
	}
	feed(t, a, "<BS><BS>line") // "line one↵line two" twice in a cell: once
	seen := map[string]int{}
	for _, m := range a.jump.matches {
		seen[m.target.Action]++
	}
	if len(seen) == 0 {
		t.Fatal("no line")
	}
	for action, n := range seen {
		if n != 1 {
			t.Errorf("%d matches in %s", n, action)
		}
	}
}

// The screen as a jump leaves it: all of it dim, the matches in the match
// color, the label over the character after a match in the yank's colors
// (F10.8).
func TestJumpDrawn(t *testing.T) {
	a := wide(160, 45)
	loadOrders(t, a, 6)
	feed(t, a, "srun")
	f := a.render()
	th := a.theme
	for x := range a.w { // JUMP, in the info color, in the mode block
		if c := f.Buf.CellAt(x, a.h-1); c.Content == "J" && f.Buf.CellAt(x+1, a.h-1).Content == "U" {
			if c.Style.Bg != th.Info {
				t.Errorf("the mode block is %v, want info %v", c.Style.Bg, th.Info)
			}
			break
		}
	}
	m := a.jump.matches[0]
	if got := f.Buf.CellAt(m.at.X, m.at.Y).Style.Fg; got != th.Match {
		t.Errorf("the matched d is %v, want match %v", got, th.Match)
	}
	if c := f.Buf.CellAt(m.at.X+m.width, m.at.Y); c.Content != m.label || c.Style.Bg != th.Yank || c.Style.Fg != th.Bg {
		t.Errorf("the label over the n after: %+v", c)
	}
	if got := f.Buf.CellAt(m.at.X+m.width+1, m.at.Y).Style.Fg; got != th.Dim {
		t.Errorf("the rest is %v, want dim %v", got, th.Dim)
	}
	golden.RequireEqual(t, f.String())
}

// The nodes of the tree are jumped to: the cursor goes to the node and the
// tree has the focus, no table is opened and no node unfolded (F10.8).
func TestJumpInTree(t *testing.T) {
	a := sized(160, 45, "nerd")
	data := a.focused()
	tabs := len(data.Tabs)
	a.win().focus(0)
	feed(t, a, "sT_sku")
	if len(a.jump.matches) != 0 {
		t.Fatalf("a capital T is exact: %d matches", len(a.jump.matches))
	}
	feed(t, a, "<BS><BS><BS><BS><BS><BS>t_sku")
	if len(a.jump.matches) != 1 || a.jump.matches[0].target.Kind != ui.KindNode {
		t.Fatalf("matches of t_sku: %+v", a.jump.matches)
	}
	a.win().focus(data.ID) // from elsewhere, the label takes the focus to the tree
	feed(t, a, a.jump.matches[0].label)
	if n, _ := a.treeNode(); n.Text != "t_sku" || a.win().Focus != 0 || len(data.Tabs) != tabs {
		t.Errorf("the cursor is on %q, focus %d, %d tabs", n.Text, a.win().Focus, len(data.Tabs))
	}
	feed(t, a, "s")
	if a.jump == nil {
		t.Fatal("s in the tree begins a jump")
	}
}

// A console's text is matched everywhere, a letter at a time, and a label puts
// the cursor on the character in its pane; s is the jump's in NORMAL and the
// vim's in VISUAL (F10.8).
func TestJumpInConsole(t *testing.T) {
	a := sized(160, 45, "nerd")
	loadOrders(t, a, 3)
	c := consoleOf(a.win().Root.B.Pane)
	c.ed.Load("select 1;\nselect 2;")
	feed(t, a, "sct") // from the table: matches in the console too
	var in []int
	for _, m := range a.jump.matches {
		if m.target.Kind == ui.KindText {
			in = append(in, m.at.Y)
		}
	}
	if len(in) != 2 {
		t.Fatalf("matches of ct in the console: %d", len(in))
	}
	feed(t, a, a.jump.matches[len(a.jump.matches)-1].label) // the last on the screen
	cur := c.ed.Cursor()
	if a.focused() != a.win().Root.B.Pane || cur.Line != 1 || cur.Col != 4 {
		t.Errorf("focus %d, the cursor at %+v", a.win().Focus, cur)
	}
	if feed(t, a, "<Esc>s"); a.jump == nil {
		t.Fatal("s in the console's NORMAL begins a jump")
	}
	feed(t, a, "<Esc>")
	feed(t, a, "vs") // VISUAL: the vim's s
	if a.jump != nil {
		t.Error("s in VISUAL is the editor's")
	}
}

// A click or the wheel puts the jump away, the click going where it is (F10.8).
func TestJumpLeftByClickAndWheel(t *testing.T) {
	a := wide(160, 45)
	loadOrders(t, a, 3)
	feed(t, a, "srun")
	click(a, a.jump.matches[0].at)
	if a.jump != nil {
		t.Error("a click ends it")
	}
	feed(t, a, "sd")
	a.Update(tea.MouseWheelMsg{X: 70, Y: 10, Button: tea.MouseWheelDown})
	if a.jump != nil {
		t.Error("the wheel ends it")
	}
	feed(t, a, "sd")
	a.Update(tea.WindowSizeMsg{Width: 150, Height: 40})
	if a.jump != nil {
		t.Error("a resize ends it")
	}
}
