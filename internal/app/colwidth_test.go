package app

import (
	"slices"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/kackerx/sqlmux/internal/db"
	"github.com/kackerx/sqlmux/internal/ui"
)

// widthsOf is how wide the focused grid draws each of its columns.
func widthsOf(t *testing.T, a *App) []int {
	t.Helper()
	_, g, area, ok := a.gridOf(a.focused())
	if !ok {
		t.Fatal("no grid")
	}
	return g.ColWidths(area)
}

// > and < set the column the cursor is on to its drawn width plus or minus the
// count, 1 at least, the others as they were; = gives them all back to automatic
// (F17.14).
func TestColWidthKeys(t *testing.T) {
	a := wide(200, 45)
	tab := loadOrders(t, a, 3)
	feed(t, a, "ll") // amount
	before := widthsOf(t, a)
	feed(t, a, "3>")
	got := widthsOf(t, a)
	if got[2] != before[2]+3 || !slices.Equal(got[:2], before[:2]) || !slices.Equal(got[3:], before[3:]) {
		t.Fatalf("3 >: %v from %v", got, before)
	}
	feed(t, a, "<")
	if got := widthsOf(t, a); got[2] != before[2]+2 {
		t.Errorf("<: %v", got)
	}
	feed(t, a, "99<")
	if got := widthsOf(t, a); got[2] != 1 {
		t.Errorf("down to 1: %v", got)
	}
	feed(t, a, ">")
	if got := widthsOf(t, a); got[2] != 2 {
		t.Errorf("up again from 1: %v", got)
	}
	feed(t, a, "l>") // another column: the first one stays
	if tab.colWidth["amount"] != 2 || tab.colWidth["paid"] == 0 {
		t.Errorf("widths by name: %v", tab.colWidth)
	}
	feed(t, a, "=")
	if tab.colWidth != nil || !slices.Equal(widthsOf(t, a), before) {
		t.Errorf("=: %v, %v", tab.colWidth, widthsOf(t, a))
	}
}

// The widths are the column's, by name: COLS hiding it and showing it again keeps it; a
// turned grid does not use them nor change them; a column gone from the table loses its
// width with the next page (F17.14).
func TestColWidthSurvives(t *testing.T) {
	a := wide(200, 45)
	tab := loadOrders(t, a, 3)
	feed(t, a, "ll>")
	set := tab.colWidth["amount"]
	feed(t, a, "fc") // hides amount
	feed(t, a, "gca<Esc>")
	if tab.colWidth["amount"] != set {
		t.Errorf("hidden and shown again: %v", tab.colWidth)
	}
	feed(t, a, "T>") // turned: nothing to set
	if tab.colWidth["amount"] != set || len(tab.colWidth) != 1 {
		t.Errorf("turned: %v", tab.colWidth)
	}
	feed(t, a, "T")
	table, cols, page := ordersTable(3)
	_ = table
	page.Cols = slices.DeleteFunc(page.Cols, func(c db.Col) bool { return c.Name == "amount" })
	for i := range page.Rows {
		page.Rows[i] = slices.Delete(page.Rows[i], 2, 3)
	}
	cols.Cols = slices.DeleteFunc(cols.Cols, func(c db.Column) bool { return c.Name == "amount" })
	a.Update(pageMsg{tab: tab, seq: tab.seq, cols: cols, page: page, next: true})
	if _, ok := tab.colWidth["amount"]; ok {
		t.Errorf("a column that is gone keeps its width: %v", tab.colWidth)
	}
}

// Dragging the │ at the right of a head sets the column on its left to the width it had plus
// what the pointer moved, 1 at least; the label column has no such edge (F17.14).
func TestColWidthDrag(t *testing.T) {
	a := wide(200, 45)
	tab := loadOrders(t, a, 3)
	before := widthsOf(t, a)
	edge := find(t, a, ui.Target{Kind: ui.KindColEdge, Pane: a.focused().ID, I: 1})
	a.Update(tea.MouseClickMsg{X: edge.Min.X, Y: edge.Min.Y, Button: tea.MouseLeft})
	a.Update(tea.MouseMotionMsg{X: edge.Min.X + 5, Y: edge.Min.Y})
	a.Update(tea.MouseReleaseMsg{X: edge.Min.X + 5, Y: edge.Min.Y, Button: tea.MouseLeft})
	if got := tab.colWidth["status"]; got != before[1]+5 || a.dragCol != nil {
		t.Errorf("dragged 5 right: %v from %v, drag %v", tab.colWidth, before, a.dragCol)
	}
	edge = find(t, a, ui.Target{Kind: ui.KindColEdge, Pane: a.focused().ID, I: 1})
	a.Update(tea.MouseClickMsg{X: edge.Min.X, Y: edge.Min.Y, Button: tea.MouseLeft})
	a.Update(tea.MouseMotionMsg{X: edge.Min.X - 60, Y: edge.Min.Y})
	a.Update(tea.MouseReleaseMsg{X: edge.Min.X - 60, Y: edge.Min.Y, Button: tea.MouseLeft})
	if tab.colWidth["status"] != 1 {
		t.Errorf("dragged far left: %v", tab.colWidth)
	}
	a.View()
	for _, h := range a.hits {
		if h.Target.Kind == ui.KindColEdge && h.Target.I < 0 {
			t.Errorf("an edge of the label column: %+v", h)
		}
	}
}

