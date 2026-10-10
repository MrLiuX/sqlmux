package app

import (
	"fmt"
	"math/rand/v2"
	"runtime"
	"testing"

	tea "charm.land/bubbletea/v2"
	uv "github.com/charmbracelet/ultraviolet"

	"github.com/kackerx/sqlmux/internal/ui"
)

// render is the frame drawn alone, over a fresh buffer and with no line
// reused: what View's frames are compared with (F8.20).
func (a *App) render() *ui.Frame { return a.drawOn(ui.NewFrame(a.w, a.h, a.theme)) }

// sameAsFresh fails unless View's content and cursor are those of a frame
// drawn alone.
func sameAsFresh(t *testing.T, a *App, step string) {
	t.Helper()
	v, f := a.View(), a.render()
	if want := f.Buf.Render(); v.Content != want {
		t.Fatalf("%s: View is not what a frame drawn alone is (reused: %v)", step, a.reuse)
	}
	if (v.Cursor == nil) != (f.Cursor == nil) || v.Cursor != nil && (v.Cursor.X != f.Cursor.X || v.Cursor.Y != f.Cursor.Y) {
		t.Fatalf("%s: cursor %v, fresh %v", step, v.Cursor, f.Cursor)
	}
}

// What View shows, whatever the frames before it were: the same as a frame
// drawn alone, over fresh buffers and with no line reused, through keys, a
// pointer wandering over the tabs, buttons and rows, the palette's list and
// a window resize (F8.20).
func TestViewIsWhatAFreshFrameIs(t *testing.T) {
	a := wide(160, 45)
	loadOrders(t, a, 100)
	feed(t, a, "<C-p>@t_user<CR><C-p>@t_sku<CR>1gt") // tabs to hover, with a × on them
	rng := rand.New(rand.NewPCG(1, 2))
	check := func(step string) { sameAsFresh(t, a, step) }
	check("first")
	// the pointer along the rows of the query bar, the grid's header and the tab bar, a cell at a time
	for _, y := range []int{1, 2, 3, 4, 5, 6, 41, 42, 43, 44} {
		for x := 30; x < 100; x++ {
			a.Update(tea.MouseMotionMsg{X: x, Y: y})
			check("raster")
		}
	}
	steps := []tea.Msg{
		teaKey("j"), teaKey("l"), teaKey("<C-p>"), teaKey("<Esc>"), tea.WindowSizeMsg{Width: 120, Height: 40},
		teaKey("<C-p>"), teaKey("a"), tea.WindowSizeMsg{Width: 160, Height: 45}, teaKey("<Esc>"),
	}
	for i := range 600 {
		var msg tea.Msg = tea.MouseMotionMsg{X: rng.IntN(a.w), Y: rng.IntN(a.h)}
		if i%7 == 0 { // a small step, within what it was over
			msg = tea.MouseMotionMsg{X: a.mouse.X + rng.IntN(3) - 1, Y: a.mouse.Y}
		}
		if i%50 == 49 {
			msg = steps[(i/50)%len(steps)]
		}
		a.Update(msg)
		check("step")
	}
}

// The pointer moving inside what the frame has under it is not drawn again;
// across an edge of any of it, it is (F8.20).
func TestPointerWithinARowDrawsNothing(t *testing.T) {
	a := wide(160, 45)
	loadOrders(t, a, 100)
	a.Update(tea.MouseMotionMsg{X: 40, Y: 10})
	first := a.View()
	a.Update(tea.MouseMotionMsg{X: 41, Y: 10})
	if !a.reuse {
		t.Fatal("a move along a row asks for a new frame")
	}
	if v := a.View(); v.Content != first.Content {
		t.Error("the frame changed")
	}
	a.Update(tea.MouseMotionMsg{X: 41, Y: 11})
	if a.reuse {
		t.Error("the next row has its own color: a new frame")
	}
	if a.View().Content == first.Content {
		t.Error("the hover did not move")
	}
	// anything else that happens makes the next frame new, the pointer at rest or not
	a.Update(tea.MouseMotionMsg{X: 42, Y: 11})
	a.Update(teaKey("j"))
	a.Update(tea.MouseMotionMsg{X: 43, Y: 11})
	if a.reuse {
		t.Error("a key came between: the frame was not seen since")
	}
}

