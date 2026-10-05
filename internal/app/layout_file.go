package app

import (
	"cmp"
	"context"
	"fmt"
	"os"
	"slices"

	tea "charm.land/bubbletea/v2"

	"sqlmux/internal/config"
	"sqlmux/internal/db"
	"sqlmux/internal/i18n"
	"sqlmux/internal/keymap"
	"sqlmux/internal/ui"
)

// The working set as a file (§14「布局文件」, F6.5): layoutOf takes it from
// the sessions, Restore builds sessions from it.

// layoutOf is every session as a Layout: the windows with their split
// trees and panes, a table's request, a console's file, connection and
// schema. The result area, a landing tab, results and what is not saved are
// left out.
func (a *App) layoutOf() *config.Layout {
	l := &config.Layout{Version: config.LayoutVersion, Active: slices.Index(a.sessions, a.sess)}
	for _, s := range a.sessions {
		ls := config.LayoutSession{Name: s.Name, Connection: s.start.Spec.Name, Active: s.Active}
		for _, w := range s.Windows {
			ls.Windows = append(ls.Windows, windowLayout(w))
		}
		l.Sessions = append(l.Sessions, ls)
	}
	return l
}

func windowLayout(w *Window) config.LayoutWindow {
	lw := config.LayoutWindow{Name: w.Name, Focus: w.Focus, Zoom: w.Zoom, SidebarOpen: w.TreeOpen, SidebarWidth: w.TreeW}
	root, ratio := w.Root, w.resultRatio
	if w.Result != nil && !w.resultHidden { // the result area is the root's lower half
		root, ratio = w.Root.A, w.Root.Ratio
	}
	if ratio != 0 { // from the area as it is, or as it was when it was closed
		lw.ResultHeight = 1 - ratio
	}
	leaves := root.Leaves()
	if w.Result != nil && w.Focus == w.Result.ID { // it is not saved: the pane focused before it is
		lw.Focus = slices.MaxFunc(leaves, func(a, b *Pane) int { return cmp.Compare(w.focusedAt[a.ID], w.focusedAt[b.ID]) }).ID
	}
	if w.Result != nil && w.Zoom == w.Result.ID {
		lw.Zoom = 0
	}
	lw.Layout = nodeLayout(root)
	for _, p := range leaves {
		lw.Panes = append(lw.Panes, paneLayout(p))
	}
	return lw
}

func nodeLayout(n *Node) *config.LayoutNode {
	switch n.Split {
	case Horiz:
		return &config.LayoutNode{Split: "h", Ratio: n.Ratio, A: nodeLayout(n.A), B: nodeLayout(n.B)}
	case Vert:
		return &config.LayoutNode{Split: "v", Ratio: n.Ratio, A: nodeLayout(n.A), B: nodeLayout(n.B)}
	default: // a leaf
	}
	return &config.LayoutNode{Pane: n.Pane.ID}
}

func paneLayout(p *Pane) config.LayoutPane {
	lp := config.LayoutPane{ID: p.ID}
	for i, tb := range p.Tabs {
		var lt config.LayoutTab
		switch {
		case tb.Data != nil:
			d := tb.Data
			lt = config.LayoutTab{Kind: "table", Connection: d.conn.Spec.Name, Schema: d.table.Schema, Table: d.table.Name,
				Where: d.applied, Order: d.order, Desc: d.desc, Limit: d.limit, Page: d.pageNo, Transpose: d.transpose}
			for name, hidden := range d.hidden {
				if hidden {
					lt.Cols = append(lt.Cols, name)
				}
			}
			slices.Sort(lt.Cols)
		case tb.Console != nil:
			t := tb.Console
			c, schema := t.conn, t.schema
			if t.want != nil { // not there yet: it is where it is going that is kept
				c, schema = t.want, t.wantSchema
			}
			lt = config.LayoutTab{Kind: "console", Connection: c.Spec.Name, Schema: schema, N: t.n}
		default: // a landing tab
			continue
		}
		if i == p.Cur {
			lp.Current = len(lp.Tabs)
		}
		lp.Tabs = append(lp.Tabs, lt)
	}
	return lp
}

// Restored is the sessions a layout made, and what was said making them.
type Restored struct {
	Sessions []*Session
	Active   int
	Warnings []string
}

// DialStart connects the first session's start connection now: at the
// program's start, a failure is the start's error (§14「恢复」).
func (r *Restored) DialStart(ctx context.Context) error {
	c := r.Sessions[0].start
	d, err := dial(ctx, c.Spec)
	if err != nil {
		return fmt.Errorf("%s: %w", c.Spec.Name, err)
	}
	c.take(d)
	return nil
}

