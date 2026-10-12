package app

import (
	"cmp"
	"context"
	"fmt"
	"maps"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/kackerx/sqlmux/internal/db"
	"github.com/kackerx/sqlmux/internal/i18n"
	"github.com/kackerx/sqlmux/internal/keymap"
	"github.com/kackerx/sqlmux/internal/ui"
)

// treeState is one window's schema tree (§7.8); what it shows is the
// session's: its connections, then its workspace.
type treeState struct {
	cursor, top int    // into the nodes showing
	at          string // the cursor's node: it stays on it as others come and go
	filter      ui.Input
	filtering   bool // the filter row has the keys (INSERT)
}

// connected answers connect: c's Main and Meta, or why not.
type connected struct {
	conn *Conn
	d    dialed
	err  error
}

// connect connects c, not connected yet or failed before; then its
// catalog loads (§7.8「连接节点」).
func (a *App) connect(c *Conn) tea.Cmd {
	if c.State == connConnecting || c.State == connReady {
		return nil
	}
	c.State = connConnecting
	spec := c.Spec
	c.eng, c.readOnly = engines[spec.Engine], spec.ReadOnly // as this dial has them
	return func() tea.Msg {
		d, err := dial(context.Background(), spec)
		return connected{c, d, err}
	}
}

// notReady is what a command that needs c connected does while it is not:
// dials again after a failure; while the dial is on, says so (F12.5).
func (a *App) notReady(c *Conn) tea.Cmd {
	if cmd := a.connect(c); cmd != nil {
		return cmd
	}
	return a.showToast(i18n.T("连接还没连上"))
}

// gotConnected takes c's connections in, or marks it failed; the queries
// waiting on it go over, or stay (§7.8「query 标题」). One of a session
// closed meanwhile closes them again: nothing would.
func (a *App) gotConnected(m connected) tea.Cmd {
	c := m.conn
	if a.sessionOf(c) == nil {
		if m.err != nil {
			return nil
		}
		return m.d.discard()
	}
	var waiting []*queryTab
	for _, t := range a.queries() {
		if t.want == c {
			t.want, waiting = nil, append(waiting, t)
		}
	}
	if m.err != nil {
		c.State, c.err = connFailed, m.err // the tabs waiting on it stay, saying why (F12.5)
		return a.showToast(i18n.T("%s：%s", c.Spec.Name, m.err))
	}
	c.take(m.d)
	cmds := []tea.Cmd{a.loadCatalog(c)}
	for _, t := range waiting {
		if why := t.stayReason(); why != "" { // begun, or running, meanwhile
			cmds = append(cmds, a.showToast(why))
			continue
		}
		cmds = append(cmds, a.switchConn(t, c, cmp.Or(t.wantSchema, c.Schema)))
	}
	return tea.Batch(cmds...)
}

// catalogMsg is what loadCatalog read.
type catalogMsg struct {
	conn    *Conn
	schemas []string
	current string
	tables  []db.Table
	funcs   []db.Function
	seqs    []db.Sequence
	err     error
}

// loadCatalog reads c's schemas and every schema's tables on its Meta; a
// table's columns wait until it is opened (§8.4).
func (a *App) loadCatalog(c *Conn) tea.Cmd {
	meta, eng := c.Meta, c.eng
	return func() tea.Msg {
		ctx := context.Background()
		schemas, current, err := eng.Schemas(ctx, meta)
		if err != nil {
			return catalogMsg{conn: c, err: err}
		}
		tables, err := eng.Tables(ctx, meta)
		if err != nil {
			return catalogMsg{conn: c, err: err}
		}
		// ponytail: functions and sequences are no reason to lose the tables: a
		// read of them that fails shows none
		funcs, _ := eng.Functions(ctx, meta)
		seqs, _ := eng.Sequences(ctx, meta)
		return catalogMsg{conn: c, schemas: schemas, current: current, tables: tables, funcs: funcs, seqs: seqs, err: err}
	}
}

