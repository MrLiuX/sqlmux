package app

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"

	"sqlmux/internal/config"
	"sqlmux/internal/keymap"
	"sqlmux/internal/ui"
)

// sessionList is the open session list (§7.8「session 列表」): the row
// selected, the sessions showing their windows, the name being typed.
type sessionList struct {
	sel, top int
	open     map[*Session]bool
	rename   *ui.Input // $ on the selected session's row
}

// sessionRow is a row of the list: session s's own, win -1, or one of its
// windows.
type sessionRow struct {
	s   *Session
	win int
}

func (a *App) sessionRows() []sessionRow {
	var rows []sessionRow
	for _, s := range a.sessions {
		rows = append(rows, sessionRow{s, -1})
		if a.sessList.open[s] {
			for wi := range s.Windows {
				rows = append(rows, sessionRow{s, wi})
			}
		}
	}
	return rows
}

// openSessions is SPC s, a click on the status bar's session block: the
// list, its selection on the session attached.
func (a *App) openSessions() {
	a.sessList = &sessionList{open: map[*Session]bool{}, sel: slices.Index(a.sessions, a.sess)}
}

func (a *App) sessionsMove(d int) {
	l := a.sessList
	n := len(a.sessionRows())
	_, rows := ui.SessionBox(a.window(), n)
	l.sel = max(min(l.sel+d, n-1), 0)
	l.top = max(min(l.top, l.sel), l.sel-rows+1, 0)
}

// sessionsFold is l (open) and h on the selection: a session shows or hides
// its windows; h on a window goes to its session's row, closed.
func (a *App) sessionsFold(open bool) {
	l := a.sessList
	r := a.sessionRows()[l.sel]
	l.open[r.s] = open
	l.sel = slices.Index(a.sessionRows(), sessionRow{r.s, -1})
	if open || r.win < 0 {
		l.sel = slices.Index(a.sessionRows(), r)
	}
	a.sessionsMove(0)
}

// sessionsAttach is ↵ on row i: its session attached, a window's switched
// to, the list closed.
func (a *App) sessionsAttach(i int) {
	if rows := a.sessionRows(); i < len(rows) {
		a.attach(rows[i].s)
		if rows[i].win >= 0 {
			a.sess.Active = rows[i].win
		}
	}
	a.sessList = nil
}

// attach makes s the session shown; the overlays go.
func (a *App) attach(s *Session) {
	a.sess = s
	a.whichKey, a.paneNumbers, a.palette, a.drop, a.cols, a.keyHelp = false, false, nil, nil, nil, nil
	a.res.Reset()
}

// sessionDialed answers newSession's dial: the new session's connection to
// the current one's spec, among specs, connected, or why not.
type sessionDialed struct {
	specs []config.Connection
	start int
	d     dialed
	err   error
}

// newSession is n (§7.8「新建 session」): a session of its own connections,
// once the one current here is connected for it, attached then; the list
// closes. One not connecting makes none, the session staying the one it
// is (F5.1).
func (a *App) newSession() tea.Cmd {
	a.sessList = nil
	var specs []config.Connection
	for _, c := range a.sess.Conns {
		specs = append(specs, c.Spec)
	}
	start := slices.Index(a.sess.Conns, a.conn())
	return tea.Batch(a.showToast("正在连接 "+specs[start].Name+"…", toastTTL), func() tea.Msg {
		d, err := dial(context.Background(), specs[start])
		return sessionDialed{specs, start, d, err}
	})
}