// Restore builds the sessions of l over the connections of specs (§14),
// none of them connected: a table tab waits for its connection's catalog.
// What cannot be built is left out, said in Warnings: a connection that is
// not there, a console file that does not read.
func Restore(specs []config.Connection, l *config.Layout) *Restored {
	r := &Restored{Active: min(max(l.Active, 0), len(l.Sessions)-1)}
	used := map[int]bool{} // the consoles taken, by number
	for _, ls := range l.Sessions {
		conns := make([]*Conn, len(specs))
		for j, spec := range specs {
			conns[j] = newConn(spec)
		}
		start := slices.IndexFunc(conns, func(c *Conn) bool { return c.Spec.Name == ls.Connection })
		if start < 0 {
			r.Warnings = append(r.Warnings, i18n.T("没有名为 %s 的连接，session %s 用 %s 开始", ls.Connection, ls.Name, conns[0].Spec.Name))
			start = 0
		}
		s := &Session{Name: ls.Name, Conns: conns, start: conns[start], last: conns[start], open: map[string]bool{}}
		for _, lw := range ls.Windows {
			s.winID++
			w, warns := restoreWindow(s, lw, used)
			w.ID = s.winID
			s.Windows = append(s.Windows, w)
			r.Warnings = append(r.Warnings, warns...)
		}
		if len(s.Windows) == 0 { // a session with no window is one with the default
			n := freeNumber(func(n int) bool { return used[n] })
			w, warning := s.newWindow(conns[start], n)
			if warning != "" {
				r.Warnings = append(r.Warnings, warning)
			} else {
				used[n] = true
			}
			s.Windows = append(s.Windows, w)
		}
		s.Active = min(max(ls.Active, 0), len(s.Windows)-1)
		r.Sessions = append(r.Sessions, s)
	}
	return r
}

// restoreWindow is lw as a window of s: the split tree with the panes it
// names, their tabs, the focus and the zoom. A pane the tree has that no
// entry gives is an empty one; one the tree does not have gets no tab; a
// window with no tree is one pane, the first there is.
func restoreWindow(s *Session, lw config.LayoutWindow, used map[int]bool) (*Window, []string) {
	var warns []string
	panes := map[int]*Pane{} // the tree's
	w := &Window{Name: lw.Name, TreeOpen: lw.SidebarOpen, TreeW: lw.SidebarWidth, Tree: &Pane{ID: 0}}
	var build func(n *config.LayoutNode) *Node
	build = func(n *config.LayoutNode) *Node {
		if n == nil {
			return nil
		}
		if n.Split == "h" || n.Split == "v" {
			a, b := build(n.A), build(n.B)
			switch {
			case a == nil:
				return b
			case b == nil:
				return a
			}
			ratio := n.Ratio
			if ratio <= 0 || ratio >= 1 {
				ratio = 0.5
			}
			return &Node{Split: map[string]Dir{"h": Horiz, "v": Vert}[n.Split], Ratio: ratio, A: a, B: b}
		}
		if n.Pane <= 0 || panes[n.Pane] != nil {
			return nil
		}
		panes[n.Pane] = &Pane{ID: n.Pane, Prev: -1}
		return leaf(panes[n.Pane])
	}
	if w.Root = build(lw.Layout); w.Root == nil {
		id := 1
		if i := slices.IndexFunc(lw.Panes, func(lp config.LayoutPane) bool { return lp.ID > 0 }); i >= 0 {
			id = lw.Panes[i].ID
		}
		panes[id] = &Pane{ID: id, Prev: -1}
		w.Root = leaf(panes[id])
	}
	filled := map[int]bool{}
	for _, lp := range lw.Panes {
		p := panes[lp.ID]
		if p == nil || filled[lp.ID] { // not in the tree, or a second entry: not read
			continue
		}
		filled[lp.ID] = true
		for i, lt := range lp.Tabs {
			tb, warn := s.restoreTab(lt, used)
			if warn != "" {
				warns = append(warns, warn)
				continue
			}
			if d := tb.Data; d != nil && slices.ContainsFunc(p.Tabs, func(o Tab) bool {
				return o.Data != nil && o.Data.conn == d.conn && idOf(o.Data.table) == idOf(d.table)
			}) { // a table is one tab of a pane (F3.37): the first stays
				continue
			}
			if i == lp.Current {
				p.Cur = len(p.Tabs)
			}
			p.Tabs = append(p.Tabs, tb)
		}
	}
	for _, p := range w.Root.Leaves() {
		w.lastID = max(w.lastID, p.ID)
	}
	if lw.ResultHeight > 0 && lw.ResultHeight < 1 {
		w.resultRatio = 1 - lw.ResultHeight
	}
	w.focus(w.Root.Leaves()[0].ID)
	if (lw.Focus != 0 || lw.SidebarOpen) && w.pane(lw.Focus) != nil { // 0 is the sidebar, when there is one
		w.focus(lw.Focus)
	}
	if lw.Zoom != 0 && lw.Zoom == w.Focus { // a zoom of another pane than the focused one hides it
		w.Zoom = lw.Zoom
	}
	return w, warns
}