// A list under the pointer follows it, and what moved the selection since is
// taken back by the pointer moving inside the row it was over: the frame is
// drawn again (F8.20).
func TestPointerInAListTakesTheSelection(t *testing.T) {
	a := wide(160, 45)
	feed(t, a, "<C-p>@t_")
	row := func(i int) tea.MouseMotionMsg {
		r := find(t, a, ui.Target{Kind: ui.KindRow, I: i})
		return tea.MouseMotionMsg{X: r.Min.X + 4, Y: r.Min.Y}
	}
	a.Update(row(2))
	a.View()
	if a.palette.sel != 2 {
		t.Fatalf("hover: %d", a.palette.sel)
	}
	feed(t, a, "<Down>")
	a.View()
	m := row(2)
	m.X++
	a.Update(m)
	if got, want := a.View().Content, a.render().Buf.Render(); got != want || a.palette.sel != 2 {
		t.Errorf("selection %d, frame is not the one drawn alone: %v", a.palette.sel, got != want)
	}
}

// A frame is drawn in the buffer of the one before the last, not a new one:
// a third of a frame's cost was its garbage (F8.20).
func TestFramesReuseTheirBuffers(t *testing.T) {
	a := wide(160, 45)
	loadOrders(t, a, 100)
	a.View()
	frame := func(y int) {
		a.Update(tea.MouseMotionMsg{X: 90, Y: y})
		a.View()
	}
	frame(6)
	frame(7)
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	for i := range 40 {
		frame(8 + i%20)
	}
	runtime.ReadMemStats(&after)
	if per := (after.TotalAlloc - before.TotalAlloc) / 40; per > 600<<10 {
		t.Errorf("%d KB a frame; a new buffer is 1100", per>>10)
	}
}

// hitRect is the first region of kind in the frame last drawn.
func hitRect(t *testing.T, a *App, kind ui.Kind) uv.Rectangle {
	t.Helper()
	a.View()
	for _, h := range a.hits {
		if h.Target.Kind == kind {
			return h.Rect
		}
	}
	t.Fatalf("no region of kind %d", kind)
	return uv.Rectangle{}
}

// dragFrames presses at p and moves a cell at a time, by (dx, dy) every
// step, letting go at the end: every frame is the one drawn alone. What a
// drag does as the pointer moves, a frame reusing the last is not told of.
func dragFrames(t *testing.T, a *App, name string, p uv.Position, dx, dy, n int) {
	t.Helper()
	a.Update(tea.MouseClickMsg{X: p.X, Y: p.Y, Button: tea.MouseLeft})
	sameAsFresh(t, a, name+" down")
	for i := range n {
		p.X, p.Y = p.X+dx, p.Y+dy
		a.Update(tea.MouseMotionMsg{X: p.X, Y: p.Y, Button: tea.MouseLeft})
		sameAsFresh(t, a, fmt.Sprintf("%s step %d at %v", name, i, p))
	}
	a.Update(tea.MouseReleaseMsg{X: p.X, Y: p.Y, Button: tea.MouseLeft})
	sameAsFresh(t, a, name+" up")
}

