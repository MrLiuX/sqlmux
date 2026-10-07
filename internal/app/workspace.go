package app

import (
	"cmp"
	"context"
	"errors"
	"io/fs"
	"os"
	"slices"
	"strconv"
	"strings"
	"sync"

	tea "charm.land/bubbletea/v2"

	"sqlmux/internal/config"
	"sqlmux/internal/db"
	"sqlmux/internal/db/mysql"
	"sqlmux/internal/db/postgres"
	"sqlmux/internal/i18n"
	"sqlmux/internal/keymap"
	"sqlmux/internal/ui"
)

// Pane is the sidebar or one of the split tree's: tables and consoles,
// side by side in its tabs (§5).
type Pane struct {
	ID        int // stable; the sidebar is 0
	Tabs      []Tab
	Cur, Prev int // tab bar * and - (T-01)
}

// Object is the title's "· name" part: the current tab.
func (p *Pane) Object() string {
	if t := p.tab(); t != nil {
		return t.Name
	}
	return ""
}

// tab is the current tab; nil when the pane has none.
func (p *Pane) tab() *Tab {
	if p.Cur < len(p.Tabs) {
		return &p.Tabs[p.Cur]
	}
	return nil
}

// landing is what a new tab is until a table or a console takes its place
// (§5「引导页」).
func landing() Tab { return Tab{Name: i18n.T("新 tab")} }

// landing reports whether t shows the landing page: a new tab, or no tab.
func (t *Tab) landing() bool { return t == nil || t.Data == nil && t.Console == nil && t.Result == nil }

// putTab puts tab in pane p: over the current tab, or after the last one,
// which it becomes, the current one the previous.
func putTab(p *Pane, tab Tab, over bool) {
	if over && len(p.Tabs) > 0 {
		p.Tabs[p.Cur] = tab
		return
	}
	p.Prev = p.Cur
	if len(p.Tabs) == 0 { // an empty pane: there is no tab to go back to
		p.Prev = -1
	}
	p.Tabs = append(p.Tabs, tab)
	p.Cur = len(p.Tabs) - 1
}

type Window struct {
	ID       int // stable within its session, as a pane's: the tree's expansion and SPC l go by it
	Name     string
	TreeOpen bool  // the ⟨0⟩ sidebar is open, not folded to its thin bar
	TreeW    int   // the sidebar's width once dragged (§7.8); 0 is the default
	Tree     *Pane // ⟨0⟩ sidebar, not part of the split tree (D-04)
	// Result is the result area, at the bottom from the first run until it
	// is closed; resultRatio is the share of the rest above it, kept then
	// (0: 1 − result_height), and log the runs', kept too (§11).
	Result      *Pane
	resultRatio float64
	log         []ui.LogLine
	tree        treeState
	Root        *Node
	Focus       int // pane ID
	Zoom        int // zoomed pane ID; 0 = none (P-03)
	lastID      int // highest pane ID handed out

	focusTick int
	focusedAt map[int]int // pane ID → focusTick when it last got focus

	resultHidden bool // SPC r took the result area out of the layout, its tabs kept (§11)
}

// focus moves focus to pane id, remembering when: moving by direction
// prefers the neighbour focused most recently (§5). Leaving a pane leaves
// its input too: the tree's filter row, a table's query bar.
func (w *Window) focus(id int) {
	if w.focusedAt == nil {
		w.focusedAt = map[int]int{}
	}
	w.focusTick++
	w.Focus, w.focusedAt[id] = id, w.focusTick
	if id != w.Tree.ID {
		w.tree.filtering = false
	}
	for _, p := range w.Root.Leaves() {
		if t := dataOf(p); t != nil && p.ID != id {
			t.stopTyping()
		}
	}
}

// pane is the window's pane id, the sidebar too; nil when there is none.
func (w *Window) pane(id int) *Pane {
	if id == w.Tree.ID {
		return w.Tree
	}
	for _, p := range w.Root.Leaves() {
		if p.ID == id {
			return p
		}
	}
	return nil
}

// Session is a workspace (tech-design §5): its windows, and every
// connection of connections.toml, each connected when first used.
type Session struct {
	Name    string
	Conns   []*Conn         // connections.toml's, in its order: the tree's roots (§7.8)
	start   *Conn           // the one it was started on, a layout's: what the file keeps (§14)
	last    *Conn           // the current connection where nothing focused names one (§5)
	open    map[string]bool // tree nodes opened or closed by hand, by node ID; the rest as §7.8 says
	Windows []*Window
	Active  int
	winID   int         // the highest window ID handed out
	prevWin int         // the ID of the window used before the one shown, SPC l's; 0 for none
	RunSeq  int         // the last run's number, #42 on its result tabs (§11)
	warning string      // console_1's file could not be read: a toast on start (§11)
	closed  []closedTab // the tabs closed last, the newest last: tab.reopen's (F5.16)

	// When each tab was last the focused pane's current one, a count by
	// what the tab holds, and the tab that is now (F5.19, §12). Not kept.
	// ponytail: a closed tab stays in the map; a session opens few enough.
	tabSeq  map[any]int
	seq     int
	focused any
}

// tabKey is what a tab is known by: its table, console or result; nil for a
// landing tab, which none are.
func tabKey(tb *Tab) any {
	switch {
	case tb == nil:
	case tb.Data != nil:
		return tb.Data
	case tb.Console != nil:
		return tb.Console
	case tb.Result != nil:
		return tb.Result
	}
	return nil
}

