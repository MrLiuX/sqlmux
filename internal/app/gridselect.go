package app

import (
	"maps"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"
)

// gridStateOf is the state of the grid pane p shows in its current tab, a
// table's or a result's, or nil: a log or a landing page has no records.
func gridStateOf(p *Pane) *gridState {
	switch t := p.tab(); {
	case t == nil:
	case t.Data != nil:
		return &t.Data.gridState
	case t.Result != nil && t.Result.page.Cols != nil:
		return &t.Result.gridState
	}
	return nil
}

// selecting is whether records are marked on s, or VISUAL is on: the
// select scope then applies (§6.4, F10.7).
func (s *gridState) selecting() bool { return s.visual || len(s.marked) > 0 }

// selected is the records selected, by position among the rows shown: those
// marked, and in VISUAL those from the anchor to the cursor.
func (s *gridState) selected() map[int]bool {
	if !s.visual {
		return s.marked
	}
	out := maps.Clone(s.marked)
	if out == nil {
		out = map[int]bool{}
	}
	for r := min(s.anchor, s.row); r <= max(s.anchor, s.row); r++ {
		out[r] = true
	}
	return out
}

// settle ends VISUAL, its range marked; deselect forgets it all.
func (s *gridState) settle()   { s.marked, s.visual = s.selected(), false }
func (s *gridState) deselect() { s.marked, s.visual = nil, false }

// positions are the records selected, in the order they show.
func (s *gridState) positions() []int {
	return slices.Sorted(maps.Keys(s.selected()))
}

// records is how many records the grid of tab has.
func records(tab *Tab) int {
	switch {
	case tab.Data != nil:
		return len(tab.Data.shownRows())
	case tab.Result != nil:
		return len(tab.Result.page.Rows)
	}
	return 0
}

// selectToggle is m: the record under the cursor marked, or not any more.
func (a *App) selectToggle() {
	p := a.focused()
	if s := gridStateOf(p); s != nil && s.row < records(p.tab()) {
		if s.marked == nil {
			s.marked = map[int]bool{}
		}
		if s.marked[s.row] {
			delete(s.marked, s.row)
		} else {
			s.marked[s.row] = true
		}
	}
}

// selectVisual is v and V: VISUAL begins at the record under the cursor, and
// the next one ends it, its range marked (F10.7).
func (a *App) selectVisual() {
	p := a.focused()
	s := gridStateOf(p)
	switch {
	case s == nil || s.row >= records(p.tab()):
	case s.visual:
		s.settle()
	default:
		s.visual, s.anchor = true, s.row
	}
}

// selectClear is esc: out of VISUAL, its range marked; with only marks, none.
func (a *App) selectClear() {
	if s := gridStateOf(a.focused()); s != nil {
		if s.visual {
			s.settle()
		} else {
			s.deselect()
		}
	}
}

// selectDelete is d on the records selected of a table (F10.7).
func (a *App) selectDelete() tea.Cmd {
	t, ok := a.focusedGrid()
	if !ok {
		return nil
	}
	return a.deleteRecords(t, t.positions())
}

// selectYank is y on the records selected: a JSON array of them, in the order
// they show, each as Y writes it; their numbers flash. Nothing is selected
// after (F10.7).
func (a *App) selectYank() tea.Cmd {
	p := a.focused()
	s := gridStateOf(p)
	if s == nil {
		return nil
	}
	var objects []string
	for _, pos := range s.positions() {
		fs, ok := jsonFields(p.tab(), pos)
		if !ok {
			continue
		}
		raw, err := rowRaw(fs)
		if err != nil {
			return a.showToast(err.Error(), toastTTL)
		}
		objects = append(objects, string(raw))
	}
	flash := yankFlash{pane: p.ID, nums: s.selected()}
	s.deselect()
	if len(objects) == 0 {
		return nil
	}
	text, err := indentJSON([]byte("[" + strings.Join(objects, ",") + "]"))
	if err != nil {
		return a.showToast(err.Error(), toastTTL)
	}
	return tea.Batch(clipCopy(text), a.flashYank(flash))
}
