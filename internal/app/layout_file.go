package app

import (
	"cmp"
	"context"
	"fmt"
	"os"
	"slices"
	"strings"

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
		ls := config.LayoutSession{Name: s.Name, Connection: s.last.Spec.Name, Active: s.Active}
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
	if w.Result != nil {
		lw.ResultHeight = 1 - ratio
		if w.Focus == w.Result.ID {
			lw.Focus = 0
		}
		if w.Zoom == w.Result.ID {
			lw.Zoom = 0
		}
	}
	lw.Layout = nodeLayout(root)
	for _, p := range root.Leaves() {
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
			lt = config.LayoutTab{Kind: "console", Connection: tb.Console.conn.Spec.Name, Schema: tb.Console.schema, N: tb.Console.n}
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

// Restore builds the sessions of l over the connections of specs (§14): the
// first session's start connection is dialed now, a failure being the
// error; the others connect once the program runs, and a table tab waits
// for its connection's catalog. What cannot be built is left out, said in
// Warnings: a connection or a console file that is not there.
func Restore(ctx context.Context, specs []config.Connection, l *config.Layout) (*Restored, error) {
	return restore(specs, l, func(c *Conn) error {
		d, err := dial(ctx, c.Spec)
		if err != nil {
			return fmt.Errorf("%s: %w", c.Spec.Name, err)
		}
		c.take(d)
		return nil
	})
}

// restore is Restore, with connect making the first session's start
// connection ready.
func restore(specs []config.Connection, l *config.Layout, connect func(*Conn) error) (*Restored, error) {
	r := &Restored{Active: min(max(l.Active, 0), len(l.Sessions)-1)}
	used := map[int]bool{} // the consoles taken, by number
	for i, ls := range l.Sessions {
		conns := make([]*Conn, len(specs))
		for j, spec := range specs {
			conns[j] = newConn(spec)
		}
		start := slices.IndexFunc(conns, func(c *Conn) bool { return c.Spec.Name == ls.Connection })
		if start < 0 {
			r.Warnings = append(r.Warnings, i18n.T("没有名为 %s 的连接，session %s 用 %s 开始", ls.Connection, ls.Name, conns[0].Spec.Name))
			start = 0
		}
		if i == 0 {
			if err := connect(conns[start]); err != nil {
				return nil, err
			}
		}
		s := &Session{Name: ls.Name, Conns: conns, last: conns[start], open: map[string]bool{}}
		for _, lw := range ls.Windows {
			s.winID++
			w, warns := restoreWindow(s, lw, used)
			w.ID = s.winID
			s.Windows = append(s.Windows, w)
			r.Warnings = append(r.Warnings, warns...)
		}
		if len(s.Windows) == 0 { // a session with no window is one with the default
			w, warning := s.newWindow(conns[start], freeNumber(used))
			used[consoleOf(w.Root.B.Pane).n] = true
			if warning != "" {
				r.Warnings = append(r.Warnings, warning)
			}
			s.Windows = append(s.Windows, w)
		}
		s.Active = min(max(ls.Active, 0), len(s.Windows)-1)
		r.Sessions = append(r.Sessions, s)
	}
	return r, nil
}

// freeNumber is the least console number not in used.
func freeNumber(used map[int]bool) int {
	n := 1
	for used[n] {
		n++
	}
	return n
}

// restoreWindow is lw as a window of s: its panes with their tabs, the
// split tree over them, the focus and the zoom; a pane the tree names that
// has no entry is an empty one, a window with no tree one pane.
func restoreWindow(s *Session, lw config.LayoutWindow, used map[int]bool) (*Window, []string) {
	var warns []string
	panes := map[int]*Pane{}
	w := &Window{Name: lw.Name, TreeOpen: lw.SidebarOpen, TreeW: lw.SidebarWidth, Tree: &Pane{ID: 0}}
	for _, lp := range lw.Panes {
		if lp.ID <= 0 || panes[lp.ID] != nil {
			continue
		}
		p := &Pane{ID: lp.ID, Prev: -1}
		for i, lt := range lp.Tabs {
			tb, warn := s.restoreTab(lt, used)
			if warn != "" {
				warns = append(warns, warn)
				continue
			}
			if i == lp.Current {
				p.Cur = len(p.Tabs)
			}
			p.Tabs = append(p.Tabs, tb)
		}
		panes[p.ID] = p
	}
	seen := map[int]bool{}
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
		if n.Pane <= 0 || seen[n.Pane] {
			return nil
		}
		seen[n.Pane] = true
		if panes[n.Pane] == nil {
			panes[n.Pane] = &Pane{ID: n.Pane, Prev: -1}
		}
		return leaf(panes[n.Pane])
	}
	w.Root = build(lw.Layout)
	if w.Root == nil { // no tree: the first pane there is, else one
		p := &Pane{ID: 1, Prev: -1}
		if len(lw.Panes) > 0 && panes[lw.Panes[0].ID] != nil {
			p = panes[lw.Panes[0].ID]
		}
		w.Root = leaf(p)
	}
	for _, p := range w.Root.Leaves() {
		w.lastID = max(w.lastID, p.ID)
	}
	if lw.ResultHeight > 0 && lw.ResultHeight < 1 {
		w.resultRatio = 1 - lw.ResultHeight
	}
	w.focus(w.Root.Leaves()[0].ID)
	if lw.Focus == 0 && lw.SidebarOpen || w.pane(lw.Focus) != nil {
		w.focus(lw.Focus)
	}
	if w.pane(lw.Zoom) != nil && lw.Zoom != 0 {
		w.Zoom = lw.Zoom
	}
	return w, warns
}

// restoreTab is a layout's tab: a table, pending its connection's catalog,
// or a console on its file; warn says why there is none.
func (s *Session) restoreTab(lt config.LayoutTab, used map[int]bool) (tb Tab, warn string) {
	i := slices.IndexFunc(s.Conns, func(c *Conn) bool { return c.Spec.Name == lt.Connection })
	if i < 0 {
		return tb, i18n.T("没有名为 %s 的连接，跳过 %s", lt.Connection, cmp.Or(lt.Table, fmt.Sprintf("console_%d", lt.N)))
	}
	c := s.Conns[i]
	switch lt.Kind {
	case "table":
		d := newDataTab(c, db.Table{Schema: lt.Schema, Name: lt.Table})
		r := request{applied: lt.Where, order: lt.Order, desc: lt.Desc, limit: cmp.Or(lt.Limit, limits[0]), pageNo: max(lt.Page, 0)}
		d.request, d.shown, d.where.Text, d.pending = r, r, lt.Where, true
		for _, name := range lt.Cols {
			d.hidden[name] = true
		}
		d.transpose = lt.Transpose
		return Tab{Name: lt.Table, Data: d}, ""
	case "console":
		n := lt.N
		if n <= 0 || used[n] {
			n = freeNumber(used)
		}
		t, err := openConsole(s.last, n)
		if err != nil {
			return tb, err.Error()
		}
		used[n] = true
		t.schema = lt.Schema
		if c != s.last { // the connection it was on goes over once it is there
			t.want, t.wantSchema = c, lt.Schema
		}
		return Tab{Name: fmt.Sprintf("console_%d", n), Console: t}, ""
	}
	return tb, i18n.T("不认识的 tab：%s", lt.Kind)
}

// Adopt makes r the program's sessions, the one it names attached; the
// warnings are said once it runs. Before the program runs only.
func (a *App) Adopt(r *Restored) {
	a.sessions, a.sess = r.Sessions, r.Sessions[r.Active]
	a.warning = strings.Trim(a.warning+"；"+strings.Join(r.Warnings, "；"), "；")
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

// dropTab takes tab i out of p as it is, no record kept, the pane staying.
func dropTab(p *Pane, i int) {
	p.Tabs = slices.Delete(p.Tabs, i, i+1)
	if p.Cur > i || p.Cur >= len(p.Tabs) {
		p.Cur = max(p.Cur-1, 0)
	}
	p.Prev = -1
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

// openThemeDrop-like: layout.open lists the layouts, last among them when
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
// unsaved changes and transactions would go (§10.5), then every session is
// replaced.
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
		specs := make([]config.Connection, len(a.sess.Conns))
		for i, c := range a.sess.Conns {
			specs[i] = c.Spec
		}
		return func() tea.Msg {
			l, err := config.LoadLayout(path)
			if err != nil {
				return layoutOpened{err: err}
			}
			r, err := Restore(context.Background(), specs, l)
			return layoutOpened{r: r, err: err}
		}
	})
}

// layoutOpened answers pickLayout.
type layoutOpened struct {
	r   *Restored
	err error
}

// gotLayoutOpened replaces the sessions with the layout's, the old ones'
// connections closed; the new ones connect as at start (§14).
func (a *App) gotLayoutOpened(m layoutOpened) tea.Cmd {
	if m.err != nil {
		return a.showToast(m.err.Error(), toastTTL)
	}
	old := a.sessions
	a.sessions, a.sess = m.r.Sessions, m.r.Sessions[m.r.Active]
	a.attach(a.sess)
	a.restored = true
	cmds := a.connectsNow()
	if len(m.r.Warnings) > 0 {
		cmds = append(cmds, a.showToast(strings.Join(m.r.Warnings, "；"), toastTTL))
	}
	for _, s := range old {
		s.Close()
	}
	return tea.Batch(cmds...)
}