func (a *App) gotCatalog(m catalogMsg) tea.Cmd {
	if a.sessionOf(m.conn) == nil { // its session closed meanwhile
		return nil
	}
	if m.err != nil {
		a.dropPending(m.conn)
		return a.showToast(i18n.T("%s：%s", m.conn.Spec.Name, m.err))
	}
	c := m.conn
	c.Schemas, c.Tables, c.funcs, c.seqs = m.schemas, m.tables, m.funcs, m.seqs
	// home stays once set, while there: what Meta's current one is moves
	// with MySQL's USE in a quick SQL, and the tree keeps what is open (§7.8)
	if !slices.Contains(c.Schemas, c.home) {
		c.home = m.current
	}
	if c.home == "" && len(c.Schemas) > 0 {
		c.home = c.Schemas[0]
	}
	if !slices.Contains(c.Schemas, c.Schema) { // the first load, or the schema is gone (§7.8)
		c.Schema = c.home
	}
	cmds := a.resolvePending(c)
	for _, t := range a.queries() { // opened before the tree had a schema, or in one gone: the connection's (§8.6)
		if t.conn == c && t.want == nil && !slices.Contains(c.Schemas, t.schema) { // one waiting for another connection has its schema
			t.schema = c.Schema
			cmds = append(cmds, a.keepQuery(t))
		}
	}
	a.clampTree()
	for _, t := range c.Tables { // R dropped the columns; an open table keeps showing them (§7.8)
		if _, ok := c.cols[idOf(t)]; !ok && a.sessionOf(c).opened(tableNode(c, t), false) {
			cmds = append(cmds, a.fetchCols(c, t))
		}
		if _, ok := c.idx[idOf(t)]; !ok && a.sessionOf(c).opened(tableNode(c, t), false) {
			cmds = append(cmds, a.fetchObjects(c, t))
		}
	}
	return tea.Batch(cmds...)
}

// connNode is connection c's node ID; schemaNode, groupNode and tableNode
// those under it, windowNode and paneNode the workspace's: a tab's ID has
// its pane's in it.
// flat is whether c has no schema level in the tree: a SQLite file is the
// schema, Tables and Views hang from it (F14.1).
func (c *Conn) flat() bool { return c.Spec.Engine == "sqlite" }

func connNode(c *Conn) string { return "conn:" + c.Spec.Name }

func tableNode(c *Conn, t db.Table) string {
	return connNode(c) + "/table:" + t.Schema + "." + t.Name
}

func schemaNode(c *Conn, sc string) string { return connNode(c) + "/schema:" + sc }

func groupNode(c *Conn, sc string, views bool) string {
	if views {
		return schemaNode(c, sc) + "/views"
	}
	return schemaNode(c, sc) + "/tables"
}

func windowNode(w *Window) string { return fmt.Sprintf("window:%d", w.ID) }

func paneNode(w *Window, p *Pane) string { return fmt.Sprintf("%s/pane:%d", windowNode(w), p.ID) }

// revealTab puts the tree's cursor on the node of pane p's current tab,
// the nodes above it opened, as nvim-tree's find_file (F3.29): a table's
// under its schema, any other's in the workspace. Not showing (a filter,
// the catalog not in), nothing changes: the cursor stays, no node opens.
func (a *App) revealTab(p *Pane) {
	tab := p.tab()
	if tab == nil || p == a.win().Tree {
		return
	}
	path := []string{"workspace", windowNode(a.win())}
	is := func(n node) bool { return n.kind == nodeTab && n.pane == p && n.tab == p.Cur && n.win == a.sess.Active }
	if t := tab.Data; t != nil {
		path = []string{connNode(t.conn), schemaNode(t.conn, t.table.Schema), groupNode(t.conn, t.table.Schema, t.table.View())}
		is = func(n node) bool { return n.kind == nodeTable && n.conn == t.conn && idOf(n.table) == idOf(t.table) }
	}
	was := maps.Clone(a.sess.open)
	for _, id := range path {
		a.sess.open[id] = true
	}
	ns, _ := a.treeNodes()
	if i := slices.IndexFunc(ns, is); i >= 0 {
		a.treeGo(i)
		return
	}
	a.sess.open = was
}

