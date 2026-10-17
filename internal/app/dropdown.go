package app

import (
	"slices"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	uv "github.com/charmbracelet/ultraviolet"

	"github.com/kackerx/sqlmux/internal/config"
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
	dropDatabase // a PG query's databases, before its schemas (F15.1)
	dropQueryConn
	dropTx
	dropAuto    // auto refresh's interval (§7.8「自动刷新」)
	dropTheme   // the theme in use (F6.2)
	dropLayout  // layout.open: the layouts there are (F6.5)
	dropExport  // what to export, of a table or a result (F12.4)
	dropHistory // the statements the query's connection ran, one to put at the cursor (F14.2)
)

// dropdown is the open one-pick dropdown; tab or query, in pane, is what
// it acts on.
type dropdown struct {
	kind     dropKind
	tab      *dataTab
	result   *resultTab // dropExport's, when it is of a result
	query    *queryTab
	hist     []config.History // dropHistory's, the newest first
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
	th, ic, unknown, err := config.LoadTheme(name, a.baseIcons)
	if err != nil {
		return a.showToast(err.Error())
	}
	a.theme, a.icons, a.themeName = th, ic, name
	return a.showToast(JoinNotes(config.UnknownThemeKeys(name, unknown), i18n.T("写进 config.toml 的 theme 才会保留")))
}

// openQueryDrop opens the focused query's dropdown of kind k, schema
// (gs, §8.6), connection or transaction mode, or the history; its selection
// on the one it has.
func (a *App) openQueryDrop(k dropKind) {
	p := a.focused()
	if c := queryOf(p); c != nil && (k != dropSchema || len(c.conn.Schemas) > 0 || !c.conn.sql()) {
		if k == dropSchema && (len(c.conn.root().Databases) > 1 || !c.conn.sql()) { // the server's databases first, then the one picked's schemas; a Redis db has none
			k = dropDatabase
		}
		a.showDrop(k, c, p)
	}
}

// showDrop opens query c's dropdown of kind k in pane p.
func (a *App) showDrop(k dropKind, c *queryTab, p *Pane) {
	a.drop = &dropdown{kind: k, query: c, pane: p}
	if k == dropHistory {
		a.drop.hist = historyOf(c.conn)
	}
	_, a.drop.sel = a.dropItems()
	a.dropMove(0)
}

// dropItems is what the dropdown offers, and which of them is current.
func (a *App) dropItems() (items []string, current int) {
	d := a.drop
	switch d.kind {
	case dropSchema:
		return d.query.conn.Schemas, slices.Index(d.query.conn.Schemas, d.query.schema)
	case dropDatabase:
		names := d.query.conn.root().databaseNames()
		current = slices.Index(names, d.query.conn.DB)
		if !d.query.conn.sql() {
			for i, n := range names {
				names[i] = dbText(n)
			}
		}
		return names, current
	case dropTheme:
		names := config.ThemeNames()
		return names, slices.Index(names, a.themeName)
	case dropLayout:
		return layoutNames(), -1
	case dropQueryConn:
		for _, c := range a.sess.Conns {
			items = append(items, c.Spec.Name)
		}
		return items, slices.Index(a.sess.Conns, d.query.conn.root())
	case dropTx:
		current = 0
		if d.query.conn.Manual {
			current = 1
		}
		return []string{i18n.T("自动"), i18n.T("手动")}, current
	case dropExport:
		return exportItems(), 0
	case dropHistory:
		for _, h := range d.hist {
			items = append(items, historyLine(h.SQL))
		}
		return items, -1
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
	case dropSchema, dropDatabase, dropQueryConn, dropTx:
		r := a.layout()[d.pane.ID]
		entry = uv.Rect(r.Max.X-w, r.Min.Y, w, 1)
		action := map[dropKind]string{dropSchema: "query.schema", dropDatabase: "query.schema", dropQueryConn: "query.conn", dropTx: "query.tx"}[d.kind]
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
	case dropHistory: // as wide as the pane lets it be
		r := a.layout()[d.pane.ID]
		w = min(w, max(r.Dx()-4, 16))
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
	case dropDatabase: // the one it is in goes on to its schemas, another is where the query goes
		if !d.query.conn.sql() { // a Redis db is its connection's: the dbs' own names are "db<n>"
			item = strings.TrimPrefix(item, "db")
		}
		if item == d.query.conn.DB {
			if d.query.conn.sql() {
				a.showDrop(dropSchema, d.query, d.pane)
			}
			return nil
		}
		return a.pickConn(d.query, d.query.conn.root().inDB(item))
	case dropQueryConn:
		if a.sess.Conns[at] == d.query.conn.root() { // the one it is on, whichever of its databases
			return nil
		}
		return a.pickConn(d.query, a.sess.Conns[at])
	case dropTx:
		return a.setTxMode(d.query.conn, at == 1)
	case dropTheme:
		return a.setTheme(item)
	case dropLayout:
		return a.pickLayout(item)
	case dropExport:
		return a.export(a.exportSource(d), at)
	case dropHistory: // the statement as it was, lines and all
		return a.queryDid(d.query, d.query.ed.Paste(d.hist[at].SQL))
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

func (a *App) dropKey(k keymap.Key) {
	if editInput(&a.drop.input, k) {
		a.drop.sel, a.drop.top = 0, 0
	}
}

// dropNote is what the dropdown notes on item i: a read-only connection's
// lock (§7.8「query 标题」), a statement's time and outcome (F14.2).
func (a *App) dropNote(i int) string {
	switch {
	case a.drop.kind == dropQueryConn && a.sess.Conns[i].readOnly:
		return a.icons.Lock.Text
	case a.drop.kind == dropHistory:
		return historyNote(a.drop.hist[i])
	}
	return ""
}

// connIcon is the connection dropdown's icon for item i, connection i's
// engine's (§7.8「query 标题」).
func (a *App) connIcon(i int) ui.Icon { return a.icons.Engine(a.sess.Conns[i].Spec.Kind()) }

func (a *App) dropView() ui.Dropdown {
	v := ui.Dropdown{Search: a.icons.Search, Input: a.drop.input, Typing: true, Mark: -1, Sel: a.drop.sel, Top: a.drop.top}
	items, current := a.dropItems()
	for i, m := range a.dropMatches() {
		if m.Index == current {
			v.Mark = i
		}
		v.Items, v.Pos = append(v.Items, items[m.Index]), append(v.Pos, m.Pos)
		if k := a.drop.kind; k == dropQueryConn || k == dropHistory {
			v.Notes = append(v.Notes, a.dropNote(m.Index))
		}
		if a.drop.kind == dropQueryConn {
			v.Icons = append(v.Icons, a.connIcon(m.Index))
		}
	}
	return v
}