// touch notes that tb, the current tab of the focused pane, is the one in
// use: the Tab scope of the palette lists them newest first (F5.19). The
// sidebar and a landing tab are none, the one before staying the one.
func (s *Session) touch(tb *Tab) {
	if key := tabKey(tb); key != nil && key != s.focused {
		if s.tabSeq == nil {
			s.tabSeq = map[any]int{}
		}
		s.seq++
		s.tabSeq[key], s.focused = s.seq, key
	}
}

// connState is how far a Conn got with connecting (§7.8「连接节点」).
type connState int

const (
	connIdle connState = iota
	connConnecting
	connFailed
	connReady
)

// Conn is one connection of connections.toml as a session has it (§5):
// its Main and Meta once connected, its catalog and what is cached of it.
type Conn struct {
	Spec config.Connection
	// eng and readOnly are Spec.Engine's and Spec.ReadOnly as the connection
	// was dialed: an edit of the spec meanwhile is for the next dial (F8.16).
	eng        db.Engine
	readOnly   bool
	State      connState
	Main, Meta *db.Worker // §8.2; nil till connected
	Addr       string     // shown in the status bar, e.g. ctw@localhost:5432
	busy       int        // its requests out that C-c cancels (§8.3)
	// tx is where Main's transaction was as its last request ended: 'I'
	// out of one, 'T' in one, 'E' in one that failed; 0 before any. The
	// replies bring it: Main may be busy, its lock not to wait on (F5.11).
	tx byte
	// Manual is the transaction mode, its consoles' and its saves': a run
	// or a save begins one, which stays till committed or rolled back
	// (F5.3). Each connects in the auto mode.
	Manual bool

	Schema    string   // its Schema: the tree cursor's under it, or the last it was under (§7.8)
	Schemas   []string // the catalog's (§8.4)
	home      string   // current_schema(), whose Tables the tree opens with
	Tables    []db.Table
	cols      map[tableID]db.Columns
	colsAsked map[tableID]bool    // for names' colors (§7.3): asked once, till cols is dropped
	ddl       map[tableID]ddlText // the palette's previews (F4.2), dropped with cols

	// DB is the database, on console titles, "" for MySQL, whose schemas
	// are its databases; startedPath Main's search_path as it connected;
	// mainPath the schema a console's run last put first on it, "" when
	// not known, touched in Main's requests only (§8.6).
	DB                    string
	startedPath, mainPath string
}

func newConn(spec config.Connection) *Conn {
	c := &Conn{cols: map[tableID]db.Columns{}, colsAsked: map[tableID]bool{}, ddl: map[tableID]ddlText{}}
	c.setSpec(spec)
	return c
}

// setSpec makes spec what the connection is, for the dial after the one it
// may be in.
func (c *Conn) setSpec(spec config.Connection) {
	c.Spec, c.eng, c.readOnly = spec, engines[spec.Engine], spec.ReadOnly
}

// engines are the engines connections.toml names (§8.1).
var engines = map[string]db.Engine{"postgres": postgres.Engine{}, "mysql": mysql.Engine{}}

func (a *App) win() *Window { return a.sess.Windows[a.sess.Active] }

// conn is the current connection (§5): the focused tab's, a result tab's
// where it ran; on the tree, the one the cursor is under; each once
// connected, else the last one. That one may not be connected either, a
// layout's other sessions connect as the program runs (F6.5): what runs
// on it looks first (Conn.State).
func (a *App) conn() *Conn {
	var c *Conn
	if p := a.focused(); p == a.win().Tree {
		if n, ok := a.treeNode(); ok {
			c = n.conn
		}
	} else if t := p.tab(); t != nil {
		switch {
		case t.Data != nil:
			c = t.Data.conn
		case t.Console != nil:
			c = t.Console.conn
		case t.Result != nil && t.Result.run != nil:
			c = t.Result.run.conn
		}
	}
	if c != nil && c.State == connReady {
		return c
	}
	return a.sess.last
}

// inTx reports whether c's Main is in a transaction, as far as its last
// request tells.
func (c *Conn) inTx() bool { return c != nil && c.tx != 0 && c.tx != 'I' }

// cancel cancels what c's Main and Meta are running (§8.3).
func (c *Conn) cancel() {
	if c == nil || c.State != connReady {
		return
	}
	c.Meta.Cancel()
	c.Main.Cancel()
}

// dropCols forgets the columns fetched: the catalog loads anew (§7.8).
func (c *Conn) dropCols() {
	clear(c.cols)
	clear(c.colsAsked)
	clear(c.ddl)
}

// newSession is a session's default workspace (§5) over conns, start the
// one connected: one window, data, with the sidebar, a pane with no tab
// and console_n on start beside it at 1 : 1 (§5, F8.9), which has what its
// file kept (§11). A file that cannot be read leaves that pane with no tab
// too: opened empty, the autosave would write over it.
func newSession(name string, conns []*Conn, start *Conn, console string) *Session {
	s := &Session{Name: name, Conns: conns, start: start, last: start, open: map[string]bool{}}
	var w *Window
	w, s.warning = s.newWindow(start, console)
	s.Windows = []*Window{w}
	return s
}

// freeWindowName is the name a new window gets: data, else data-2, data-3…
// the first no window of s has (F8.11).
func (s *Session) freeWindowName() string {
	name := "data"
	for n := 2; slices.ContainsFunc(s.Windows, func(w *Window) bool { return w.Name == name }); n++ {
		name = "data-" + strconv.Itoa(n)
	}
	return name
}