// nodeKind is what a tree node stands for (§7.8).
type nodeKind int

const (
	nodeConn nodeKind = iota
	nodeSchema
	nodeGroup // Tables or Views
	nodeTable // a table or a view
	nodeColumn
	nodeObjects // a table's Indexes or Keys, a schema's Functions or Sequences (F14.3)
	nodeIndex
	nodeKey
	nodeFunc
	nodeSequence
	nodeWorkspace
	nodeWindow
	nodeTab
	nodeSep       // a rule between the groups of connections, no node (F10.6)
	nodeQueries   // the root over the query files
	nodeQueryFile // one of them
)

// node is one of the tree's rows: what it stands for, and how it draws.
type node struct {
	ui.TreeNode
	id     string // stable while the node exists: expansion and the cursor go by it
	kind   nodeKind
	conn   *Conn    // the connection it is under; nil in the workspace
	schema string   // the schema it is under; "" outside them
	table  db.Table // a table's, or a column's table
	column string
	index  db.Index    // an index leaf's
	key    db.Key      // a key leaf's
	fn     db.Function // a function leaf's
	query  string      // a query file's name
	win    int
	pane   *Pane
	tab    int
}

// opened is whether node id of s's tree shows its children: as the user
// left it, else def, §7.8's default.
func (s *Session) opened(id string, def bool) bool {
	if v, ok := s.open[id]; ok {
		return v
	}
	return def
}

