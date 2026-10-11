package db

import (
	"cmp"
	"context"
	"sync"
)

// Worker owns one connection and runs one request on it at a time (§8.2).
// Requests come from tea.Cmds, each on its own goroutine already, so a lock
// is all it takes; which result is current is the caller's seq to decide.
type Worker struct {
	conn *counted
	// Redial, Broken and Notify are the connection coming back (§8.2「断线重连」);
	// with none, a lost connection stays lost. Redial dials again, Broken
	// says whether a request's error is a lost connection, Notify hears of
	// each redial.
	Redial func(context.Context) (Conn, error)
	Broken func(error) Lost
	Notify func(Event)
	broken bool // the connection is gone and the last redial failed: the next request dials first
	closed bool // Close was called: no more redials, or the new connection would be no one's
	tx     byte // where the transaction on conn was as the last request ended

	run    sync.Mutex // held for the whole request
	mu     sync.Mutex // guards cancel
	cancel context.CancelFunc
}

// counted is a Worker's connection, counting the calls that went through in
// the request running: a request that failed on its first call, with nothing
// sent, may run again whatever it writes (§8.2「断线重连」). It is the same
// value for each request till a redial, which is what callers compare.
// tx is where the transaction was after the last call that went through, 0
// before one: a request that began one itself and lost the connection in it
// has lost a transaction, which the request before it did not leave.
type counted struct {
	Conn
	calls int
	tx    byte
}

func (c *counted) Exec(ctx context.Context, sql string, maxRows int) ([]Result, error) {
	r, err := c.Conn.Exec(ctx, sql, maxRows)
	if err == nil {
		c.went()
	}
	return r, err
}

func (c *counted) Query(ctx context.Context, sql string, args ...Val) (Result, error) {
	r, err := c.Conn.Query(ctx, sql, args...)
	if err == nil {
		c.went()
	}
	return r, err
}

// Stream is Each on the connection, counted as a call when it went through.
func (c *counted) Stream(ctx context.Context, sql string, head func([]Col) error, row func([]Val) error) error {
	err := Each(ctx, c.Conn, sql, head, row)
	if err == nil {
		c.went()
	}
	return err
}

// went notes a call that went through.
func (c *counted) went() {
	c.calls++
	c.tx = TxStatus(c.Conn)
}

// Setup runs f on c, a session's setting up that a request run again would
// repeat harmlessly, search_path say: what it does is not counted as done.
func Setup(c Conn, f func() error) error {
	cc, ok := c.(*counted)
	if !ok {
		return f()
	}
	was, tx := cc.calls, cc.tx
	defer func() { cc.calls, cc.tx = was, tx }()
	return f()
}

func (c *counted) TxStatus() byte { return TxStatus(c.Conn) }

func (c *counted) closed() bool {
	cl, ok := c.Conn.(closer)
	return ok && cl.Closed()
}

func NewWorker(c Conn) *Worker { return &Worker{conn: &counted{Conn: c}} }

// Exec and Query run again on a lost connection, redialed: they are what
// the catalog reads with.
func (w *Worker) Exec(ctx context.Context, sql string, maxRows int) (rs []Result, err error) {
	err = w.do(ctx, true, func(ctx context.Context, c Conn) (err error) { rs, err = c.Exec(ctx, sql, maxRows); return err })
	return rs, err
}

func (w *Worker) Query(ctx context.Context, sql string, args ...Val) (r Result, err error) {
	err = w.do(ctx, true, func(ctx context.Context, c Conn) (err error) { r, err = c.Query(ctx, sql, args...); return err })
	return r, err
}

// Run runs f on the connection as one request: nothing else runs on it in
// between, so f can keep a transaction open (§12 快速 SQL). A connection
// lost is redialed; f, which may write, runs again only when nothing of it
// was sent, and never when the connection's transaction is lost with it:
// that is TxLost (§8.2「断线重连」).
func (w *Worker) Run(ctx context.Context, f func(context.Context, Conn) error) error {
	return w.do(ctx, false, f)
}

// RunSafe is Run for an f that only reads: run again on the new connection
// whenever the connection was lost.
func (w *Worker) RunSafe(ctx context.Context, f func(context.Context, Conn) error) error {
	return w.do(ctx, true, f)
}