// newWindow is a window of the default layout (§5): the sidebar, a pane
// with no tab and console_n on conn beside it at 1 : 1 (§5, F8.9), named
// by freeWindowName; warning says why the console's file could not be read.
func (s *Session) newWindow(conn *Conn, console string) (w *Window, warning string) {
	data := &Pane{ID: 1, Prev: -1}
	consolePane := &Pane{ID: 2, Prev: -1}
	if conn != nil { // a console is a connection's: none yet at a start with none (F7.4)
		if cons, err := openConsole(conn, console); err != nil {
			warning = err.Error()
		} else {
			consolePane.Tabs = []Tab{{Name: console, Console: cons}}
		}
	}
	return s.window(&Node{Split: Horiz, Ratio: 0.5, A: leaf(data), B: leaf(consolePane)}, 2), warning
}

// blankWindow is the window window.new and session.new make: the sidebar and
// one empty pane, the landing page, no console, as tmux's new-window (§5,
// F8.18).
func (s *Session) blankWindow() *Window { return s.window(leaf(&Pane{ID: 1, Prev: -1}), 1) }

// window is a new window of s over root, whose panes number up to lastID,
// named by freeWindowName, the pane 1 focused.
func (s *Session) window(root *Node, lastID int) *Window {
	s.winID++
	w := &Window{ID: s.winID, Name: s.freeWindowName(), TreeOpen: true, Tree: &Pane{ID: 0}, Root: root, lastID: lastID}
	w.focus(1)
	return w
}

// selectWindow shows window i of the session, the one shown so far being
// SPC l's way back.
func (a *App) selectWindow(i int) {
	s := a.sess
	if i == s.Active || i < 0 || i >= len(s.Windows) {
		return
	}
	s.prevWin, s.Active = s.Windows[s.Active].ID, i
	a.winRename = nil // the name typed is the window's left
}

// windowByID is the index of the session's window id; -1 for none.
func (s *Session) windowByID(id int) int {
	return slices.IndexFunc(s.Windows, func(w *Window) bool { return w.ID == id })
}

// addWindow is SPC c: a window of one empty pane at the end, shown (F8.18).
func (a *App) addWindow() tea.Cmd {
	a.sess.Windows = append(a.sess.Windows, a.sess.blankWindow())
	a.selectWindow(len(a.sess.Windows) - 1)
	return nil
}

// cycleWindow is SPC n (d 1) and SPC p (d -1): the next or previous
// window, around the ends.
func (a *App) cycleWindow(d int) {
	n := len(a.sess.Windows)
	a.selectWindow((a.sess.Active + d + n) % n)
}

// renameWindowKey edits the name typed in the status bar: ↵ takes it but a
// blank one, or another window's of the session, which is refused with the
// input left; esc drops it (§5, F8.11).
func (a *App) renameWindowKey(k keymap.Key) tea.Cmd {
	name, done := typeName(a.winRename, k)
	if !done {
		return nil
	}
	if name != "" && slices.ContainsFunc(a.sess.Windows, func(w *Window) bool { return w != a.win() && w.Name == name }) {
		return a.showToast(i18n.T("已有同名 window"), toastTTL)
	}
	if name != "" {
		a.win().Name = name
	}
	a.winRename = nil
	return nil
}

// nameAsk is a name typed in the status bar, over its left: label says
// what for, ↵ gives the trimmed name, never blank, to done, which is told to
// keep the input when the name was refused with a toast (esc drops it).
type nameAsk struct {
	label string
	in    ui.Input
	done  func(a *App, name string) (cmd tea.Cmd, keep bool)
}

// askNameKey is key k in that input.
func (a *App) askNameKey(k keymap.Key) tea.Cmd {
	n := a.askName
	name, done := typeName(&n.in, k)
	if !done {
		return nil
	}
	if name == "" {
		a.askName = nil
		return nil
	}
	cmd, keep := n.done(a, name)
	if !keep {
		a.askName = nil
	}
	return cmd
}

// askSessionName is session.rename: this session's name typed in the status
// bar; another session's is refused (F8.11).
func (a *App) askSessionName() {
	s := a.sess
	a.askName = &nameAsk{label: i18n.T("session 名："), in: ui.Input{Text: s.Name, Pos: len(s.Name)}, done: func(a *App, name string) (tea.Cmd, bool) {
		if a.sessionNameTaken(s, name) {
			return a.showToast(i18n.T("已有同名 session"), toastTTL), true
		}
		s.Name = name
		return nil, false
	}}
}

// sessionNameTaken is whether name is another session's than s's.
func (a *App) sessionNameTaken(s *Session, name string) bool {
	return name != s.Name && slices.ContainsFunc(a.sessions, func(o *Session) bool { return o.Name == name })
}

// askConsoleName is console.rename: the focused console's name typed in the
// status bar. Its name is its file's: the file is written, then renamed,
// the state's and the tabs' names going with it. A name that is no file's,
// or that another file, or another console open, has, is refused (F8.11).
func (a *App) askConsoleName() {
	t := consoleOf(a.focused())
	if t == nil {
		return
	}
	a.askName = &nameAsk{label: i18n.T("console 名："), in: ui.Input{Text: t.name, Pos: len(t.name)}, done: func(a *App, name string) (tea.Cmd, bool) {
		if name == t.name {
			return nil, false
		}
		path := config.ConsolePath(name)
		if err := config.CheckConsoleName(name); err != nil {
			return a.showToast(err.Error(), toastTTL), true
		}
		fi, err := os.Stat(path)
		old, _ := os.Stat(t.path)
		switch {
		case err == nil && !os.SameFile(fi, old) || a.consoleOpen(name, t): // a name of its own file only differing in case is its own
			return a.showToast(i18n.T("已有同名 console"), toastTTL), true
		case err != nil && !errors.Is(err, fs.ErrNotExist):
			return a.showToast(err.Error(), toastTTL), true
		}
		if err := t.flush(); err != nil {
			return a.saveFailed(err), true
		}
		if err := os.Rename(t.path, path); err != nil && !errors.Is(err, fs.ErrNotExist) { // no file yet: nothing typed
			return a.showToast(err.Error(), toastTTL), true
		}
		delete(a.state.Consoles, t.name)
		t.name, t.path = name, path
		for _, s := range a.sessions {
			for _, w := range s.Windows {
				for _, p := range w.Root.Leaves() {
					for i := range p.Tabs {
						if p.Tabs[i].Console == t {
							p.Tabs[i].Name = name
						}
					}
				}
			}
		}
		return a.keepConsole(t), false
	}}
}