// treeNodes is what the tree shows, top to bottom (§7.8): the
// connections, a connected one's schemas, their Tables and Views, tables
// and, opened, their columns; then the workspace's windows, panes and
// tabs. A filter keeps the tables and views it matches with the nodes
// above them, opened; matches is how many.
//
// ponytail: rebuilt on every call, 4–5 times a key press. Measured at
// 160×45: 1.7ms a call at 10 schemas × 100 tables, 4ms at 50 × 100, 19ms at
// 20 × 1000 (46ms filtering), so 5 / 16 / 90ms a key. Build it once per
// Update, or group the tables by schema up front, if catalogs that big turn up.
func (a *App) treeNodes() (ns []node, matches int) {
	s, ic, th := a.sess, a.icons, a.theme
	if len(s.Conns) == 0 { // a start with no connection: the one line of drawSidebar (F7.4)
		return nil, 0
	}
	filter := a.win().tree.filter.Text
	filtering := filter != ""
	pos := map[string][]int{} // what the filter kept, by table node ID
	for _, c := range s.Conns {
		names := make([]string, len(c.Tables))
		for i, t := range c.Tables {
			names[i] = t.Name
		}
		for _, m := range ui.Filter(filter, names) {
			pos[tableNode(c, c.Tables[m.Index])] = m.Pos
		}
	}
	open := func(id string, def bool) bool { return filtering || a.sess.opened(id, def) }
	current := "" // the node of the table ↵ lands on, and its tab (§7.8)
	target := a.openTarget()
	if t := dataOf(target); t != nil {
		current = tableNode(t.conn, t.table)
	}
	add := func(n node) bool {
		ns = append(ns, n)
		return n.Branch && n.Open
	}

	group := "" // the one the connections so far are in: a rule marks where another begins (F10.6)
	for _, c := range s.Conns {
		if g := c.Spec.Group; g != group && !filtering {
			group = g
			add(node{id: "sep:" + g, kind: nodeSep, TreeNode: ui.TreeNode{Sep: true, Text: g}})
		}
		cid := connNode(c)
		cn := node{id: cid, kind: nodeConn, conn: c, TreeNode: ui.TreeNode{Branch: true, Open: c.State == connReady && open(cid, true), Icon: ic.Engine(c.Spec.Engine), IconFg: th.Info, Text: c.Spec.Name}}
		if c.readOnly { // dim after the name, as … (§7.8)
			cn.Aside = ic.Lock.Text
		}
		switch c.State {
		case connConnecting:
			cn.Aside = strings.TrimSpace(cn.Aside + " …")
		case connFailed:
			cn.TextFg = th.Error
		case connIdle, connReady: // the name as it is
		}
		if c.down() != nil { // a redial failed: its next request tries again (F8.19)
			cn.TextFg = th.Error
		}
		if filtering && !slices.ContainsFunc(c.Tables, func(t db.Table) bool { _, ok := pos[tableNode(c, t)]; return ok }) {
			continue
		}
		if !add(cn) {
			continue
		}
		flat := c.flat() // the file is the schema: Tables and Views hang from it (F14.1)
		depth := func(d int) int { return d - b2i(flat) }
		for _, sc := range c.Schemas {
			var groups [2][]db.Table // Tables, Views
			for _, t := range c.Tables {
				if t.Schema == sc {
					groups[b2i(t.View())] = append(groups[b2i(t.View())], t)
				}
			}
			kept := func(ts []db.Table) (out []db.Table) {
				for _, t := range ts {
					if _, ok := pos[tableNode(c, t)]; ok {
						out = append(out, t)
					}
				}
				return out
			}
			if len(kept(groups[0]))+len(kept(groups[1])) == 0 && filtering {
				continue
			}
			id := schemaNode(c, sc)
			if !flat && !add(node{id: id, kind: nodeSchema, conn: c, schema: sc, TreeNode: ui.TreeNode{Depth: 1, Branch: true, Open: open(id, sc == c.home), Icon: ic.Schema, IconFg: th.PK, Text: sc}}) {
				continue
			}
			for g, ts := range groups {
				label, icon, gid := "Tables", ic.Table, groupNode(c, sc, g == 1)
				if g == 1 {
					label, icon = "Views", ic.View
				}
				shown := kept(ts)
				if filtering && len(shown) == 0 {
					continue
				}
				gn := node{id: gid, kind: nodeGroup, conn: c, schema: sc, TreeNode: ui.TreeNode{Depth: depth(2), Branch: len(ts) > 0, Open: open(gid, sc == c.home && g == 0), Icon: icon, IconFg: th.Func, Text: fmt.Sprintf("%s (%d)", label, len(ts))}}
				if !add(gn) {
					continue
				}
				for _, t := range shown {
					tid := tableNode(c, t)
					rows := ui.Magnitude(t.Rows)
					if t.Kind == "v" || flat { // a plain view has no rows to count, a file keeps no estimate
						rows = ""
					}
					tn := node{id: tid, kind: nodeTable, conn: c, schema: sc, table: t, TreeNode: ui.TreeNode{
						Depth: depth(3), Branch: true, Open: !filtering && a.sess.opened(tid, false), Icon: icon, IconFg: th.Func, Text: t.Name, Pos: pos[tid],
						Note: rows, NoteFg: th.Border, Current: tid == current,
					}}
					if !add(tn) {
						continue
					}
					cols := c.cols[idOf(t)] // none until fetched: no children, no "loading" (§7.8)
					for _, col := range cols.Cols {
						coln := node{id: tid + "/" + col.Name, kind: nodeColumn, conn: c, schema: sc, table: t, column: col.Name, TreeNode: ui.TreeNode{
							Depth: depth(4), Icon: ic.Column, IconFg: th.Dim, Text: col.Name, Note: col.Type, NoteFg: th.Dim,
						}}
						if slices.Contains(cols.PK, col.Name) {
							coln.Icon, coln.IconFg = ic.Key, th.PK
						}
						add(coln)
					}
					if filtering { // only tables and views take part (F14.3)
						continue
					}
					if ixs, ok := c.idx[idOf(t)]; ok && hasIndexes(t) {
						xid := "indexes:" + tid // not tid + "/…": a column may be named so
						if add(node{id: xid, kind: nodeObjects, conn: c, schema: sc, table: t, TreeNode: ui.TreeNode{Depth: depth(4), Branch: len(ixs) > 0, Open: a.sess.opened(xid, false), Icon: ic.Index, IconFg: th.Func, Text: fmt.Sprintf("Indexes (%d)", len(ixs))}}) {
							for _, ix := range ixs {
								add(node{id: xid + "/" + ix.Name, kind: nodeIndex, conn: c, schema: sc, table: t, index: ix, TreeNode: ui.TreeNode{Depth: depth(5), Icon: ic.Index, IconFg: th.Dim, Text: indexText(ix)}})
							}
						}
					}
					if keys, ok := c.keys[idOf(t)]; ok && hasKeys(t) {
						kid := "keys:" + tid
						if add(node{id: kid, kind: nodeObjects, conn: c, schema: sc, table: t, TreeNode: ui.TreeNode{Depth: depth(4), Branch: len(keys) > 0, Open: a.sess.opened(kid, false), Icon: ic.Key, IconFg: th.PK, Text: fmt.Sprintf("Keys (%d)", len(keys))}}) {
							for i, k := range keys {
								add(node{id: fmt.Sprintf("%s/%d", kid, i), kind: nodeKey, conn: c, schema: sc, table: t, key: k, TreeNode: ui.TreeNode{Depth: depth(5), Icon: ic.Key, IconFg: th.Dim, Text: keyText(c, k)}})
							}
						}
					}
				}
			}
			if filtering {
				continue
			}
			var fns []db.Function // what a schema has besides tables and views: a group each when it has any (F14.3)
			for _, f := range c.funcs {
				if f.Schema == sc {
					fns = append(fns, f)
				}
			}
			if fid := schemaNode(c, sc) + "/functions"; len(fns) > 0 && add(node{id: fid, kind: nodeObjects, conn: c, schema: sc, TreeNode: ui.TreeNode{Depth: depth(2), Branch: true, Open: a.sess.opened(fid, false), Icon: ic.Func, IconFg: th.Func, Text: fmt.Sprintf("Functions (%d)", len(fns))}}) {
				for _, f := range fns {
					add(node{id: fid + "/" + functionText(f), kind: nodeFunc, conn: c, schema: sc, fn: f, TreeNode: ui.TreeNode{Depth: depth(3), Icon: ic.Func, IconFg: th.Dim, Text: functionText(f)}})
				}
			}
			var seqs []db.Sequence
			for _, q := range c.seqs {
				if q.Schema == sc {
					seqs = append(seqs, q)
				}
			}
			if qid := schemaNode(c, sc) + "/sequences"; len(seqs) > 0 && add(node{id: qid, kind: nodeObjects, conn: c, schema: sc, TreeNode: ui.TreeNode{Depth: depth(2), Branch: true, Open: a.sess.opened(qid, false), Icon: ic.Sequence, IconFg: th.Func, Text: fmt.Sprintf("Sequences (%d)", len(seqs))}}) {
				for _, q := range seqs {
					add(node{id: qid + "/" + q.Name, kind: nodeSequence, conn: c, schema: sc, TreeNode: ui.TreeNode{Depth: depth(3), Icon: ic.Sequence, IconFg: th.Dim, Text: q.Name}})
				}
			}
		}
	}
	if filtering { // the workspace isn't what a filter looks for (§7.8)
		return ns, len(pos)
	}
	if len(a.groupNames()) > 0 { // a rule with no title between the connections and the rest
		add(node{id: "sep:", kind: nodeSep, TreeNode: ui.TreeNode{Sep: true}})
	}
	if add(node{id: "workspace", kind: nodeWorkspace, TreeNode: ui.TreeNode{Branch: true, Open: a.sess.opened("workspace", true), Icon: ic.Window, IconFg: th.Info, Text: i18n.T("工作区")}}) {
		a.workspaceNodes(add, target)
	}
	if add(node{id: "queries", kind: nodeQueries, TreeNode: ui.TreeNode{Branch: true, Open: a.sess.opened("queries", true), Icon: ic.Query, IconFg: th.Info, Text: "Queries"}}) {
		for _, name := range a.queryList() {
			open := a.queryOpen(name, nil)
			cn := node{id: "queries/" + name, kind: nodeQueryFile, query: name, TreeNode: ui.TreeNode{
				Depth: 1, Icon: ic.Query, IconFg: th.Info, Text: name, Current: open,
				Note: a.state.Queries[name].Conn, NoteFg: th.Dim,
			}}
			add(cn)
		}
	}
	return ns, len(pos)
}

