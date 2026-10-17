package app

import (
	"image/color"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/exp/golden"

	"github.com/kackerx/sqlmux/internal/editor"
	"github.com/kackerx/sqlmux/internal/keymap"
	"github.com/kackerx/sqlmux/internal/ui"
)

// previewApp is the palette open on t_order with its DDL in the preview.
func previewApp(t *testing.T) *App {
	t.Helper()
	a := wide(160, 45)
	long := ddlOrder + "\n" + strings.Repeat("create index i on t_order (id);\n", 20) + "create index last_one on t_order (note);"
	feed(t, a, "<C-p>@t_order")
	rest(a)
	a.Update(ddlMsg{conn: a.sess.Conns[0], table: tableNamed(a, "t_order"), text: long})
	return a
}

// C-o puts the focus in the preview, whose text is a read only vim: j moves in
// it, v selects, / searches, y copies and the list and the input are not
// touched; esc goes back to the list, a search lit or a selection first, as does
// :q; a candidate that changes takes the focus back too (F17.6).
func TestPalettePreviewFocus(t *testing.T) {
	a := previewApp(t)
	p := a.palette
	input := p.input.Text
	feed(t, a, "<C-o>")
	ed := p.pvEd
	if ed == nil || ed.Mode() != editor.Normal || !ed.ReadOnly || a.mode() != keymap.Normal {
		t.Fatalf("C-o: %+v", ed)
	}
	// the keys of the panes under it are none of its: no window, tab, help
	windows := len(a.sess.Windows)
	feed(t, a, "<Space>cgt?")
	if len(a.sess.Windows) != windows || a.keyHelp != nil || p.pvEd != ed {
		t.Errorf("a key that is the panes': %d windows, help %v", len(a.sess.Windows), a.keyHelp != nil)
	}
	feed(t, a, "<Esc>") // the ? was a search of the editor's, which esc puts away
	feed(t, a, "jjv")
	if c := ed.Cursor(); c.Line != 2 || ed.Mode() != editor.Visual || p.input.Text != input || p.sel != 0 {
		t.Errorf("jj v: cursor %+v mode %v input %q sel %d", c, ed.Mode(), p.input.Text, p.sel)
	}
	if a.mode() != keymap.Visual {
		t.Errorf("the mode block: %v", a.mode())
	}
	feed(t, a, "<Esc>")
	if p.pvEd == nil || ed.Mode() != editor.Normal {
		t.Fatalf("esc in VISUAL: focus %v mode %v", p.pvEd != nil, ed.Mode())
	}
	// a search lit goes first
	feed(t, a, "/last<CR>")
	if !ed.Lit() {
		t.Fatal("no search lit")
	}
	feed(t, a, "<Esc>")
	if p.pvEd == nil || ed.Lit() {
		t.Fatalf("esc with a search lit: focus %v, lit %v", p.pvEd != nil, ed.Lit())
	}
	feed(t, a, "<Esc>")
	if p.pvEd != nil || p.input.Text != input {
		t.Fatalf("esc: focus %v, input %q", p.pvEd != nil, p.input.Text)
	}
	feed(t, a, "<C-n>") // the list has the keys again
	if p.sel != 1 {
		t.Errorf("C-n: sel %d", p.sel)
	}
	feed(t, a, "<C-p><C-o>")
	if p.pvEd == nil {
		t.Fatal("not back in")
	}
	// y copies, to the register with a flash, "+y to the system's clipboard
	feed(t, a, "yy")
	if a.flash == nil || a.flash.text.Mode != ui.SelLines {
		t.Errorf("yy: flash %+v", a.flash)
	}
	if got, ok := clipText(keys(t, a, "\"+yy")); !ok || got != "create table public.t_order (\n" {
		t.Errorf(`"+yy: %q`, got)
	}
	// :q goes back; another candidate takes the focus back at once
	feed(t, a, ":q<CR>")
	if p.pvEd != nil {
		t.Error(":q")
	}
	// a window with no room for the preview takes the focus back to the list
	feed(t, a, "<C-o>")
	if p.pvEd == nil {
		t.Fatal("not in")
	}
	a.Update(tea.WindowSizeMsg{Width: 160, Height: 14})
	if a.previewEd() != nil || p.pvEd != nil {
		t.Error("no room for the preview")
	}
	a.Update(tea.WindowSizeMsg{Width: 160, Height: 45})
	feed(t, a, "<C-o>")
	a.paletteMove(1)
	a.wantDDL()
	if p.pvEd != nil {
		t.Error("another candidate")
	}
	// no preview, nothing to put the focus in
	feed(t, a, "<C-u>>")
	feed(t, a, "<C-o>")
	if p.pvEd != nil {
		t.Error("C-o with no preview")
	}
}