// typeName is key k on in, a name typed in its place: ↵ ends it with the
// name trimmed, esc with none, ""; done is whether it ended.
func typeName(in *ui.Input, k keymap.Key) (name string, done bool) {
	switch k {
	case keymap.Esc:
		return "", true
	case "<CR>":
		return strings.TrimSpace(in.Text), true
	}
	editInput(in, k)
	return "", false
}

// closeWindow is window.close (§5): the window's consoles written, asked
// first when changes would go; the one used before it shown, else the one
// before it. The last one stays, as the last pane.
func (a *App) closeWindow() tea.Cmd {
	s := a.sess
	if len(s.Windows) == 1 {
		return nil
	}
	w := a.win()
	var ps []*Pane
	if w.Root != nil {
		ps = w.Root.Leaves()
	}
	return a.closeAsking(unsaved(ps...), i18n.T("这个 window 里"), consolesIn(ps...), func() tea.Cmd {
		if cmd, ok := a.flushAll(ps...); !ok {
			return cmd
		}
		i := s.Active
		s.Windows = slices.Delete(s.Windows, i, i+1)
		if s.Active = s.windowByID(s.prevWin); s.Active < 0 {
			s.Active = max(i-1, 0)
		}
		s.prevWin = 0
		return nil
	})
}

// OpenEmpty is the session of a start with no connection: the default
// layout with no console, a console being a connection's, the connection
// form open as the App begins (§14「启动时找不到连接」).
func OpenEmpty() *Session {
	s := &Session{Name: "sqlmux", open: map[string]bool{}}
	w, _ := s.newWindow(nil, config.ConsoleName(1))
	s.Windows = []*Window{w}
	return s
}

// Open is a session over specs, start the connection it connects now
// (§5); the others wait to be used.
func Open(ctx context.Context, specs []config.Connection, start int) (*Session, error) {
	conns := make([]*Conn, len(specs))
	for i, spec := range specs {
		conns[i] = newConn(spec)
	}
	d, err := dial(ctx, specs[start])
	if err != nil {
		return nil, err
	}
	conns[start].take(d)
	return newSession(specs[start].Name, conns, conns[start], config.ConsoleName(1)), nil
}

// dialed is a connection's Main and Meta as dial connected them, and
// where Main landed.
type dialed struct {
	main, meta db.Conn
	info       db.Info
}

// secrets caches what a connection's password source gave, by its name
// (§13): the keychain asks once a run, not once a session; a failed dial
// drops it, the next one asks again.
var secrets sync.Map

// dial connects spec's Main and then its Meta (§8.2).
func dial(ctx context.Context, spec config.Connection) (dialed, error) {
	spec, err := spec.Expanded()
	if err != nil {
		return dialed{}, err
	}
	pw, cached := secrets.Load(spec.Name)
	if !cached {
		s, err := spec.Secret()
		if err != nil {
			return dialed{}, err
		}
		pw = s
		secrets.Store(spec.Name, s)
	}
	eng := engines[spec.Engine]
	main, info, err := eng.Connect(ctx, spec.DSN, pw.(string), spec.ReadOnly) // a read-only connection's too (§13)
	if err != nil {
		secrets.Delete(spec.Name)
		return dialed{}, err
	}
	meta, _, err := eng.Connect(ctx, spec.DSN, pw.(string), true)
	if err != nil {
		secrets.Delete(spec.Name)
		main.Close()
		return dialed{}, err
	}
	return dialed{main, meta, info}, nil
}

// take makes c connected over d.
func (c *Conn) take(d dialed) {
	c.State, c.Main, c.Meta = connReady, db.NewWorker(d.main), db.NewWorker(d.meta)
	c.Addr, c.DB, c.startedPath = d.info.Addr, d.info.Database, d.info.Path
}

// Close closes the session's connections.
func (s *Session) Close() {
	for _, w := range s.workers() {
		w.Close()
	}
}

// workers is the session's connections connected, Main and Meta.
func (s *Session) workers() []*db.Worker {
	var ws []*db.Worker
	for _, c := range s.Conns {
		if c.State == connReady {
			ws = append(ws, c.Main, c.Meta)
		}
	}
	return ws
}

// openTable shows c's table t in openTarget's pane and focuses it (§7.8
// 「打开已有的表」, §12).
func (a *App) openTable(c *Conn, t db.Table) tea.Cmd { return a.openTableIn(a.openTarget(), c, t) }

// openTableIn shows c's table t in pane p and focuses it: p's tab of it, a
// pane having one at most (F3.37), else a new one. A landing tab current
// gives way to it, replaced, or closed for the tab there (§5「引导页」).
func (a *App) openTableIn(p *Pane, c *Conn, t db.Table) tea.Cmd {
	return a.openTableAs(p, c, t, nil, a.newTurned)
}