// restoreTab is a layout's tab: a table, pending its connection's catalog,
// or a console on its file; warn says why there is none. A tab with no
// connection is on the session's start (§14).
func (s *Session) restoreTab(lt config.LayoutTab, used map[int]bool) (tb Tab, warn string) {
	if lt.Kind != "table" && lt.Kind != "console" {
		return tb, i18n.T("不认识的 tab")
	}
	name := cmp.Or(lt.Connection, s.start.Spec.Name)
	i := slices.IndexFunc(s.Conns, func(c *Conn) bool { return c.Spec.Name == name })
	if i < 0 {
		return tb, i18n.T("没有名为 %s 的连接，跳过 %s", name, cmp.Or(lt.Table, fmt.Sprintf("console_%d", lt.N)))
	}
	c := s.Conns[i]
	if lt.Kind == "table" { // a schema it does not name is the connection's home, known with the catalog
		d := newDataTab(c, db.Table{Schema: lt.Schema, Name: lt.Table})
		r := d.request
		r.applied, r.order, r.desc, r.pageNo = lt.Where, lt.Order, lt.Desc, max(lt.Page, 0)
		if lt.Limit > 0 { // typed by hand it may be anything
			r.limit = min(lt.Limit, maxLimit)
		}
		d.setRequest(r)
		d.pending = true
		for _, name := range lt.Cols {
			d.hidden[name] = true
		}
		d.transpose = lt.Transpose
		return Tab{Name: lt.Table, Data: d}, ""
	}
	n := lt.N
	if n <= 0 || used[n] { // taken: the next free one, with its own file
		n = freeNumber(func(n int) bool { return used[n] })
	}
	t, err := openConsole(s.start, n)
	if err != nil {
		return tb, err.Error()
	}
	used[n] = true
	if c == s.start {
		t.schema = lt.Schema
	} else { // the connection it was on goes over once it is there
		t.want, t.wantSchema = c, lt.Schema
	}
	return Tab{Name: fmt.Sprintf("console_%d", n), Console: t}, ""
}

// Adopt makes r the program's sessions, the one it names attached; the
// warnings are said once it runs. Before the program runs only.
func (a *App) Adopt(r *Restored) {
	a.sessions, a.sess = r.Sessions, r.Sessions[r.Active]
	a.warning = JoinNotes(append([]string{a.warning}, r.Warnings...)...)
	a.plainConsoles()
	a.restored = true
}

// connectsNow are the connections the running program connects to at once:
// each session's start, a pending table tab's, the one a console waits for.
func (a *App) connectsNow() []tea.Cmd {
	var cmds []tea.Cmd
	catalog := map[*Conn]bool{}
	want := func(c *Conn) {
		if c.State == connReady && !catalog[c] {
			catalog[c] = true
			cmds = append(cmds, a.loadCatalog(c))
		} else if c.State != connReady {
			cmds = append(cmds, a.connect(c))
		}
	}
	for _, s := range a.sessions {
		want(s.last)
		for _, p := range s.panes() {
			for _, tb := range p.Tabs {
				switch {
				case tb.Data != nil && tb.Data.pending:
					want(tb.Data.conn)
				case tb.Console != nil && tb.Console.want != nil:
					want(tb.Console.want)
				}
			}
		}
	}
	return cmds
}

// resolvePending runs once c's catalog is in: a table tab waiting for it
// gets its table and fetches; one whose table is not in it is closed, said
// (§14「恢复」).
func (a *App) resolvePending(c *Conn) []tea.Cmd {
	var cmds []tea.Cmd
	for _, s := range a.sessions {
		for _, p := range s.panes() {
			for i := len(p.Tabs) - 1; i >= 0; i-- {
				d := p.Tabs[i].Data
				if d == nil || !d.pending || d.conn != c {
					continue
				}
				d.pending = false
				home := func(t db.Table) string { return cmp.Or(t.Schema, c.home) } // a hand's layout may leave it out
				if slices.ContainsFunc(p.Tabs[:i], func(o Tab) bool {
					return o.Data != nil && o.Data.conn == c && o.Data.table.Name == d.table.Name && home(o.Data.table) == home(d.table)
				}) { // the same table as one before it: a pane has it once (F3.37)
					dropTab(p, i)
					continue
				}
				d.table.Schema = home(d.table)
				if j := slices.IndexFunc(c.Tables, func(t db.Table) bool { return idOf(t) == idOf(d.table) }); j >= 0 {
					d.table = c.Tables[j]
					cmds = append(cmds, a.fetch(d, true))
					continue
				}
				cmds = append(cmds, a.showToast(i18n.T("%s 里没有表 %s，跳过", c.Spec.Name, d.table.Schema+"."+d.table.Name), toastTTL))
				dropTab(p, i)
			}
		}
	}
	return cmds
}