// workspaceNodes adds the windows and their tabs under the workspace root,
// the tabs of a window by pane number, each marked with its pane's (F10.5).
func (a *App) workspaceNodes(add func(node) bool, target *Pane) {
	s, th, ic := a.sess, a.theme, a.icons
	for wi, w := range s.Windows {
		var leaves []*Pane
		if w.Root != nil {
			leaves = w.Root.Leaves()
		}
		tabs := 0
		for _, p := range leaves {
			tabs += len(p.Tabs)
		}
		wid := windowNode(w)
		if !add(node{id: wid, kind: nodeWindow, win: wi, TreeNode: ui.TreeNode{Depth: 1, Branch: tabs > 0, Open: a.sess.opened(wid, true), Icon: ic.Window, IconFg: th.Info, Text: w.Name}}) {
			continue
		}
		for n, p := range leaves { // ⟨n⟩ by order: the sidebar is 0 (§7.8)
			for i, tb := range p.Tabs {
				icon, _ := a.tabIcon(&p.Tabs[i])
				// a blank for a landing tab's: its name in line with the others'
				icon.Text = cmp.Or(icon.Text, " ")
				fg := th.Info // a query's
				if tb.Data != nil {
					fg = th.Func
				}
				tn := node{id: fmt.Sprintf("%s/tab:%d", paneNode(w, p), i), kind: nodeTab, win: wi, pane: p, tab: i, TreeNode: ui.TreeNode{
					Depth: 2, Prefix: ic.Number(n + 1), Icon: icon, IconFg: fg, Text: tb.Name,
					Current: wi == s.Active && p == target && i == p.Cur,
				}}
				if tb.Data != nil {
					tn.Aside = strings.Join(tabSummary(tb.Data), " · ")
				}
				add(tn)
			}
		}
	}
}