// openTableAs is openTableIn asking for r, when a new tab is made, in place
// of the first page as the table opens, and for it turned or not as turned
// says.
func (a *App) openTableAs(p *Pane, c *Conn, t db.Table, r *request, turned bool) tea.Cmd {
	a.showPane(p.ID)
	landing := p.tab().landing()
	if i := slices.IndexFunc(p.Tabs, func(tb Tab) bool { return tb.Data != nil && tb.Data.conn == c && idOf(tb.Data.table) == idOf(t) }); i >= 0 {
		if closed := p.Cur; landing {
			if a.closeTab(p, closed); closed < i {
				i--
			}
		}
		selectTab(p, i)
		return nil
	}
	// ponytail: named by the table alone; two connections' t_order tabs read
	// the same, the status bar telling the focused one's (§7.8). Add the
	// connection, t_order [seed], if that confuses.
	tab := Tab{Name: t.Name, Data: newDataTab(c, t)}
	tab.Data.transpose = turned
	if r == nil {
		first := tab.Data.request
		first.desc = a.newDesc
		tab.Data.setRequest(first)
	} else {
		tab.Data.setRequest(*r)
	}
	putTab(p, tab, landing)
	return a.fetch(tab.Data, true)
}

// showTab focuses pane p and switches to its tab i.
func (a *App) showTab(p *Pane, i int) {
	a.showPane(p.ID)
	selectTab(p, i)
}

// selectTab makes tab i pane p's current one; the one it was becomes the
// previous, the - (T-01). What was being typed in it goes, as when its pane
// loses focus.
func selectTab(p *Pane, i int) {
	if i == p.Cur || i < 0 || i >= len(p.Tabs) {
		return
	}
	if t := dataOf(p); t != nil {
		t.stopTyping()
	}
	p.Prev, p.Cur = p.Cur, i
}

// cycleTab is gt (d 1) and gT (d -1) on the focused pane, as in vim: the
// next or previous tab, around the ends; with a count gt goes to tab count
// and gT back count tabs.
func (a *App) cycleTab(d, count int) {
	p := a.focused()
	if n := len(p.Tabs); d > 0 && count > 0 {
		selectTab(p, count-1)
	} else if n > 0 {
		selectTab(p, ((p.Cur+d*max(count, 1))%n+n)%n)
	}
}

// newTab is tab.new, a tab bar's +: a landing tab after the last one,
// which it becomes, in the focused pane (§5「引导页」).
func (a *App) newTab() {
	p := a.openTarget()
	a.showPane(p.ID)
	putTab(p, landing(), false)
}

// newConsole is console.new (§5, §11): console_n, n the least no session
// has open, with what its file keeps, on conn, in its Schema, kept for the
// file, in place of a landing tab or in a new tab of openTarget's pane.
// ok is false when the file cannot be read: no console, a toast.
func (a *App) newConsole(conn *Conn) (cmd tea.Cmd, ok bool) {
	p, name := a.openTarget(), a.freeConsole()
	c, err := openConsole(conn, name)
	if err != nil {
		return a.showToast(err.Error(), toastTTL), false
	}
	a.showPane(p.ID)
	putTab(p, Tab{Name: name, Console: c}, p.tab().landing())
	return a.keepConsole(c), true
}

// restoreConsole puts console t, console_1 of the layout started with, on
// the connection and schema its file last ran on (§11): connected, now;
// not yet, once it is, t staying where it opened meanwhile. One gone from
// connections.toml leaves t as it is; a schema gone, gotCatalog's fallback
// takes it back. New consoles go on the current connection (newConsole).
func (a *App) restoreConsole(t *consoleTab) tea.Cmd {
	st, ok := a.state.Consoles[t.name]
	i := slices.IndexFunc(a.sess.Conns, func(c *Conn) bool { return c.Spec.Name == st.Conn })
	switch {
	case !ok || i < 0:
		return nil
	case a.sess.Conns[i].State == connReady:
		t.conn, t.schema = a.sess.Conns[i], cmp.Or(st.Schema, a.sess.Conns[i].Schema)
		return nil
	}
	t.want, t.wantSchema = a.sess.Conns[i], st.Schema
	return a.connect(t.want)
}

// pickConn is console t's connection dropdown taking c (§7.8「console
// 标题」): not while t's own is in a transaction, nor while t runs (a run
// may begin one); one not connected connects first, t going over once it
// is.
func (a *App) pickConn(t *consoleTab, c *Conn) tea.Cmd {
	t.want = nil
	why := t.stayReason()
	switch {
	case c == t.conn:
		return nil
	case why != "":
		return a.showToast(why, toastTTL)
	case c.State == connReady:
		return a.switchConn(t, c, c.Schema)
	}
	t.want, t.wantSchema = c, ""
	return a.connect(c)
}

// stayReason is why console t stays on its connection, "" for none: its
// own in a transaction, or t running, a run that may begin one (§7.8).
func (t *consoleTab) stayReason() string {
	switch {
	case t.running != nil:
		return i18n.T("正在执行，结束后再换连接")
	case t.conn.inTx():
		return i18n.T(txOpen)
	}
	return ""
}

// switchConn puts console t on connection c and schema: its next run runs
// there; its file keeps them (§11).
func (a *App) switchConn(t *consoleTab, c *Conn, schema string) tea.Cmd {
	t.conn, t.schema, t.comp, t.asked = c, schema, nil, nil
	return a.keepConsole(t)
}

