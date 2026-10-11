package app

import (
	"slices"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	uv "github.com/charmbracelet/ultraviolet"

	"github.com/kackerx/sqlmux/internal/config"
	"github.com/kackerx/sqlmux/internal/db"
	"github.com/kackerx/sqlmux/internal/i18n"
	"github.com/kackerx/sqlmux/internal/keymap"
	"github.com/kackerx/sqlmux/internal/ui"
)

// dropKind is what an open dropdown picks: a table's ORDER or LIMIT (§7.8
// 「查询条」), a query's schema (§8.6), connection or transaction mode
// (§7.8「query 标题」).
type dropKind int

const (
	dropOrder dropKind = iota
	dropLimit
	dropSchema
	dropQueryConn
	dropTx
	dropAuto   // auto refresh's interval (§7.8「自动刷新」)
	dropTheme  // the theme in use (F6.2)
	dropLayout // layout.open: the layouts there are (F6.5)
)

// dropdown is the open one-pick dropdown; tab or query, in pane, is what
// it acts on.
type dropdown struct {
	kind     dropKind
	tab      *dataTab
	query    *queryTab
	pane     *Pane
	input    ui.Input
	sel, top int
}

var limits = []int{100, 500, 1000} // §8.5

// maxLimit caps a page size typed in: a page is all in memory (§7.8).
const maxLimit = 10000

// limitTyped is the page size typed in LIMIT's filter (§7.8): a positive
// whole number, capped at maxLimit; 0 when what is typed is not one.
func limitTyped(s string) int {
	n, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil || n <= 0 {
		return 0
	}
	return min(n, maxLimit)
}

// openDrop opens a dropdown of kind k for the focused table; its
// selection starts on the current value.
func (a *App) openDrop(k dropKind) {
	p := a.focused()
	d := &dropdown{kind: k, tab: dataOf(p), pane: p}
	if d.tab == nil || len(d.tab.page.Cols) == 0 {
		return
	}
	a.drop = d
	_, d.sel = a.dropItems()
	a.dropMove(0)
}

// openThemeDrop is theme.switch: the themes in a dropdown under the focused
// pane's title, the one in use marked (F6.2).
func (a *App) openThemeDrop() {
	p := a.focused()
	a.drop = &dropdown{kind: dropTheme, pane: p}
	_, a.drop.sel = a.dropItems()
	a.dropMove(0)
}

// setTheme puts theme name on the screen at once; it is this run's, nothing
// is written: config.toml is the user's alone (§14).
func (a *App) setTheme(name string) tea.Cmd {
	th, ic, err := config.LoadTheme(name, a.baseIcons)
	if err != nil {
		return a.showToast(err.Error(), toastTTL)
	}
	a.theme, a.icons, a.themeName = th, ic, name
	return a.showToast(i18n.T("写进 config.toml 的 theme 才会保留"), toastTTL)
}

// openQueryDrop opens the focused query's dropdown of kind k, schema
// (gs, §8.6), connection or transaction mode, its selection on the one it
// has.
func (a *App) openQueryDrop(k dropKind) {
	p := a.focused()
	if c := queryOf(p); c != nil && (k != dropSchema || len(c.conn.Schemas) > 0) {
		a.drop = &dropdown{kind: k, query: c, pane: p}
		_, a.drop.sel = a.dropItems()
		a.dropMove(0)
	}
}

// dropItems is what the dropdown offers, and which of them is current.
func (a *App) dropItems() (items []string, current int) {
	d := a.drop
	switch d.kind {
	case dropSchema:
		return d.query.conn.Schemas, slices.Index(d.query.conn.Schemas, d.query.schema)
	case dropTheme:
		names := config.ThemeNames()
		return names, slices.Index(names, a.themeName)
	case dropLayout:
		return layoutNames(), -1
	case dropQueryConn:
		for _, c := range a.sess.Conns {
			items = append(items, c.Spec.Name)
		}
		return items, slices.Index(a.sess.Conns, d.query.conn)
	case dropTx:
		current = 0
		if d.query.conn.Manual {
			current = 1
		}
		return []string{i18n.T("自动"), i18n.T("手动")}, current
	case dropOrder: // first the row identity, "默认"; then the columns
		items = []string{i18n.T("默认")}
		current = 0
		for i, f := range d.tab.ordered() { // as the grid has them (F10.3)
			name := d.tab.page.Cols[f].Name
			items = append(items, name)
			if len(d.tab.order) > 0 && name == d.tab.order[0].Col {
				current = i + 1
			}
		}
		return items, current
	case dropAuto:
		for _, i := range autoIntervals {
			items = append(items, autoLabel(i))
		}
		return items, slices.Index(autoIntervals, d.tab.auto)
	case dropLimit: // a number typed goes first
		if n := limitTyped(d.input.Text); n > 0 {
			items = append(items, strconv.Itoa(n))
		}
		for _, n := range limits {
			items = append(items, strconv.Itoa(n))
		}
		return items, slices.Index(items, strconv.Itoa(d.tab.limit))
	}
	return nil, -1
}

