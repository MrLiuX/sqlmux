package app

import (
	"context"

	tea "charm.land/bubbletea/v2"

	"sqlmux/internal/config"
	"sqlmux/internal/db"
	"sqlmux/internal/i18n"
)

// Send hands a message to the program from outside its Update: the
// Workers' redials happen in the requests' goroutines (§8.2「断线重连」).
// main sets it to the program's; a test sets what it likes.
var Send = func(tea.Msg) {}

// connEvent is a Worker's redial, as Worker.Notify tells it; Meta says
// which of the connection's two.
type connEvent struct {
	conn *Conn
	meta bool
	db.Event
}

// worker is a Worker over conn, dialed from spec, that dials again when the
// connection is lost; the password is asked for again as the first time
// (§8.2「断线重连」).
func (c *Conn) worker(conn db.Conn, spec config.Connection, readOnly, meta bool) *db.Worker {
	w := db.NewWorker(conn)
	w.Redial = func(ctx context.Context) (db.Conn, error) {
		secrets.Delete(spec.Name)
		c, _, err := connectTo(ctx, spec, readOnly)
		return c, err
	}
	w.Broken = engines[spec.Engine].Broken
	w.Notify = func(e db.Event) { Send(connEvent{c, meta, e}) }
	return w
}

// down is why c's last redial failed, Main's or Meta's; nil when both went
// through, or none was needed.
func (c *Conn) down() error {
	if c.downMain != nil {
		return c.downMain
	}
	return c.downMeta
}

// gotConnEvent takes in a redial. A failed one turns c's tree node red,
// with the reason in a toast, until a redial goes through. A good one
// says so only when the user has something to do: the request is not run
// again, or the transaction is gone, which Main's status goes back from
// (F8.19). The manual mode stays: leaving it would have the next write
// commit by itself.
func (a *App) gotConnEvent(m connEvent) tea.Cmd {
	c := m.conn
	if a.sessionOf(c) == nil { // its session closed meanwhile
		return nil
	}
	err := &c.downMain
	if m.meta {
		err = &c.downMeta
	}
	*err = m.Err
	name := c.Spec.Name
	switch {
	case m.Err != nil:
		return a.showToast(i18n.T("%s：%s", name, m.Err), toastTTL)
	case m.TxLost:
		c.tx = 'I'
		return tea.Batch(append(a.txEnded(c), a.showToast(i18n.T("%s：连接断开，事务已丢失", name), toastTTL))...)
	case !m.Rerun:
		return a.showToast(i18n.T("%s：已重连，请重新执行", name), toastTTL)
	}
	return nil
}