// keepConsole writes where console t runs to the state, by its file
// (§11, §14).
func (a *App) keepConsole(t *consoleTab) tea.Cmd {
	if a.state.Consoles == nil {
		a.state.Consoles = map[string]config.ConsoleState{}
	}
	a.state.Consoles[t.name] = config.ConsoleState{Conn: t.conn.Spec.Name, Schema: t.schema}
	return a.saveState()
}

// consoleOpen is whether a console of any session but except is open on the
// file name is: two on one file would write over each other's autosaves
// (§11). The file systems of macOS and Windows tell names apart by more
// than case no better than that, so neither does this.
func (a *App) consoleOpen(name string, except *consoleTab) bool {
	return slices.ContainsFunc(a.consoles(), func(c *consoleTab) bool { return c != except && strings.EqualFold(c.name, name) })
}

// sessionOf is the session c is a connection of: an answer may be for one
// not attached, or one closed, nil then (F5.1).
func (a *App) sessionOf(c *Conn) *Session {
	for _, s := range a.sessions {
		if slices.Contains(s.Conns, c) {
			return s
		}
	}
	return nil
}

// consoles is every session's consoles: an answer may be for a session not
// attached (F5.1).
func (a *App) consoles() []*consoleTab {
	var out []*consoleTab
	for _, s := range a.sessions {
		out = append(out, s.consoles()...)
	}
	return out
}

// freeConsole is the first of console_1, console_2… no session has open
// (§11).
func (a *App) freeConsole() string {
	return freeConsoleName(func(name string) bool { return a.consoleOpen(name, nil) })
}

// freeConsoleName is the first of console_1, console_2… taken says is not.
func freeConsoleName(taken func(string) bool) string {
	for n := 1; ; n++ {
		if name := config.ConsoleName(n); !taken(name) {
			return name
		}
	}
}

// Close closes every session's connections, as the program ends.
func (a *App) Close() {
	for _, s := range a.sessions {
		s.Close()
	}
}

// panes is the session's panes but the sidebars, in every window.
func (s *Session) panes() []*Pane {
	var ps []*Pane
	for _, w := range s.Windows {
		if w.Root != nil {
			ps = append(ps, w.Root.Leaves()...)
		}
	}
	return ps
}

// consoles is the session's consoles, in every window.
func (s *Session) consoles() []*consoleTab { return consolesIn(s.panes()...) }

// consolesIn is the consoles of panes.
func consolesIn(panes ...*Pane) []*consoleTab {
	var out []*consoleTab
	for _, p := range panes {
		for _, t := range p.Tabs {
			if t.Console != nil {
				out = append(out, t.Console)
			}
		}
	}
	return out
}

// paneShowing is the pane of the window whose current tab is t; nil when
// t is not on screen.
func (a *App) paneShowing(t *dataTab) *Pane {
	for _, p := range a.win().Root.Leaves() {
		if dataOf(p) == t {
			return p
		}
	}
	return nil
}

// closeTab closes pane p's tab i: the current one (:q) gives way to the
// previous, as vim's alternate; another leaves the current and previous
// tabs as they were. Closing the last tab closes the pane too, except the
// sidebar and the window's only pane; the result area's log stays (§11).
func (a *App) closeTab(p *Pane, i int) {
	if p == a.win().Tree || i < 0 || i >= len(p.Tabs) || p.Tabs[i].Result != nil && p.Tabs[i].Result.run == nil {
		return
	}
	a.sess.recordClosed(a.win().ID, p.ID, p.Tabs[i])
	dropTab(p, i)
	if len(p.Tabs) == 0 {
		a.removePane(p.ID)
	}
}

// dropTab takes tab i out of p, the pane staying: when it was the current
// one, the one used before it gives way to take its place (vim's alternate),
// else the one that now stands where it did (T-01).
func dropTab(p *Pane, i int) {
	p.Tabs = slices.Delete(p.Tabs, i, i+1)
	switch {
	case i != p.Cur:
		if p.Cur > i {
			p.Cur--
		}
		if p.Prev == i {
			p.Prev = -1
		} else if p.Prev > i {
			p.Prev--
		}
	case p.Prev > i:
		p.Cur, p.Prev = p.Prev-1, -1
	case p.Prev >= 0 && p.Prev != i:
		p.Cur, p.Prev = p.Prev, -1
	default:
		p.Cur, p.Prev = max(min(i, len(p.Tabs)-1), 0), -1
	}
}

// closeTabAsking is x on pane p's tab i (F3.35): a console's is written
// first, asked about only when its connection's transaction would go with
// it (§10.5, §11); a table's with changes asks first.
func (a *App) closeTabAsking(p *Pane, i int) tea.Cmd {
	return a.closeTabThen(p, i, func() tea.Cmd { return nil })
}

// closeTabThen is closeTabAsking, then runs after once the tab is closed.
func (a *App) closeTabThen(p *Pane, i int, after func() tea.Cmd) tea.Cmd {
	if i < 0 || i >= len(p.Tabs) { // tab.close.at's i may come from a user's map
		return nil
	}
	if t := p.Tabs[i].Console; t != nil {
		return a.closeAsking(0, "", []*consoleTab{t}, func() tea.Cmd {
			if err := t.flush(); err != nil {
				return a.saveFailed(err)
			}
			a.closeTab(p, i)
			return after()
		})
	}
	n, name := 0, ""
	if t := p.Tabs[i].Data; t != nil {
		n, name = t.changes(), t.table.Name+" "
	}
	return a.unlessLost(n, name, nil, verbClose, func() tea.Cmd { a.closeTab(p, i); return after() })
}

