package app

import (
	"context"
	"slices"

	tea "charm.land/bubbletea/v2"

	"sqlmux/internal/db"
)

// txOpen is why a connection's transaction keeps it as it is: a console on
// it, or the auto mode (§5).
const txOpen = "当前连接有未提交的事务，先提交或回滚"

// setTxMode puts c in the manual transaction mode, or back in the auto one
// but with a transaction going on, which nothing would end then (§5).
func (a *App) setTxMode(c *Conn, manual bool) tea.Cmd {
	if !manual && c.inTx() {
		return a.showToast(txOpen, toastTTL)
	}
	c.Manual = manual
	return nil
}

// txDone answers endTx.
type txDone struct {
	conn     *Conn
	sql, tag string
	tx       byte
	err      error
}

// endTx is tx.commit and tx.rollback, sql commit or rollback, on c's Main,
// out of a cancel's reach as a save's commit is (§10.3); nothing when c is
// in no transaction.
func (a *App) endTx(c *Conn, sql string) tea.Cmd {
	if !c.inTx() {
		return nil
	}
	main := c.Main
	return func() tea.Msg {
		m := txDone{conn: c, sql: sql}
		m.err = main.Run(context.Background(), func(ctx context.Context, cn db.Conn) error {
			defer func() { m.tx = db.TxStatus(cn) }()
			var err error
			m.tag, err = db.End(ctx, cn, sql)
			return err
		})
		return m
	}
}

// gotTx takes where the transaction is now, and c's tables load again: in
// the manual mode they showed what is rolled back now, and their counts,
// from Meta, see what is committed (§7.8). It ending fine says nothing,
// the status bar does; a commit of a failed one was a rollback.
func (a *App) gotTx(m txDone) tea.Cmd {
	m.conn.tx = m.tx
	var cmds []tea.Cmd
	for _, s := range a.sessions {
		for _, p := range s.panes() {
			for _, tab := range p.Tabs {
				if t := tab.Data; t != nil && t.conn == m.conn {
					cmds = append(cmds, a.fetch(t, true))
				}
			}
		}
	}
	switch {
	case m.err != nil:
		cmds = append(cmds, a.showToast(m.err.Error(), toastTTL))
	case m.sql == "commit" && m.tag == "ROLLBACK":
		cmds = append(cmds, a.showToast("事务已出错，提交改为了回滚", toastTTL))
	}
	return tea.Batch(cmds...)
}

// orphaned is the session's connections in a transaction that closing the
// consoles cs leaves with no console on them: closing asks first, and
// rolls them back (§10.5).
func (a *App) orphaned(cs []*consoleTab) []*Conn {
	rest := slices.DeleteFunc(a.sess.consoles(), func(t *consoleTab) bool { return slices.Contains(cs, t) })
	var out []*Conn
	for _, t := range cs {
		c := t.conn
		if c.inTx() && !slices.Contains(out, c) && !slices.ContainsFunc(rest, func(o *consoleTab) bool { return o.conn == c }) {
			out = append(out, c)
		}
	}
	return out
}

// closeAsking runs close, which closes the consoles cs and tabs holding n
// changes, of what (§10.5): asked first when they or the transactions
// orphaned would be lost, those rolled back once their consoles are
// gone indeed.
func (a *App) closeAsking(n int, what string, cs []*consoleTab, close func() tea.Cmd) tea.Cmd {
	lost := a.orphaned(cs)
	return a.unlessLost(n, what, lost, "关闭", func() tea.Cmd {
		cmds := []tea.Cmd{close()}
		for _, c := range lost {
			if !slices.ContainsFunc(a.sess.consoles(), func(t *consoleTab) bool { return t.conn == c }) {
				cmds = append(cmds, a.endTx(c, "rollback"))
			}
		}
		return tea.Batch(cmds...)
	})
}