// dropPending closes the table tabs waiting on connection c, which is not
// to be had, without a word: the connection's own toast says it (§14).
func (a *App) dropPending(c *Conn) {
	for _, s := range a.sessions {
		for _, p := range s.panes() {
			for i := len(p.Tabs) - 1; i >= 0; i-- {
				if d := p.Tabs[i].Data; d != nil && d.pending && d.conn == c {
					dropTab(p, i)
				}
			}
		}
	}
}

// verbOpen is what opening a layout asks about: it replaces every session
// (§14).
var verbOpen = lostVerb{i18n.N("打开"), i18n.N("打开布局会丢弃。"), i18n.N("打开布局会回滚。")}

// SaveLayoutOnExit has the program write its working set to path as it ends
// by its own quit paths (§14「保存」).
func (a *App) SaveLayoutOnExit(path string) { a.lastPath = path }

// saveLast writes the working set where SaveLayoutOnExit said, as the
// program quits. An error has no one to tell then.
// ponytail: a program killed does not write; the layout is the one of the
// last quit.
func (a *App) saveLast() {
	if a.lastPath != "" {
		_ = config.SaveLayout(a.lastPath, a.layoutOf())
	}
}

// askLayoutName is layout.save: the name typed in the status bar, ↵ writes
// layouts/<name>.toml, esc drops it (§14「命令」).
func (a *App) askLayoutName() {
	a.layoutName = &ui.Input{}
}

// layoutNameKey is key k in that input.
func (a *App) layoutNameKey(k keymap.Key) tea.Cmd {
	name, done := typeName(a.layoutName, k)
	if !done {
		return nil
	}
	a.layoutName = nil
	if name == "" {
		return nil
	}
	if err := config.CheckLayoutName(name); err != nil {
		return a.showToast(err.Error(), toastTTL)
	}
	if err := config.SaveLayout(config.LayoutPath(name), a.layoutOf()); err != nil {
		return a.showToast(err.Error(), toastTTL)
	}
	return a.showToast(i18n.T("已保存布局 %s", name), toastTTL)
}

// openLayoutDrop is layout.open: the layouts listed, last among them when
// there is one.
func (a *App) openLayoutDrop() {
	a.drop = &dropdown{kind: dropLayout, pane: a.focused(), sel: 0}
	a.dropMove(0)
}

// layoutNames are what layout.open offers: the layouts of layouts/, and
// last, where the program left off, when it is kept.
func layoutNames() []string {
	names := config.LayoutNames()
	if _, err := os.Stat(config.LastPath()); err == nil {
		names = append([]string{"last"}, slices.DeleteFunc(names, func(n string) bool { return n == "last" })...)
	}
	return names
}

// pickLayout is a layout.open pick: asked first when every session's
// unsaved changes and transactions would go (§10.5); then the consoles are
// written, the layout read and every session replaced, all in this one
// Update (§14「命令」): between, what was typed would be lost or written
// over the new consoles' files. The connections are dialed in the background,
// the first session's start among them.
func (a *App) pickLayout(name string) tea.Cmd {
	path := config.LayoutPath(name)
	if name == "last" {
		path = config.LastPath()
	}
	var ps []*Pane
	for _, s := range a.sessions {
		ps = append(ps, s.panes()...)
	}
	return a.unlessLost(unsaved(ps...), "", txConns(a.sessions...), verbOpen, func() tea.Cmd {
		if cmd, ok := a.flushAll(ps...); !ok { // the consoles' text is not lost (§11)
			return cmd
		}
		l, err := config.LoadLayout(path)
		if err != nil {
			return a.showToast(err.Error(), toastTTL)
		}
		specs := make([]config.Connection, len(a.sess.Conns))
		for i, c := range a.sess.Conns {
			specs[i] = c.Spec
		}
		r := Restore(specs, l)
		var ws []*db.Worker
		for _, s := range a.sessions {
			ws = append(ws, s.workers()...)
		}
		a.sessions = r.Sessions
		a.attach(r.Sessions[r.Active])
		cmds := append(a.connectsNow(), closeWorkers(ws))
		if len(r.Warnings) > 0 {
			cmds = append(cmds, a.showToast(JoinNotes(r.Warnings...), toastTTL))
		}
		return tea.Batch(cmds...)
	})
}
