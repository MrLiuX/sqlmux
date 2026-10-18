package app

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	uv "github.com/charmbracelet/ultraviolet"

	"github.com/kackerx/sqlmux/internal/db"
	"github.com/kackerx/sqlmux/internal/i18n"
	"github.com/kackerx/sqlmux/internal/keymap"
	"github.com/kackerx/sqlmux/internal/ui"
)

// colsMenu is the open COLS list (Q-04, §7.8): which of a table's columns
// the grid shows. The keys are the list's until / gives them to the filter.
// The ORDER list is the same list with the way each column is sorted by in
// place of the boxes (F12.9).
type colsMenu struct {
	tab      *dataTab
	pane     *Pane
	filter   ui.Input
	typing   bool      // the filter has the keys
	order    bool      // the ORDER list
	pick     *pickMenu // a tree node's databases or schemas, in this list's place (F17.13)
	sel, top int
}

// scope is the keymap table of the list's keys.
func (m *colsMenu) scope() string {
	if m.order {
		return "order"
	}
	return "cols"
}

// openCols opens the COLS list, or with order the ORDER list, its cursor on
// the column sorted by first.
func (a *App) openCols(order bool) {
	p := a.focused()
	t := dataOf(p)
	if t == nil || len(t.page.Cols) == 0 {
		return
	}
	a.cols = &colsMenu{tab: t, pane: p, order: order}
	if order && len(t.order) > 0 {
		a.cols.sel = slices.IndexFunc(t.ordered(), func(f int) bool { return t.page.Cols[f].Name == t.order[0].Col })
		a.colsMove(0)
	}
}

// colsMatches is the columns that pass the filter, in COLS's order; Index is
// the column's in the page's.
func (a *App) colsMatches() []ui.Match {
	if p := a.cols.pick; p != nil {
		ms := ui.Filter(a.cols.filter.Text, p.names)
		slices.SortFunc(ms, func(x, y ui.Match) int { return x.Index - y.Index })
		return ms
	}
	t := a.cols.tab
	o := t.ordered()
	names := make([]string, len(o))
	for i, f := range o {
		names[i] = t.page.Cols[f].Name
	}
	ms := ui.Filter(a.cols.filter.Text, names)
	slices.SortFunc(ms, func(x, y ui.Match) int { return x.Index - y.Index })
	for i := range ms {
		ms[i].Index = o[ms[i].Index]
	}
	return ms
}

// colsBox is where v, the list as drawn, opens: under the COLS chip.
func (a *App) colsBox(v ui.Dropdown) (uv.Rectangle, int) {
	m := a.cols
	if m.pick != nil { // under the tree's top, where the node is
		r := bodyRect(a.layout()[m.pane.ID])
		w := 30
		for i, n := range m.pick.names {
			w = max(w, ui.Width(n+"  "+m.pick.note(i))+10)
		}
		return v.Box(a.window(), uv.Rect(r.Min.X, r.Min.Y, 1, 1), min(w, r.Dx()+2))
	}
	w := 30
	for _, c := range m.tab.page.Cols {
		w = max(w, ui.Width(c.Name+"  "+m.tab.typeOf(c.Name))+10) // box, key, borders, padding
	}
	chip := "grid.cols"
	if m.order {
		chip, w = "grid.order", w+ui.Width(orderMark(m.tab.order, ""))+1
	}
	return v.Box(a.window(), a.chipRect(m.pane, m.tab, chip), min(w, a.layout()[m.pane.ID].Dx())) // the pane's width at most (F15.6)
}

func (a *App) colsMove(d int) {
	m := a.cols
	n := len(a.colsMatches())
	_, rows := a.colsBox(a.colsView())
	m.sel, m.top = ui.ListView(m.sel+d, m.top, n, rows)
}

// colsShift is J (d 1) and K on the selected column: it and the one next to
// it in the list change places, the cursor staying on the column it was on;
// with the filter on it does nothing, the list not being all there (F10.3).
func (a *App) colsShift(d int) {
	m := a.cols
	if m.pick != nil {
		return
	}
	t := m.tab
	o := t.ordered()
	if j := m.sel + d; m.filter.Text != "" || m.sel >= len(o) || j < 0 || j >= len(o) {
		return
	}
	at := t.fieldAt(t.col)
	o[m.sel], o[m.sel+d] = o[m.sel+d], o[m.sel]
	t.colOrder = nil
	for _, f := range o {
		t.colOrder = append(t.colOrder, t.page.Cols[f].Name)
	}
	if at >= 0 {
		t.col = slices.Index(t.shownCols(), at)
	}
	a.colsMove(d)
}

