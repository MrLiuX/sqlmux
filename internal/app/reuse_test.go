package app

import (
	"math/rand/v2"
	"runtime"
	"testing"

	tea "charm.land/bubbletea/v2"

	"sqlmux/internal/ui"
)

// What View shows, whatever the frames before it were: the same as a frame
// drawn alone, over fresh buffers and with no line reused, through keys, a
// pointer wandering over the tabs, buttons and rows, the palette's list and
// a window resize (F8.20).
func TestViewIsWhatAFreshFrameIs(t *testing.T) {
	a := wide(160, 45)
	loadOrders(t, a, 100)
	feed(t, a, "<C-p>@t_user<CR><C-p>@t_sku<CR>1gt") // tabs to hover, with a × on them
	rng := rand.New(rand.NewPCG(1, 2))
	check := func(step string) {
		t.Helper()
		if got, want := a.View().Content, a.render().Render(); got != want {
			t.Fatalf("%s: View is not what a frame drawn alone is", step)
		}
	}
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
	if got, want := a.View().Content, a.render().Render(); got != want || a.palette.sel != 2 {
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