// dropMatches is the items that pass the filter, best first; a page size
// typed in is first whatever the filter makes of it, the same preset
// dropped (§7.8).
func (a *App) dropMatches() []ui.Match {
	items, _ := a.dropItems()
	d := a.drop
	if d.kind != dropLimit || limitTyped(d.input.Text) == 0 {
		return ui.Filter(d.input.Text, items)
	}
	ms := []ui.Match{{Index: 0}}
	for _, m := range ui.Filter(d.input.Text, items[1:]) {
		if m.Index++; items[m.Index] != items[0] {
			ms = append(ms, m)
		}
	}
	return ms
}

// dropBox is where v, the dropdown as drawn, opens (§8.6, §7.8): under its
// chip, left aligned with it, as wide as its longest item with its icon
// and note. A query's opens under its part of the query title, as
// hits, the frame's, have it, its right edge on the pane's right border;
// with the part left out, at that border. Auto refresh's under its button,
// or given way, at the query bar's right end as a schema one.
func (a *App) dropBox(v ui.Dropdown, hits []ui.Hit) (uv.Rectangle, int) {
	d := a.drop
	w := 16
	items, _ := a.dropItems()
	for i, s := range items {
		n := ui.Width(s)
		if note := a.dropNote(i); note != "" {
			n += 1 + ui.Width(note)
		}
		if d.kind == dropQueryConn {
			n += ui.Width(a.connIcon(i).Text) + 1
		}
		if d.kind == dropOrder { // the mark before it
			n += ui.Width(orderMark(d.tab.order, "", 0)) + 1
		}
		w = max(w, n+4) // border and padding on both sides
	}
	var entry uv.Rectangle
	switch d.kind {
	case dropSchema, dropQueryConn, dropTx:
		r := a.layout()[d.pane.ID]
		entry = uv.Rect(r.Max.X-w, r.Min.Y, w, 1)
		action := map[dropKind]string{dropSchema: "query.schema", dropQueryConn: "query.conn", dropTx: "query.tx"}[d.kind]
		for _, h := range hits {
			if h.Target == (ui.Target{Kind: ui.KindHint, Pane: d.pane.ID, Action: action}) {
				entry, w = h.Rect, max(w, r.Max.X-h.Rect.Min.X)
			}
		}
	case dropOrder:
		entry = a.chipRect(d.pane, d.tab, "grid.order")
	case dropLimit:
		entry = a.chipRect(d.pane, d.tab, "grid.limit")
	case dropTheme, dropLayout:
		r := a.layout()[d.pane.ID]
		entry = uv.Rect(r.Min.X+2, r.Min.Y, w, 1)
	case dropAuto:
		r := a.layout()[d.pane.ID]
		if entry = a.queryBar(d.pane, d.tab).ButtonRect(bodyRect(r), "grid.refresh.auto"); entry.Empty() {
			entry = uv.Rect(r.Max.X-w, bodyRect(r).Min.Y+1, w, 1)
		}
	}
	return v.Box(a.window(), entry, w)
}

func (a *App) dropMove(d int) {
	m := a.drop
	n := len(a.dropMatches())
	_, rows := a.dropBox(a.dropView(), a.hits)
	m.sel = max(min(m.sel+d, n-1), 0)
	m.top = max(min(m.top, m.sel), m.sel-rows+1)
}

// dropPick takes the item at i, closes the dropdown and refetches from the
// first page.
func (a *App) dropPick(i int) tea.Cmd {
	d, ms := a.drop, a.dropMatches()
	items, _ := a.dropItems()
	if a.drop = nil; i >= len(ms) {
		return nil
	}
	at := ms[i].Index
	item := items[at]
	switch t := d.tab; d.kind {
	case dropSchema: // the next run sets it (§8.6)
		d.query.schema = item
		return a.keepQuery(d.query)
	case dropQueryConn:
		return a.pickConn(d.query, a.sess.Conns[at])
	case dropTx:
		return a.setTxMode(d.query.conn, at == 1)
	case dropTheme:
		return a.setTheme(item)
	case dropLayout:
		return a.pickLayout(item)
	case dropAuto: // the ticking so far stops; this interval's starts
		if t.auto, t.autoGen = autoIntervals[at], t.autoGen+1; t.auto == 0 {
			return nil
		}
		return autoTick(t)
	case dropOrder, dropLimit: // below
	}
	switch t := d.tab; d.kind {
	case dropOrder:
		switch {
		case at == 0: // 默认: by the row identity
			t.order, t.desc = nil, a.newDesc
		case len(t.order) == 1 && t.order[0].Col == item, len(t.order) == 0 && len(t.cols.Key()) > 0 && t.cols.Key()[0] == item: // the column the chip shows first, alone: the other way (§7.8)
			return a.toggleOrder(t)
		default: // only this column
			t.order, t.desc = []db.OrderCol{{Col: item, Desc: a.newDesc}}, false
		}
	case dropLimit:
		t.limit, _ = strconv.Atoi(item)
	case dropSchema, dropQueryConn, dropTx, dropAuto, dropTheme, dropLayout: // above
	}
	d.tab.pageNo = 0
	return a.fetch(d.tab, false)
}

