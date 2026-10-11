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

// dropKind is what an open dropdown picks: a table's LIMIT (§7.8
// 「查询条」), a query's schema (§8.6), connection or transaction mode
// (§7.8「query 标题」).
type dropKind int

const (
	dropLimit dropKind = iota
	dropSchema
	dropQueryConn
	dropTx
	dropAuto   // auto refresh's interval (§7.8「自动刷新」)
	dropTheme  // the theme in use (F6.2)
	dropLayout // layout.open: the layouts there are (F6.5)
	dropExport // what to export, of a table or a result (F12.4)
)

// dropdown is the open one-pick dropdown; tab or query, in pane, is what
// it acts on.
type dropdown struct {
	kind     dropKind
	tab      *dataTab
	result   *resultTab // dropExport's, when it is of a result
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
	case dropExport:
		return exportItems(), 0
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
	case dropExport:
		r := a.layout()[d.pane.ID]
		if d.tab != nil {
			entry = a.queryBar(d.pane, d.tab).ButtonRect(bodyRect(r), "grid.export")
		}
		for _, h := range hits { // a result's button on its pane's title
			if d.result != nil && h.Target.Pane == d.pane.ID && h.Target.Action == "result.export" {
				entry = h.Rect
			}
		}
		if entry.Empty() {
			entry = uv.Rect(r.Max.X-w, bodyRect(r).Min.Y+1, w, 1)
			if d.result != nil {
				entry = uv.Rect(r.Max.X-w, r.Min.Y, w, 1)
			}
		}
	}
	return v.Box(a.window(), entry, w)
}

func (a *App) dropMove(d int) {
	m := a.drop
	n := len(a.dropMatches())
	_, rows := a.dropBox(a.dropView(), a.hits)
	m.sel, m.top = ui.ListView(m.sel+d, m.top, n, rows)
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
	switch d.kind {
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
	case dropExport:
		return a.export(a.exportSource(d), at)
	case dropAuto: // the ticking so far stops; this interval's starts
		t := d.tab
		if t.auto, t.autoGen = autoIntervals[at], t.autoGen+1; t.auto == 0 {
			return nil
		}
		return autoTick(t)
	case dropLimit:
		d.tab.limit, _ = strconv.Atoi(item)
	}
	d.tab.pageNo = 0
	return a.fetch(d.tab, false)
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
		t.order, t.desc = nil, a.newDesc
	case t.sortsAlone(col):
		return a.toggleOrder(t)
	default:
		t.order, t.desc = []db.OrderCol{{Col: col, Desc: a.newDesc}}, false
	}
	t.pageNo = 0
	return a.fetch(t, false)
}

// orderOnly is the menu's 排序 ↑ and ↓: by col alone, its way given.
func (a *App) orderOnly(t *dataTab, col string, desc bool) tea.Cmd {
	t.order, t.desc, t.pageNo = []db.OrderCol{{Col: col, Desc: desc}}, false, 0
	return a.fetch(t, false)
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
	t.order, t.desc, t.pageNo = order, len(order) == 0 && a.newDesc, 0
	return a.fetch(t, false)
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
		if a.drop.kind == dropQueryConn {
			v.Notes, v.Icons = append(v.Notes, a.dropNote(m.Index)), append(v.Icons, a.connIcon(m.Index))
		}
	}
	return v
}