func (a *App) gotSessionDialed(m sessionDialed) tea.Cmd {
	if m.err != nil {
		return a.showToast(m.specs[m.start].Name+"："+m.err.Error(), toastTTL)
	}
	conns := make([]*Conn, len(m.specs))
	for i, spec := range m.specs {
		conns[i] = newConn(spec)
	}
	conns[m.start].take(m.d)
	n := 1 // tmux's: the least number no session is named
	for slices.ContainsFunc(a.sessions, func(s *Session) bool { return s.Name == strconv.Itoa(n) }) {
		n++
	}
	s := newSession(strconv.Itoa(n), conns, conns[m.start], a.freeConsole())
	a.sessions = append(a.sessions, s)
	a.attach(s)
	cmds := []tea.Cmd{a.loadCatalog(conns[m.start])}
	if s.warning != "" {
		cmds = append(cmds, a.showToast(s.warning, toastTTL))
	}
	for _, t := range s.consoles() {
		cmds = append(cmds, a.restoreConsole(t))
	}
	return tea.Batch(cmds...)
}

// killSession is x on a session's row (§7.8): asked first when changes or
// a transaction would go; its consoles written, its connections closed,
// which rolls a transaction back. The one attached gives way to the one
// before it, or after; the last one gone is quitting. The list stays.
func (a *App) killSession() tea.Cmd {
	r := a.sessionRows()[a.sessList.sel]
	if r.win >= 0 {
		return nil
	}
	if len(a.sessions) == 1 {
		return a.quit()
	}
	s, ps := r.s, r.s.panes()
	var text []string
	if n := unsaved(ps...); n > 0 {
		text = append(text, fmt.Sprintf("有 %d 处修改未保存，关闭会丢弃。", n))
	}
	if slices.ContainsFunc(s.Conns, func(c *Conn) bool { return c.inTx() }) {
		text = append(text, "有未提交的事务，关闭会回滚。")
	}
	kill := func() tea.Cmd {
		if cmd, ok := a.flushAll(ps...); !ok { // the consoles' text is not lost (§11)
			return cmd
		}
		i := slices.Index(a.sessions, s)
		a.sessions = slices.Delete(a.sessions, i, i+1)
		if a.sess == s {
			a.attach(a.sessions[max(i-1, 0)])
		}
		if l := a.sessList; l != nil {
			l.sel = min(l.sel, len(a.sessionRows())-1)
		}
		return func() tea.Msg { s.Close(); return nil }
	}
	if len(text) == 0 {
		return kill()
	}
	a.confirm = &confirmBox{strings.Join(text, ""), "关闭", kill}
	return nil
}

// renameSession is $ on a session's row: its name typed in its place.
func (a *App) renameSession() {
	if r := a.sessionRows()[a.sessList.sel]; r.win < 0 {
		a.sessList.rename = &ui.Input{Text: r.s.Name, Pos: len(r.s.Name)}
	}
}

// renameKey edits the name typed: ↵ takes it, but nothing typed or another
// session's name; esc drops it.
func (a *App) renameKey(k keymap.Key) tea.Cmd {
	l := a.sessList
	switch k {
	case keymap.Esc:
		l.rename = nil
	case "<CR>":
		name, s := strings.TrimSpace(l.rename.Text), a.sessionRows()[l.sel].s
		l.rename = nil
		switch {
		case name == "" || name == s.Name:
		case slices.ContainsFunc(a.sessions, func(o *Session) bool { return o.Name == name }):
			return a.showToast("已有同名 session", toastTTL)
		default:
			s.Name = name
		}
	default:
		editInput(l.rename, k)
	}
	return nil
}

func (a *App) sessionsView() ui.SessionList {
	l := a.sessList
	v := ui.SessionList{Sel: l.sel, Top: l.top}
	for i, r := range a.sessionRows() {
		row := ui.SessionRow{Name: r.s.Name, Note: fmt.Sprintf("%d windows", len(r.s.Windows)), Current: r.s == a.sess}
		if r.win >= 0 {
			row = ui.SessionRow{Window: true, Name: fmt.Sprintf("%d: %s", r.win, r.s.Windows[r.win].Name), Current: r.s == a.sess && r.win == r.s.Active}
		}
		if i == l.sel && l.rename != nil {
			row.Edit = l.rename
		}
		v.Rows = append(v.Rows, row)
	}
	return v
}