// orderMark is what leads an item of the ORDER list: the place of the column
// in the sorting and its way, "1↓", "2↑", blank for one that does not sort;
// padded to the width of the longest, as the rows line up (F12.1).
func orderMark(order []db.OrderCol, item string, at int) string {
	w := len(strconv.Itoa(len(order))) + 1
	i := slices.IndexFunc(order, func(o db.OrderCol) bool { return o.Col == item })
	mark := ""
	if at != 0 && i >= 0 { // 默认 is no column
		mark = strconv.Itoa(i+1) + arrow(order[i].Desc)
	}
	return mark + strings.Repeat(" ", max(w-ui.Width(mark), 0))
}

// orderCycle is space on a column of the ORDER list: in the sorting at the
// end, the way `[grid] order` says; the other way; out of it. The dropdown
// stays open to go on (F12.1).
func (a *App) orderCycle(t *dataTab, col string) tea.Cmd {
	order := slices.Clone(t.order)
	switch i := slices.IndexFunc(order, func(o db.OrderCol) bool { return o.Col == col }); {
	case i < 0:
		order = append(order, db.OrderCol{Col: col, Desc: a.newDesc})
	case order[i].Desc == a.newDesc:
		order[i].Desc = !a.newDesc
	default:
		order = slices.Delete(order, i, i+1)
	}
	t.order, t.desc = order, len(order) == 0 && a.newDesc
	t.pageNo = 0
	return a.fetch(t, false)
}

// dropToggle is space in a dropdown: the column under the cursor goes through
// ORDER's cycle while the filter is empty; else it is a character of the
// filter, as in any other.
func (a *App) dropToggle() tea.Cmd {
	d := a.drop
	if d.kind != dropOrder || d.input.Text != "" {
		a.dropKey("<Space>")
		return nil
	}
	if ms := a.dropMatches(); d.sel < len(ms) && ms[d.sel].Index > 0 { // 默认 is no column
		items, _ := a.dropItems()
		return a.orderCycle(d.tab, items[ms[d.sel].Index])
	}
	return nil
}

// toggleOrder turns t's ORDER the other way, from the first page (§7.8):
// sorted by default, its row identity's first column goes the other way
// than the default does.
func (a *App) toggleOrder(t *dataTab) tea.Cmd {
	switch key := t.cols.Key(); {
	case len(t.order) > 0: // the first column of several
		t.order = slices.Clone(t.order)
		t.order[0].Desc = !t.order[0].Desc
	case key != nil:
		t.order, t.desc = []db.OrderCol{{Col: key[0], Desc: !t.desc}}, false
	default: // nothing to turn: the chip shows no direction
		return nil
	}
	t.pageNo = 0
	return a.fetch(t, false)
}

func (a *App) dropKey(k keymap.Key) {
	if editInput(&a.drop.input, k) {
		a.drop.sel, a.drop.top = 0, 0
	}
}

// dropNote is what the dropdown notes on item i: a read-only connection's
// lock (§7.8「query 标题」).
func (a *App) dropNote(i int) string {
	if a.drop.kind == dropQueryConn && a.sess.Conns[i].readOnly {
		return a.icons.Lock.Text
	}
	return ""
}

// connIcon is the connection dropdown's icon for item i, connection i's
// engine's (§7.8「query 标题」).
func (a *App) connIcon(i int) ui.Icon { return a.icons.Engine(a.sess.Conns[i].Spec.Engine) }

func (a *App) dropView() ui.Dropdown {
	v := ui.Dropdown{Search: a.icons.Search, Input: a.drop.input, Typing: true, Mark: -1, Sel: a.drop.sel, Top: a.drop.top}
	items, current := a.dropItems()
	for i, m := range a.dropMatches() {
		if m.Index == current {
			v.Mark = i
		}
		v.Items, v.Pos = append(v.Items, items[m.Index]), append(v.Pos, m.Pos)
		if a.drop.kind == dropOrder {
			v.Icons = append(v.Icons, ui.Icon{Text: orderMark(a.drop.tab.order, items[m.Index], m.Index)})
		}
		if a.drop.kind == dropQueryConn {
			v.Notes, v.Icons = append(v.Notes, a.dropNote(m.Index)), append(v.Icons, a.connIcon(m.Index))
		}
	}
	return v
}