// removePane takes pane id out of the tree: its sibling gets the space and
// the focus, and any zoom ends. The window's last pane besides the result
// area stays. The result area goes with its tabs, pinned or not, its
// height kept for the next time (§11).
func (a *App) removePane(id int) {
	win := a.win()
	if win.onlyPane(id) {
		return
	}
	if res := win.Result; res != nil && id == res.ID {
		for _, tb := range res.Tabs { // what a run going on took the place of goes too
			if r := tb.Result.run; r != nil {
				r.prev = nil
			}
		}
		win.resultRatio, win.Result = win.Root.Ratio, nil // it is the root's lower half
	}
	if root, heir := win.Root.remove(id); root != nil {
		win.Root, win.Zoom = root, 0
		win.focus(heir.ID)
	}
}

// onlyPane reports whether pane id is the window's one pane besides the
// result area, which stays (§5).
func (w *Window) onlyPane(id int) bool {
	n := len(w.Root.Leaves())
	if w.Result != nil && !w.resultHidden {
		if id == w.Result.ID {
			return false
		}
		n--
	}
	return n == 1
}

// panesByNumber is ⟨n⟩ order: the sidebar is 0, then the tree's leaves.
func (a *App) panesByNumber() []*Pane {
	return append([]*Pane{a.win().Tree}, a.win().Root.Leaves()...)
}

// focusSide moves focus to the neighbouring pane on side, from the rects the
// last frame was drawn with (tech-design §5). A folded sidebar takes no focus.
func (a *App) focusSide(side string) {
	win := a.win()
	rects := a.layout()
	if !win.TreeOpen {
		delete(rects, win.Tree.ID)
	}
	if id, ok := neighbor(rects, win.Focus, side, a.recent); ok {
		win.focus(id)
	}
}

// focusNext is pane.focus.next: the pane after the focused one going
// clockwise round the middle of the screen, the status bar in it, as each
// pane's middle lies from 12 o'clock; a tie is by ⟨n⟩, tmux's order; the
// last gives way to the first (§6.8, F5.16). Whole numbers, doubled so a
// middle is one, which keeps two on one ray equal.
func (a *App) focusNext() {
	win := a.win()
	rects := a.layout()
	if !win.TreeOpen {
		delete(rects, win.Tree.ID)
	}
	type vec struct{ x, y int } // from the middle, y up
	at := map[int]vec{}
	var ids []int
	for _, p := range a.panesByNumber() {
		if r, ok := rects[p.ID]; ok {
			at[p.ID] = vec{r.Min.X + r.Max.X - a.w, a.h - (r.Min.Y + r.Max.Y)}
			ids = append(ids, p.ID)
		}
	}
	half := func(v vec) int { // 0 from 12 o'clock to before 6, 1 from 6 to before 12
		switch {
		case v.x == 0 && v.y == 0: // on the middle: it has no direction, and is first
			return -1
		case v.x > 0 || v.x == 0 && v.y > 0:
			return 0
		}
		return 1
	}
	slices.SortStableFunc(ids, func(x, y int) int {
		u, v := at[x], at[y]
		if c := cmp.Compare(half(u), half(v)); c != 0 {
			return c
		}
		return cmp.Compare(u.x*v.y, u.y*v.x) // v clockwise of u: the cross product u × v is negative
	})
	if i := slices.Index(ids, win.Focus); i >= 0 {
		win.focus(ids[(i+1)%len(ids)])
	}
}

// recent reports whether pane x beats y as the one focused most recently:
// moving by direction (§5) and opening a table from the tree (§12) pick by
// it. Of two never focused, the one first in ⟨n⟩ order (up / left) wins.
func (a *App) recent(x, y int) bool {
	win := a.win()
	if win.focusedAt[x] != win.focusedAt[y] {
		return win.focusedAt[x] > win.focusedAt[y]
	}
	ps := a.panesByNumber()
	at := func(id int) int { return slices.IndexFunc(ps, func(p *Pane) bool { return p.ID == id }) }
	return at(x) < at(y)
}

// splitPane divides the focused pane along d; the new, empty pane gets focus.
func (a *App) splitPane(d Dir) {
	win := a.win()
	p := a.focused()
	if p == win.Tree || p == win.Result { // one result area a window, at the bottom (§11)
		return
	}
	win.lastID++ // never reused, so pane IDs stay stable (§5)
	np := &Pane{ID: win.lastID, Prev: -1}
	win.Root, win.Zoom = win.Root.split(p.ID, d, np), 0
	win.focus(np.ID)
}

// closePane closes the focused pane; its sibling takes the space. The
// sidebar and the window's only pane stay.
func (a *App) closePane() {
	win := a.win()
	if win.Focus != win.Tree.ID {
		a.removePane(win.Focus)
	}
}

// resizeStep is how far one SPC H/J/K/L moves a border.
const resizeStep = 0.05

func (a *App) resizePane(d Dir, sign float64, count int) {
	win := a.win()
	if win.Focus == win.Tree.ID {
		return
	}
	win.Root = win.Root.resize(win.Focus, d, sign*resizeStep*float64(max(count, 1)))
}

func (a *App) toggleZoom() {
	win := a.win()
	switch {
	case win.Zoom != 0:
		win.Zoom = 0
	case win.Focus != win.Tree.ID:
		win.Zoom = win.Focus
	}
}

// toggleTree is SPC b: the sidebar folded, or open with the focus on it,
// its cursor on the node of the tab focused before (§7.8, F3.29).
func (a *App) toggleTree() {
	win := a.win()
	switch win.TreeOpen = !win.TreeOpen; {
	case win.TreeOpen:
		a.revealTab(a.focused())
		win.focus(win.Tree.ID)
	case win.Focus == win.Tree.ID:
		win.focus(win.Root.Leaves()[0].ID)
	}
}