// A result tab sets its columns the same way (F17.14).
func TestColWidthInResult(t *testing.T) {
	r := db.Result{Cols: []db.Col{{Name: "a"}, {Name: "b"}}, Rows: [][]db.Val{{{S: "1"}, {S: "2"}}}}
	a, _ := inRun(t, &execDB{res: map[string]db.Result{"select 1": r}}, "select 1;")
	press(t, a, "<CR>")
	a.win().focus(a.win().Result.ID)
	before := widthsOf(t, a)
	edge := find(t, a, ui.Target{Kind: ui.KindColEdge, Pane: a.focused().ID, I: 0}) // the edge of a result's head, dragged
	a.Update(tea.MouseClickMsg{X: edge.Min.X, Y: edge.Min.Y, Button: tea.MouseLeft})
	a.Update(tea.MouseMotionMsg{X: edge.Min.X + 4, Y: edge.Min.Y})
	a.Update(tea.MouseReleaseMsg{X: edge.Min.X + 4, Y: edge.Min.Y, Button: tea.MouseLeft})
	if got := resultOf(a.focused()).colWidth["a"]; got != before[0]+4 {
		t.Errorf("dragged: %d from %v", got, before)
	}
	feed(t, a, "=")
	before = widthsOf(t, a)
	feed(t, a, ">")
	rt := resultOf(a.focused())
	if got := widthsOf(t, a); got[0] != before[0]+1 || rt.colWidth["a"] != before[0]+1 {
		t.Errorf("%v from %v", got, before)
	}
	feed(t, a, "=")
	if rt.colWidth != nil {
		t.Errorf("=: %v", rt.colWidth)
	}
}

// The column follows the pointer as it is dragged: after each move the frame is drawn again
// and the │ is where the pointer is, not reused from before it moved (F17.14).
func TestColWidthDragFollowsThePointer(t *testing.T) {
	a := wide(200, 45)
	loadOrders(t, a, 3)
	id := a.focused().ID
	edge := find(t, a, ui.Target{Kind: ui.KindColEdge, Pane: id, I: 1})
	a.Update(tea.MouseClickMsg{X: edge.Min.X, Y: edge.Min.Y, Button: tea.MouseLeft})
	a.Update(tea.MouseMotionMsg{X: edge.Min.X, Y: edge.Min.Y}) // the pointer is known to be here
	a.View()
	if a.stillOver(tea.MouseMotionMsg{X: edge.Min.X, Y: edge.Min.Y}) { // a move that crosses no region still changes what the frame shows
		t.Error("a pointer moving with a column dragged is taken for no change")
	}
	for _, dx := range []int{1, 3, 6} {
		a.View()
		a.Update(tea.MouseMotionMsg{X: edge.Min.X + dx, Y: edge.Min.Y})
		if a.reuse {
			t.Fatalf("after a move of %d the last frame is reused", dx)
		}
		if got := find(t, a, ui.Target{Kind: ui.KindColEdge, Pane: id, I: 1}); got.Min.X != edge.Min.X+dx {
			t.Errorf("moved %d: the │ is at %d, want %d", dx, got.Min.X, edge.Min.X+dx)
		}
	}
	a.Update(tea.MouseReleaseMsg{X: edge.Min.X + 6, Y: edge.Min.Y, Button: tea.MouseLeft})
}

// Dragging the edge of a grid in another pane takes the focus there, as a click on its head
// does.
func TestColWidthDragFocusesItsPane(t *testing.T) {
	a := sized(160, 45, "nerd")
	loadOrders(t, a, 3)
	grid := a.focused().ID
	a.win().focus(2) // the query's pane
	if a.focused().ID == grid {
		t.Fatal("no other pane")
	}
	edge := find(t, a, ui.Target{Kind: ui.KindColEdge, Pane: grid, I: 0})
	a.Update(tea.MouseClickMsg{X: edge.Min.X, Y: edge.Min.Y, Button: tea.MouseLeft})
	if a.focused().ID != grid {
		t.Errorf("focus is in pane %d, want %d", a.focused().ID, grid)
	}
	a.Update(tea.MouseReleaseMsg{X: edge.Min.X, Y: edge.Min.Y, Button: tea.MouseLeft})
}