func b2i(b bool) int {
	if b {
		return 1
	}
	return 0
}

// treeRows is how many nodes the sidebar shows.
func (a *App) treeRows() int { return ui.TreeRows(a.sidebarRect().Dy() - 2) }

// treeAt is where the cursor is among ns, on its node while that shows,
// else on the row it was on, a rule not being where it rests (F10.6); and
// the first row the view shows.
func (a *App) treeAt(ns []node) (cursor, top int) {
	t := a.win().tree
	if i := slices.IndexFunc(ns, func(n node) bool { return n.id == t.at }); i >= 0 {
		t.cursor = i
	}
	for t.cursor < len(ns)-1 && ns[t.cursor].kind == nodeSep {
		t.cursor++
	}
	cursor, top = ui.ListView(t.cursor, t.top, len(ns), a.treeRows())
	if top == cursor && cursor > 0 && ns[cursor-1].kind == nodeSep { // a rule shows with the node it is over
		top--
	}
	return cursor, top
}

// clampTree settles the cursor on a node; its schema is its connection's
// Schema from now on, until the cursor is under another (§7.8).
func (a *App) clampTree() {
	ns, _ := a.treeNodes()
	t := &a.win().tree
	t.cursor, t.top = a.treeAt(ns)
	if t.cursor < len(ns) {
		n := ns[t.cursor]
		if t.at = n.id; n.schema != "" {
			n.conn.Schema = n.schema
		}
	}
}

// treeGo puts the cursor on row i.
func (a *App) treeGo(i int) {
	t := &a.win().tree
	t.cursor, t.at = i, ""
	a.clampTree()
}