// The wheel scrolls the preview over it, the list over the list; with the focus in
// the preview it scrolls the editor (F17.6).
func TestPalettePreviewWheel(t *testing.T) {
	a := previewApp(t)
	_, ms := a.paletteMatches()
	_, rows, grid := a.paletteBox(len(ms))
	row0 := find(t, a, ui.Target{Kind: ui.KindRow, I: 0}).Min.Add(uv.Pos(3, 0))
	wheel := func(x, y int, b tea.MouseButton) { a.View(); a.Update(tea.MouseWheelMsg{X: x, Y: y, Button: b}) }
	wheel(grid.Min.X+3, grid.Min.Y+2, tea.MouseWheelDown)
	if a.palette.pvTop != wheelStep || a.palette.sel != 0 {
		t.Errorf("over the preview: top %d, sel %d", a.palette.pvTop, a.palette.sel)
	}
	wheel(grid.Min.X+3, grid.Min.Y+2, tea.MouseWheelUp)
	if a.palette.pvTop != 0 {
		t.Errorf("and back: %d", a.palette.pvTop)
	}
	wheel(row0.X, row0.Y, tea.MouseWheelDown)
	if a.palette.sel != min(wheelStep, len(ms)-1) || len(ms) < 2 || a.palette.pvTop != 0 {
		t.Errorf("over the list: sel %d, top %d, %d candidates in %d rows", a.palette.sel, a.palette.pvTop, len(ms), rows)
	}
	wheel(row0.X, row0.Y, tea.MouseWheelUp)
	a.wantDDL()
	feed(t, a, "<C-o>")
	ed := a.palette.pvEd
	wheel(grid.Min.X+3, grid.Min.Y+2, tea.MouseWheelDown)
	if ed.Top() != wheelStep || a.palette.pvTop != 0 {
		t.Errorf("focused: editor top %d, preview top %d", ed.Top(), a.palette.pvTop)
	}
}

func TestGoldenPalettePreviewFocus(t *testing.T) {
	a := previewApp(t)
	feed(t, a, "<C-o>jj")
	golden.RequireEqual(t, a.render().String())
}

// With the focus in the preview the terminal's cursor is the editor's, and the text
// is in SQL's colors as it is when the list has the focus (F17.6).
func TestPalettePreviewFocusDrawsCursorAndColors(t *testing.T) {
	a := previewApp(t)
	grid := a.previewRect()
	word := func(f *ui.Frame, from int) (fg color.Color) { // the color of the first "create" in the preview's rows
		for x := from; x < grid.Max.X-6; x++ {
			if f.Buf.CellAt(x, grid.Min.Y).Content == "c" && f.Buf.CellAt(x+1, grid.Min.Y).Content == "r" && f.Buf.CellAt(x+5, grid.Min.Y).Content == "e" {
				return f.Buf.CellAt(x, grid.Min.Y).Style.Fg
			}
		}
		return nil
	}
	list := word(a.render(), grid.Min.X)
	if list == nil || list == a.theme.Fg {
		t.Fatalf("the keyword is in the plain color in the list's preview: %v", list)
	}
	input := a.render().Cursor
	feed(t, a, "<C-o>")
	f := a.render()
	if got := word(f, grid.Min.X); got != list {
		t.Errorf("focused: the keyword is %v, was %v", got, list)
	}
	if f.Cursor == nil || f.Cursor.Y != grid.Min.Y || input == nil || f.Cursor.Y == input.Y {
		t.Fatalf("the cursor: %v, the input's was %v", f.Cursor, input)
	}
	feed(t, a, "jj")
	if c := a.render().Cursor; c == nil || c.Y != grid.Min.Y+2 {
		t.Errorf("two lines down: %v", c)
	}
}
