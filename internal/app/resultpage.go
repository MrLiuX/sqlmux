package app

import (
	"context"
	"errors"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/kackerx/sqlmux/internal/i18n"
	"github.com/kackerx/sqlmux/internal/sqlkit"
)

// pageDone answers resultPage: what mainRun made of the one statement, for
// the page at offset of tab rt (§11「翻页」, F10.4).
type pageDone struct {
	rt     *resultTab
	offset int
	m      runDone
}

// canPage is whether the result tab can go d pages on, 1 or -1: its
// statement was given its LIMIT and has no OFFSET of its own (a tab that is
// pageable has its rows in, its run being done), nothing of it is on its
// way, the next page is only there when this one was cut at max_rows, the
// one before when this is not the first.
func (rt *resultTab) canPage(d int) bool {
	return rt.pageable && !rt.paging && (d > 0 && rt.page.Truncated || d < 0 && rt.offset > 0)
}

// resultPage is ] (d 1) and [ on the result tab shown: its statement asked
// for again on its console's connection with the OFFSET of the next page or
// the one before, which then takes the place of the rows in this tab only,
// the other tabs of its run and the pinned ones left as they are. The rows
// shown stay until the page is in. A page of a read with no ORDER BY may
// repeat or miss rows, as DataGrip's does (§11).
func (a *App) resultPage(d int) tea.Cmd {
	rt := resultOf(a.focused())
	if rt == nil || !rt.canPage(d) {
		return nil
	}
	r := rt.run
	if r.conn.State != connReady {
		return a.showToast(i18n.T("连接还没连上"), toastTTL)
	}
	offset := rt.offset + d*a.maxRows
	s := r.stmts[rt.stmt]
	text, _ := sqlkit.AutoLimit(r.sql[s.Start:s.End], r.conn.eng.Dialect(), a.maxRows+1, offset)
	rt.paging = true
	r.conn.busy++
	maxRows, manual := a.maxRows, r.conn.Manual
	return func() tea.Msg { return pageDone{rt, offset, mainRun(r, []string{text}, maxRows, manual, false)} }
}

// gotResultPage puts the page in its tab, a line of the log saying so; one
// that failed is logged as a run's is and the log shows, the tab as it was;
// one cancelled is logged and told in a toast, the tab as it was and shown
// still, as a rerun cancelled leaves it (F10.4). A tab closed or rerun
// meanwhile takes the page all the same, unseen.
func (a *App) gotResultPage(m pageDone) tea.Cmd {
	rt, r := m.rt, m.m.r
	if a.sessionOf(r.conn) == nil { // its session closed, its connection with it (F5.1)
		return nil
	}
	r.conn.busy--
	if m.m.tx != 0 {
		r.conn.tx = m.m.tx
	}
	rt.paging = false
	s := r.stmts[rt.stmt]
	stmt := r.sql[s.Start:s.End]
	if m.m.set != "" { // none ran: the SET is what failed, not the statement (§8.6)
		stmt = m.m.set
	}
	p := r.win.Result
	if m.m.err != nil {
		if errors.Is(m.m.err, context.Canceled) {
			a.log(r, stmt, i18n.T("已取消"), false)
			return a.showToast(i18n.T("查询已取消"), toastTTL)
		}
		a.logError(r, stmt, m.m.err)
		if p != nil {
			selectTab(p, 0)
		}
		return a.deadlocked(r.conn, m.m.err)
	}
	res := m.m.rs[0]
	a.log(r, stmt, resultText(res, m.offset), false)
	rt.gridState = gridState{page: res, transpose: rt.transpose}
	rt.offset, rt.arrived = m.offset, time.Now()
	return nil
}