// jumpToPane is SPC q's second key: a digit picks ⟨n⟩, anything else just
// closes the numbers.
func (a *App) jumpToPane(k keymap.Key) {
	a.paneNumbers = false
	n, err := strconv.Atoi(string(k))
	ps := a.panesByNumber()
	if err != nil || n < 0 || n >= len(ps) || n == 0 && !a.win().TreeOpen {
		return
	}
	a.win().Zoom = 0
	a.win().focus(ps[n].ID)
}

// showPane focuses pane id where the palette sends the user (§12). A zoom on
// another pane ends first, as a jump by number ends it (§5): the pane is on
// screen once it has focus.
func (a *App) showPane(id int) {
	if a.win().Zoom != id {
		a.win().Zoom = 0
	}
	a.focusPane(id)
}

// focusPane gives focus to pane id if it is on screen (a click).
func (a *App) focusPane(id int) {
	if _, ok := a.layout()[id]; ok && (id != a.win().Tree.ID || a.win().TreeOpen) {
		a.win().focus(id)
	}
}

// wheelStep is how many rows one wheel notch scrolls (§7.4).
const wheelStep = 3

// scrollPane scrolls pane id by down notches and right notches (negative:
// up, left), within its content.
func (a *App) scrollPane(id, down, right int) {
	switch p := a.win().pane(id); {
	case p == a.win().Tree:
		a.scrollTree(down)
	case p != nil && resultOf(p) != nil && resultOf(p).run == nil: // the log
		a.logMove(p, func(r, c, _, _ int) (int, int) { return r + down*wheelStep, c })
	case p != nil && consoleOf(p) != nil: // not sideways: nowrap scrolls with the cursor (§11)
		c := consoleOf(p)
		a.consoleView(p, c)
		c.ed.Scroll(down * wheelStep)
		c.comp = nil
	case p != nil:
		a.scrollGrid(p, down*wheelStep, right)
	}
}

// closedTab is a table or a console tab that was closed, as it was (F5.16).
type closedTab struct {
	win, pane int // the window and pane it was in, by ID
	conn      *Conn
	table     db.Table // a table's, with the request it had
	req       request
	turned    bool   // a table's: it was turned
	console   string // the console's name, "" for a table
	schema    string
}

// closedKept is how many closed tabs tab.reopen goes back through.
const closedKept = 20

// recordClosed notes tab, closed in pane of window, for tab.reopen: a table
// or a console, no landing tab or result.
func (s *Session) recordClosed(win, pane int, tab Tab) {
	var c closedTab
	switch {
	case tab.Data != nil:
		d := tab.Data
		c = closedTab{conn: d.conn, table: d.table, req: request{applied: d.applied, order: d.order, desc: d.desc, limit: d.limit}, turned: d.transpose}
	case tab.Console != nil:
		c = closedTab{conn: tab.Console.conn, console: tab.Console.name, schema: tab.Console.schema}
	default:
		return
	}
	c.win, c.pane = win, pane
	s.closed = append(s.closed, c)
	if len(s.closed) > closedKept {
		s.closed = s.closed[1:]
	}
}

// reopenTab is tab.reopen: the tab closed last, in its pane when it is
// still there, else in the current one, a table with the WHERE, ORDER and
// LIMIT it had, a console on its file, connection and schema. One that is
// open already is only shown (F3.37, §11); no tab to reopen is no change.
func (a *App) reopenTab() tea.Cmd {
	s := a.sess
	if len(s.closed) == 0 {
		return nil
	}
	c := s.closed[len(s.closed)-1]
	s.closed = s.closed[:len(s.closed)-1]
	if c.console != "" {
		for _, s2 := range a.sessions {
			for wi, w := range s2.Windows {
				for _, p := range w.Root.Leaves() {
					if i := slices.IndexFunc(p.Tabs, func(tb Tab) bool { return tb.Console != nil && strings.EqualFold(tb.Console.name, c.console) }); i >= 0 {
						if s2 != s {
							a.attach(s2)
						}
						a.selectWindow(wi)
						a.showTab(p, i)
						return nil
					}
				}
			}
		}
	}
	p := a.openTarget()
	if wi := s.windowByID(c.win); wi >= 0 && s.Windows[wi].pane(c.pane) != nil {
		a.selectWindow(wi)
		p = s.Windows[wi].pane(c.pane)
	}
	if c.console == "" {
		return a.openTableAs(p, c.conn, c.table, &c.req, c.turned)
	}
	t, err := openConsole(c.conn, c.console)
	if err != nil {
		return a.showToast(err.Error(), toastTTL)
	}
	t.schema = c.schema
	a.showPane(p.ID)
	putTab(p, Tab{Name: c.console, Console: t}, p.tab().landing())
	return a.keepConsole(t)
}

// quitTab is tab.quit: the current tab closed as tab.close does, and when
// that leaves the window's only pane without a tab, the window too; an
// empty pane is closed as it is (F5.16). The session's last window stays.
func (a *App) quitTab() tea.Cmd {
	p, win := a.focused(), a.win()
	if p == win.Tree {
		return nil
	}
	sweep := func() tea.Cmd {
		if win != a.win() || win.pane(p.ID) != p || len(p.Tabs) > 0 {
			return nil
		}
		if win.onlyPane(p.ID) {
			return a.closeWindow()
		}
		a.removePane(p.ID)
		return nil
	}
	if len(p.Tabs) == 0 {
		return sweep()
	}
	return a.closeTabThen(p, p.Cur, sweep)
}