// colsToggle shows or hides the column at row i of the list.
func (a *App) colsToggle(i int) {
	if p := a.cols.pick; p != nil {
		if ms := a.colsMatches(); i < len(ms) {
			p.set(ms[i].Index, !p.on[ms[i].Index])
		}
		return
	}
	if ms := a.colsMatches(); i < len(ms) {
		name := a.cols.tab.page.Cols[ms[i].Index].Name
		a.colsSet(func(n string) bool { return n == name }, a.cols.tab.hidden[name])
	}
}

// colsSetAll shows or hides every column the filter lets through (a / A).
func (a *App) colsSetAll(show bool) { a.colsSet(func(string) bool { return true }, show) }

// colsSet shows or hides the listed columns which, and moves the cursor off
// a column it hides.
func (a *App) colsSet(which func(name string) bool, show bool) {
	if p := a.cols.pick; p != nil {
		for _, m := range a.colsMatches() {
			p.set(m.Index, show)
		}
		return
	}
	t := a.cols.tab
	at := t.fieldAt(t.col)
	for _, m := range a.colsMatches() {
		if name := t.page.Cols[m.Index].Name; which(name) {
			t.hidden[name] = !show
		}
	}
	t.col = t.nearestShown(at)
}

// colsEsc clears the filter, and when there is none, closes the list (Q-04); a pick
// list that closes with a change is the command that writes it and loads again.
func (a *App) colsEsc() tea.Cmd {
	if a.cols.filter.Text != "" {
		a.cols.filter, a.cols.sel, a.cols.top = ui.Input{}, 0, 0
		return nil
	}
	return a.closeCols()
}

// closeCols closes the list.
func (a *App) closeCols() tea.Cmd {
	m := a.cols
	a.cols = nil
	if m != nil && m.pick != nil {
		return a.applyPick(m.pick)
	}
	return nil
}

// colsFilterKey edits the filter; ↵ and esc give the keys back to the list,
// the text kept (§7.8).
func (a *App) colsFilterKey(k keymap.Key) {
	m := a.cols
	switch k {
	case "<CR>", keymap.Esc:
		m.typing = false
	default:
		if editInput(&m.filter, k) {
			m.sel, m.top = 0, 0
		}
	}
}

func (a *App) colsView() ui.Dropdown {
	m := a.cols
	if m.pick != nil {
		return a.pickView()
	}
	t := m.tab
	ms := a.colsMatches()
	v := ui.Dropdown{
		Search: a.icons.Filter, Input: m.filter, Typing: m.typing, Mark: -1, Sel: m.sel, Top: m.top,
		Pane:  m.pane.ID,
		Count: fmt.Sprintf("%d/%d", len(ms), len(t.page.Cols)),
		Hints: bound(
			ui.Hint{Key: a.hints("cols", "/", "cols.move.down", "cols.move.up"), Label: i18n.T("移动")},
			ui.Hint{Key: a.keys.Hint("cols.all", "cols"), Label: i18n.T("全选"), Action: "cols.all"},
			ui.Hint{Key: a.keys.Hint("cols.none", "cols"), Label: i18n.T("全不选"), Action: "cols.none"},
		),
	}
	if m.order {
		v.Hints = bound(
			ui.Hint{Key: a.hints("order", "/", "order.down", "order.up"), Label: i18n.T("移动")},
			ui.Hint{Key: a.keys.Hint("order.toggle", "order"), Label: i18n.T("方向"), Action: "order.toggle"},
			ui.Hint{Key: a.keys.Hint("order.remove", "order"), Label: i18n.T("移出"), Action: "order.remove"},
			ui.Hint{Key: a.keys.Hint("order.clear", "order"), Label: i18n.T("清空"), Action: "order.clear"},
			ui.Hint{Key: a.keys.Hint("order.only", "order"), Label: i18n.T("只按这一列"), Action: "order.only"},
			ui.Hint{Key: a.keys.Hint("order.filter", "order"), Label: i18n.T("搜索"), Action: "order.filter"},
		)
	}
	for _, mt := range ms {
		name := t.page.Cols[mt.Index].Name
		note := t.typeOf(name)
		if slices.Contains(t.cols.PK, name) {
			note += " " + a.icons.Key.Text
		}
		v.Items, v.Pos = append(v.Items, name), append(v.Pos, mt.Pos)
		v.Notes = append(v.Notes, note)
		if m.order {
			v.Icons = append(v.Icons, ui.Icon{Text: orderMark(t.order, name)})
		} else {
			v.Checks = append(v.Checks, !t.hidden[name])
		}
	}
	return v
}

// colsSelected is the name of the column the list's cursor is on.
func (a *App) colsSelected() (string, bool) {
	ms := a.colsMatches()
	if a.cols.sel >= len(ms) {
		return "", false
	}
	return a.cols.tab.page.Cols[ms[a.cols.sel].Index].Name, true
}

// orderToggleSel is space in the ORDER list: the column joins the sorting, or
// turns, and the list stays to go on.
func (a *App) orderToggleSel() tea.Cmd {
	if col, ok := a.colsSelected(); ok {
		return a.orderToggle(a.cols.tab, col)
	}
	return nil
}