// treeMove moves the cursor d nodes, a rule not counting, and stops at the ends.
func (a *App) treeMove(d int) {
	ns, _ := a.treeNodes()
	cur, _ := a.treeAt(ns)
	step := 1
	if d < 0 {
		step = -1
	}
	for ; d != 0; d -= step {
		i := cur + step
		for i >= 0 && i < len(ns) && ns[i].kind == nodeSep {
			i += step
		}
		if i < 0 || i >= len(ns) {
			break
		}
		cur = i
	}
	a.treeGo(cur)
}

// treeNode is the node under the cursor.
func (a *App) treeNode() (node, bool) {
	ns, _ := a.treeNodes()
	if cur, _ := a.treeAt(ns); cur < len(ns) {
		return ns[cur], true
	}
	return node{}, false
}

// scrollTree moves the view, not the cursor, which is only pulled back into
// it, as in nvim (§7.8).
func (a *App) scrollTree(notches int) {
	ns, _ := a.treeNodes()
	t := &a.win().tree
	cur, _ := a.treeAt(ns)
	rows := a.treeRows()
	t.top = max(min(t.top+notches*wheelStep, len(ns)-rows), 0)
	a.treeGo(max(min(cur, t.top+rows-1), t.top))
}

// treeFold opens or closes n; a table opening fetches its columns unless
// the catalog has them (§8.4), a connection not connected connects
// (§7.8「连接节点」).
func (a *App) treeFold(n node, open bool) tea.Cmd {
	if !n.Branch {
		return nil
	}
	a.sess.open[n.id] = open
	a.clampTree()
	switch {
	case open && n.kind == nodeConn:
		return a.connect(n.conn)
	case open && n.kind == nodeTable:
		var cmds []tea.Cmd
		if _, ok := n.conn.cols[idOf(n.table)]; !ok {
			cmds = append(cmds, a.fetchCols(n.conn, n.table))
		}
		if _, ok := n.conn.idx[idOf(n.table)]; !ok {
			cmds = append(cmds, a.fetchObjects(n.conn, n.table))
		}
		return tea.Batch(cmds...)
	}
	return nil
}

// refreshTree is R (§7.8): the current connection's catalog loads anew,
// its columns dropped; on a connection not connected, or failed, the tree
// cursor's, it connects instead, as opening it does.
func (a *App) refreshTree() tea.Cmd {
	if n, ok := a.treeNode(); ok && a.focused() == a.win().Tree && n.kind == nodeConn && n.conn.State != connReady {
		return a.treeFold(n, true)
	}
	c := a.conn()
	if c == nil {
		return a.noConnection()
	}
	if c.State != connReady { // a layout's other session, still connecting (F6.5)
		return a.notReady(c)
	}
	c.dropCols()
	return a.loadCatalog(c)
}

// treeExpand is l: a closed node opens, an open one hands the cursor to its
// first child; a leaf does nothing (§7.8).
func (a *App) treeExpand() tea.Cmd {
	n, ok := a.treeNode()
	switch {
	case !ok || !n.Branch:
	case !n.Open:
		return a.treeFold(n, true)
	default:
		ns, _ := a.treeNodes()
		if cur, _ := a.treeAt(ns); cur+1 < len(ns) && ns[cur+1].Depth > n.Depth {
			a.treeGo(cur + 1)
		}
	}
	return nil
}

// treeCollapse is h: an open node closes; a closed one or a leaf hands the
// cursor to its parent (§7.8).
func (a *App) treeCollapse() tea.Cmd {
	n, ok := a.treeNode()
	if !ok {
		return nil
	}
	if n.Branch && n.Open {
		return a.treeFold(n, false)
	}
	ns, _ := a.treeNodes()
	cur, _ := a.treeAt(ns)
	for i := cur - 1; i >= 0; i-- {
		if ns[i].Depth < n.Depth {
			a.treeGo(i)
			break
		}
	}
	return nil
}

