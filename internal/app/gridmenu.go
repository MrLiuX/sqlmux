package app

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	uv "github.com/charmbracelet/ultraviolet"

	"github.com/kackerx/sqlmux/internal/i18n"
	"github.com/kackerx/sqlmux/internal/ui"
)

// gridMenu is the right-click menu of a cell or a head of the grid pane the
// pointer is on (§7.6「右键菜单」, F12.3), the cursor put on the cell first; the
// head's is the head's column. It opens nothing where there are no items: a
// result's head, or a table's in the turned view, which has none.
func (a *App) gridMenu(t ui.Target, p uv.Position) {
	pane := a.win().pane(t.Pane)
	if pane == nil {
		return
	}
	a.focusPane(t.Pane)
	var items []menuItem
	switch d, r := dataOf(pane), resultOf(pane); {
	case d != nil && t.Kind == ui.KindHead:
		d.col = t.I
		items = a.columnItems(d, false)
	case d != nil:
		a.gridGoto(strings.TrimPrefix(t.Action, "grid.goto "))
		items = a.columnItems(d, true)
	case r != nil && t.Kind == ui.KindCell:
		a.gridGoto(strings.TrimPrefix(t.Action, "grid.goto "))
		items = []menuItem{
			{i18n.T("在值编辑器中打开"), func() tea.Cmd { return a.valueOpen() }},
			{i18n.T("复制这一格"), func() tea.Cmd { return a.yankGrid(false) }},
		}
	}
	if len(items) > 0 {
		a.menu = &menu{at: p, items: items}
	}
}

// columnItems are the menu of the cell under the cursor of table t, or of
// its column's head: what sorts, hides and, of a cell, opens, filters and
// copies, each what the action of its name does.
func (a *App) columnItems(t *dataTab, cell bool) []menuItem {
	col := t.page.Cols[t.fieldAt(t.col)].Name
	sort := []menuItem{
		{i18n.T("按这一列排序 ↑"), func() tea.Cmd { return a.orderOnly(t, col, false) }},
		{i18n.T("按这一列排序 ↓"), func() tea.Cmd { return a.orderOnly(t, col, true) }},
		{i18n.T("加入排序"), func() tea.Cmd { return a.orderCycle(t, col) }},
		{i18n.T("隐藏这一列"), func() tea.Cmd { a.hideColumn(t, t.fieldAt(t.col)); return nil }},
	}
	if !cell {
		return sort
	}
	items := []menuItem{
		{i18n.T("在值编辑器中打开"), func() tea.Cmd { return a.valueOpen() }},
		{i18n.T("按这个值过滤"), func() tea.Cmd { return a.filterCell() }},
	}
	items = append(items, sort...)
	return append(items,
		menuItem{i18n.T("复制这一格"), func() tea.Cmd { return a.yankGrid(false) }},
		menuItem{i18n.T("复制行为 JSON"), func() tea.Cmd { return a.yankJSON() }},
	)
}

// hideColumn hides the page's column field, as COLS does, the cursor moved
// off it to the nearest shown one.
func (a *App) hideColumn(t *dataTab, field int) {
	t.hidden[t.page.Cols[field].Name] = true
	t.col = t.nearestShown(field)
}

// headClick is a click on the head of the shown column i of table pane: that
// column is the cursor's, and it sorts, or with Shift is added to the sorting.
func (a *App) headClick(pane, i int, shift bool) tea.Cmd {
	a.focusPane(pane)
	t := dataOf(a.focused())
	if t == nil || t.fieldAt(i) < 0 {
		return nil
	}
	t.col = i
	a.gridMove(func(r, c, _, _ int) (int, int) { return r, c })
	return a.run(map[bool]string{false: "grid.order.cycle", true: "grid.order.add"}[shift], 0)
}