// ExecEach runs stmts on c one after the other, each an Exec of its own
// and so, autocommit, a transaction of its own; it stops at the first that
// fails (§11「执行」). begin, when not nil, says which go in a transaction
// begun first if c is in none, as psql's AUTOCOMMIT off does (the manual
// mode, F5.3). rs has one result for each statement that ran, the one that
// failed not among them. It goes in a Worker.Run, as one request.
func ExecEach(ctx context.Context, c Conn, stmts []string, maxRows int, begin func(stmt string) bool) (rs []Result, err error) {
	for _, s := range stmts {
		if begin != nil && TxStatus(c) == 'I' && begin(s) {
			if _, err := c.Exec(ctx, "begin", 0); err != nil {
				return rs, err
			}
		}
		r, err := c.Exec(ctx, s, maxRows)
		if err != nil {
			return rs, err
		}
		one := Result{} // one statement, one result
		if len(r) > 0 {
			one = r[0]
		}
		rs = append(rs, one)
	}
	return rs, nil
}

// Cancel cancels the request running now, if any; queued ones still run.
func (w *Worker) Cancel() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.cancel != nil {
		w.cancel()
	}
}

func (w *Worker) Close() error {
	w.Cancel()
	w.run.Lock()
	defer w.run.Unlock()
	w.closed = true
	return w.conn.Close()
}

// do runs f, redialing when the connection is lost: before f when it is
// known to be, after when f's error says so, once a request. Reads (safe)
// run again then; a request that may write runs again only if nothing of
// it was sent, and not at all once the connection's transaction is lost
// with it.
func (w *Worker) do(ctx context.Context, safe bool, f func(context.Context, Conn) error) error {
	w.run.Lock()
	defer w.run.Unlock()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	w.setCancel(cancel)
	defer w.setCancel(nil)
	redialed := false
	if w.redials() && (w.broken || w.conn.closed()) {
		txLost := w.inTx()
		if err := w.redial(ctx, txLost, safe || !txLost); err != nil {
			return err
		}
		redialed = true
		if txLost && !safe {
			return TxLost{}
		}
	}
	w.conn.calls, w.conn.tx = 0, 0
	err := f(ctx, w.conn)
	// A request stopped by Cancel or the caller's ctx fails with the ctx's
	// error, which says more than the server's reply to the cancel.
	if err != nil && ctx.Err() == nil && w.redials() {
		if lost := w.Broken(err); lost != NotLost {
			err = w.lost(ctx, safe, redialed, lost, err, f)
		}
	}
	if !w.broken && !w.conn.closed() { // a closed one says it is out of a transaction, which it is not known to be
		w.tx = TxStatus(w.conn)
	}
	if err != nil && ctx.Err() != nil {
		return ctx.Err()
	}
	return err
}

// lost is what a request that failed with err on a lost connection comes
// to: the connection dialed again unless this request did it already, and f
// run again if it may.
func (w *Worker) lost(ctx context.Context, safe, redialed bool, lost Lost, err error, f func(context.Context, Conn) error) error {
	if redialed { // once a request: the next one dials
		w.broken = true
		return err
	}
	// the transaction the connection was in as the request last knew it: after
	// its last call that went through, else as the request found it (F13.1). A
	// COMMIT of this request's that went through ends the transaction, one it
	// began that is lost is lost as the user's is; the app is told of it unless
	// the request is a read run again, which has lost nothing the user had
	had := w.inTx()
	tx := cmp.Or(w.conn.tx, w.tx)
	txLost, sent := tx == 'T' || tx == 'E', w.conn.calls > 0
	again := safe || !txLost && lost == LostUnsent && !sent
	if rerr := w.redial(ctx, txLost && (had || !safe), again); rerr != nil {
		return rerr
	}
	switch {
	case again:
		if err = f(ctx, w.conn); err != nil && ctx.Err() == nil && w.Broken(err) != NotLost {
			w.broken = true // lost again: the next request dials, this one is over
		}
	case txLost:
		return TxLost{Err: err, Sent: lost == LostSent}
	}
	return err
}

func (w *Worker) redials() bool { return w.Redial != nil && w.Broken != nil && !w.closed }

// inTx is whether conn had a transaction as the last request left it.
func (w *Worker) inTx() bool { return w.tx == 'T' || w.tx == 'E' }

// redial replaces the connection. txLost is whether a transaction went with
// the old one, rerun whether the request goes on to run. A cancel of ctx on
// the way is not the database's failure: no Notify, and the request's error
// is the dial's, which wraps the ctx's.
func (w *Worker) redial(ctx context.Context, txLost, rerun bool) error {
	go w.conn.Close() // a peer that is gone may hold Close for long
	c, err := w.Redial(ctx)
	if err != nil {
		w.broken = true
		if ctx.Err() == nil && w.Notify != nil {
			w.Notify(Event{Err: err})
		}
		return err
	}
	w.conn, w.broken, w.tx = &counted{Conn: c}, false, 'I'
	if w.Notify != nil {
		w.Notify(Event{TxLost: txLost, Rerun: rerun})
	}
	return nil
}

func (w *Worker) setCancel(c context.CancelFunc) {
	w.mu.Lock()
	w.cancel = c
	w.mu.Unlock()
}