// treeOpen is ↵ (and t, tablesOnly) on the cursor's node (§7.8): a table
// opens, a column opens its table on that column, a tab is switched to, a
// window too; the rest open and close. t only opens tables, as ↵ does
// since a pane has a tab of a table at most (F3.37).
func (a *App) treeOpen(tablesOnly bool) tea.Cmd {
	n, ok := a.treeNode()
	switch {
	case !ok:
	case n.kind == nodeTable:
		return a.openTable(n.conn, n.table)
	case tablesOnly:
	case n.kind == nodeQueryFile:
		return a.openQueryFile(n.query)
	case n.kind == nodeColumn:
		cmd := a.openTable(n.conn, n.table)
		a.gotoColumn(n.conn, n.table, n.column)
		return cmd
	case n.kind == nodeIndex:
		a.openIndexDDL(n.conn, n.table, n.index)
	case n.kind == nodeKey:
		if n.key.Ref != nil {
			return a.openRef(n.conn, n.key.Ref)
		}
	case n.kind == nodeFunc:
		return a.openFunction(n.conn, n.fn)
	case n.kind == nodeSequence:
	case n.kind == nodeTab: // another window's: that window first (F5.2)
		a.selectWindow(n.win)
		a.showTab(n.pane, n.tab)
	case n.kind == nodeWindow: // l and h open and close it
		a.selectWindow(n.win)
	default:
		return a.treeFold(n, !n.Open)
	}
	return nil
}

// tabSummary is what t shows when not all rows in the default order: its
// WHERE, its ORDER (§7.8).
func tabSummary(t *dataTab) (out []string) {
	if s := t.shown; strings.TrimSpace(s.applied) != "" {
		out = append(out, s.applied)
	}
	if s := t.shown; len(s.order) > 0 {
		out = append(out, orderText(s.order))
	}
	return out
}

// gotoColumn puts the cursor of the table just opened on column col: now,
// or once its page is in. Hidden by COLS, it stays where it is (§7.8).
func (a *App) gotoColumn(c *Conn, t db.Table, col string) {
	dt := dataOf(a.focused())
	if dt == nil || dt.conn != c || idOf(dt.table) != idOf(t) {
		return
	}
	dt.wantCol = col
	if dt.page.Cols != nil {
		dt.applyWantCol()
	}
}

// treeFilter puts the keys in the tree's filter row, showing and focusing
// the tree first.
func (a *App) treeFilter() {
	win := a.win()
	win.TreeOpen = true
	a.showPane(win.Tree.ID)
	if win.Focus == win.Tree.ID {
		win.tree.filtering = true
		win.tree.filter.Pos = len(win.tree.filter.Text)
	}
}

// filterKey edits the tree's filter, nvim-tree's live filter (§7.8): the
// cursor goes to the first match as it changes, and ↵ keeps it for the
// list; esc clears it, the cursor staying on its node.
func (a *App) filterKey(k keymap.Key) {
	t := &a.win().tree
	switch k {
	case "<CR>":
		t.filtering = false
		a.treeFirstMatch()
	case keymap.Esc:
		t.filter, t.filtering = ui.Input{}, false
		a.clampTree()
	default:
		if editInput(&t.filter, k) {
			a.treeFirstMatch()
		}
	}
}

// treeFirstMatch puts the cursor on the first table or view the filter
// kept, else the first node.
func (a *App) treeFirstMatch() {
	ns, _ := a.treeNodes()
	a.treeGo(max(slices.IndexFunc(ns, func(n node) bool { return n.kind == nodeTable }), 0))
}

// openTarget is the pane a table opens in (§5, §12): the focused one;
// with the tree or the result area focused, the one focused most recently,
// a query's too (F3.37); the first, if none ever was. Never the result
// area.
func (a *App) openTarget() *Pane {
	win := a.win()
	if p := a.focused(); p != win.Tree && p != win.Result {
		return p
	}
	var recent *Pane
	for _, p := range win.Root.Leaves() {
		if p != win.Result && (recent == nil || a.recent(p.ID, recent.ID)) {
			recent = p
		}
	}
	return recent
}