// orderRemove is a: the column leaves the sorting.
func (a *App) orderRemove() tea.Cmd {
	col, ok := a.colsSelected()
	t := a.cols.tab
	if !ok || !slices.ContainsFunc(t.order, func(o db.OrderCol) bool { return o.Col == col }) {
		return nil
	}
	return a.setOrder(t, slices.DeleteFunc(slices.Clone(t.order), func(o db.OrderCol) bool { return o.Col == col }))
}

// orderClear is A: back to the default sorting.
func (a *App) orderClear() tea.Cmd {
	if len(a.cols.tab.order) == 0 {
		return nil
	}
	return a.setOrder(a.cols.tab, nil)
}

// orderPick is ↵ on row i: that column alone, and the list closes; on the
// one the chip shows alone it is the other way (§7.8).
func (a *App) orderPick(i int) tea.Cmd {
	t, ms := a.cols.tab, a.colsMatches()
	if a.cols = nil; i >= len(ms) {
		return nil
	}
	col := t.page.Cols[ms[i].Index].Name
	if t.sortsAlone(col) {
		return a.toggleOrder(t)
	}
	return a.setOrder(t, []db.OrderCol{{Col: col, Desc: a.newDesc}})
}

// orderMark is what leads a column of the ORDER list: the place of the column
// in the sorting and its way, "1↓", "2↑", blank for one that does not sort;
// padded to the width of the longest, as the rows line up (F12.1).
func orderMark(order []db.OrderCol, col string) string {
	w := len(strconv.Itoa(len(order))) + 1
	i := slices.IndexFunc(order, func(o db.OrderCol) bool { return o.Col == col })
	mark := ""
	if i >= 0 {
		mark = strconv.Itoa(i+1) + arrow(order[i].Desc)
	}
	return mark + strings.Repeat(" ", max(w-ui.Width(mark), 0))
}

// sortsAlone is whether col is all the chip shows: the only column of the
// ORDER, or with none the row identity's first.
func (t *dataTab) sortsAlone(col string) bool {
	key := t.cols.Key()
	return len(t.order) == 1 && t.order[0].Col == col || len(t.order) == 0 && len(key) > 0 && key[0] == col
}

// orderHead is a click on col's head, and gs: by that column alone, then the
// other way, then back to the default (§7.6「表头」).
func (a *App) orderHead(t *dataTab, col string) tea.Cmd {
	switch {
	case t.sortsAlone(col) && len(t.order) == 1 && t.order[0].Desc != a.newDesc: // the other way already
		return a.setOrder(t, nil)
	case t.sortsAlone(col):
		return a.toggleOrder(t)
	}
	return a.setOrder(t, []db.OrderCol{{Col: col, Desc: a.newDesc}})
}

// orderOnly is the menu's 排序 ↑ and ↓: by col alone, its way given.
func (a *App) orderOnly(t *dataTab, col string, desc bool) tea.Cmd {
	return a.setOrder(t, []db.OrderCol{{Col: col, Desc: desc}})
}

// orderToggle is space on a column of the ORDER list, gS and Shift-click on a
// head: the column joins the sorting at the end, the way `[grid] order` says,
// or if it is in it goes the other way; out of it is a, not this (F12.9).
func (a *App) orderToggle(t *dataTab, col string) tea.Cmd {
	order := slices.Clone(t.order)
	if i := slices.IndexFunc(order, func(o db.OrderCol) bool { return o.Col == col }); i < 0 {
		order = append(order, db.OrderCol{Col: col, Desc: a.newDesc})
	} else {
		order[i].Desc = !order[i].Desc
	}
	return a.setOrder(t, order)
}

// setOrder sorts t by order, from the first page; none is the default, by the
// row identity.
func (a *App) setOrder(t *dataTab, order []db.OrderCol) tea.Cmd {
	for _, o := range order {
		if !t.cols.CanSort(o.Col) {
			return a.showToast(i18n.T("不能按 %s 排序", o.Col))
		}
	}
	t.order, t.desc, t.pageNo = order, len(order) == 0 && a.newDesc, 0
	return a.fetch(t, false)
}

// toggleOrder turns t's ORDER the other way, from the first page (§7.8):
// sorted by default, its row identity's first column goes the other way
// than the default does.
func (a *App) toggleOrder(t *dataTab) tea.Cmd {
	switch key := t.cols.Key(); {
	case len(t.order) > 0: // the first column of several
		order := slices.Clone(t.order)
		order[0].Desc = !order[0].Desc
		return a.setOrder(t, order)
	case key != nil:
		return a.setOrder(t, []db.OrderCol{{Col: key[0], Desc: !t.desc}})
	}
	return nil // nothing to turn: the chip shows no direction
}