// The four drags, none of which a pointer inside what it is over may leave
// to a frame reused: a border (out of the screen too), the sidebar's edge, a
// selection in a query, one in the WHERE (F8.20).
func TestDragsAreDrawnFresh(t *testing.T) {
	a := sized(110, 32, "nerd")
	for _, d := range []struct {
		kind   ui.Kind
		dx, dy int
	}{{ui.KindBorder, 1, 0}, {ui.KindBorder, -1, 0}, {ui.KindBorder, 0, 1}, {ui.KindTreeEdge, 1, 0}, {ui.KindTreeEdge, -1, 0}} {
		dragFrames(t, a, fmt.Sprint("kind ", d.kind, d.dx, d.dy), hitRect(t, a, d.kind).Min, d.dx, d.dy, 30)
	}
	// uneven steps, off the screen and back
	rng := rand.New(rand.NewPCG(11, 12))
	for _, kind := range []ui.Kind{ui.KindBorder, ui.KindTreeEdge} {
		p := hitRect(t, a, kind).Min
		a.Update(tea.MouseClickMsg{X: p.X, Y: p.Y, Button: tea.MouseLeft})
		for i := range 60 {
			p.X, p.Y = p.X+rng.IntN(15)-7, p.Y+rng.IntN(9)-4
			a.Update(tea.MouseMotionMsg{X: p.X, Y: p.Y, Button: tea.MouseLeft})
			sameAsFresh(t, a, fmt.Sprint("uneven ", kind, " step ", i))
		}
		a.Update(tea.MouseReleaseMsg{X: p.X, Y: p.Y, Button: tea.MouseLeft})
	}

	c, _ := inQuery(t, "", "")
	c.Update(tea.WindowSizeMsg{Width: 110, Height: 32})
	feed(t, c, "iselect id, status, amount from t_order where id = 1<CR>and status = 'done'<Esc>")
	c.View()
	var r uv.Rectangle
	for _, h := range c.hits {
		if h.Target.Kind == ui.KindText && h.Target.Pane == c.win().Focus {
			r = h.Rect
		}
	}
	if r.Empty() {
		t.Fatal("no query text")
	}
	dragFrames(t, c, "query text", uv.Pos(r.Min.X+2, r.Min.Y), 1, 0, 30)
	dragFrames(t, c, "query text down", uv.Pos(r.Min.X+2, r.Min.Y), 1, 1, 20)

	w := sizedWith(110, 32, userConfig(t, "vim = false", ""))
	w.win().focus(1)
	tab := loadOrders(t, w, 3)
	feed(t, w, "/id = 1 and status = 'done'")
	w.View()
	pn := w.focused()
	in := w.queryBar(pn, tab).InputRect(bodyRect(w.layout()[pn.ID]))
	dragFrames(t, w, "where", uv.Pos(in.Min.X+1, in.Min.Y), 1, 0, 20)
}

// The window resized in one dimension, the buffer spared by the frames before
// being of the other size (F8.20).
func TestResizeOneDimensionIsDrawnFresh(t *testing.T) {
	a := wide(100, 30)
	loadOrders(t, a, 100)
	for _, s := range []tea.WindowSizeMsg{{Width: 100, Height: 30}, {Width: 90, Height: 30}, {Width: 90, Height: 30}, {Width: 90, Height: 25}, {Width: 90, Height: 25}, {Width: 100, Height: 25}} {
		a.Update(s)
		sameAsFresh(t, a, fmt.Sprintf("%dx%d", s.Width, s.Height))
		a.Update(tea.MouseMotionMsg{X: 10, Y: 10})
		sameAsFresh(t, a, fmt.Sprintf("%dx%d moved", s.Width, s.Height))
	}
}

// The × of a tab shows when the pointer is over its tab, and over the cell
// after one with no mark, which its blank takes: a pointer coming into that
// cell from above or below shows it too (F8.20).
func TestTabCloseFollowsThePointerVertically(t *testing.T) {
	a := wide(120, 30)
	loadOrders(t, a, 100)
	feed(t, a, "<C-p>@t_user<CR><C-p>@t_sku<CR><C-p>@t_refund<CR>1gt")
	a.View()
	var ys []int
	for _, h := range a.hits {
		if h.Target.Kind == ui.KindTab {
			ys = append(ys, h.Rect.Min.Y)
		}
	}
	if len(ys) == 0 {
		t.Fatal("no tabs")
	}
	for x := range a.w {
		for _, y := range []int{ys[0] - 1, ys[0], ys[0] + 1, ys[0]} {
			a.Update(tea.MouseMotionMsg{X: x, Y: y})
			sameAsFresh(t, a, fmt.Sprintf("tabs %d,%d", x, y))
		}
	}
}
