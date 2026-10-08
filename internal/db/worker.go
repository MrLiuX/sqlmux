package db

import (
	"context"
	"sync"
)

// Worker owns one connection and runs one request on it at a time (§8.2).
// Requests come from tea.Cmds, each on its own goroutine already, so a lock
// is all it takes; which result is current is the caller's seq to decide.
type Worker struct {
	conn Conn
	// Redial, Broken and Notify are the connection coming back (§8.2「断线重连」);
	// with none, a lost connection stays lost. Redial dials again as the
	// connection was dialed, Broken says whether a request's error is a lost
	// connection, Notify hears of each redial.
	Redial func(context.Context) (Conn, error)
	Broken func(error) Lost
	Notify func(Event)
	broken bool // the connection is gone and the last redial failed: the next request dials first
	tx     byte // where the transaction on conn was as the last request ended

	run    sync.Mutex // held for the whole request
	mu     sync.Mutex // guards cancel
	cancel context.CancelFunc
}

func NewWorker(c Conn) *Worker { return &Worker{conn: c} }

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
// lost with f's statements sent is redialed, and f not run again: it may
// have written; the caller's error is the original.
func (w *Worker) Run(ctx context.Context, f func(context.Context, Conn) error) error {
	return w.do(ctx, false, f)
}

// RunSafe is Run for an f that only reads: run again on the new connection
// when the connection was lost.
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
	return w.conn.Close()
}

// do runs f, redialing when the connection is lost: before f when it is
// known to be, after when f's error says so, once a request. f runs again
// then if again says it may: it was safe, or nothing had been sent.
func (w *Worker) do(ctx context.Context, safe bool, f func(context.Context, Conn) error) error {
	w.run.Lock()
	defer w.run.Unlock()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	w.setCancel(cancel)
	defer w.setCancel(nil)
	if c, ok := w.conn.(closer); w.Redial != nil && (w.broken || ok && c.Closed()) {
		if err := w.redial(ctx, true); err != nil {
			return err
		}
	}
	err := f(ctx, w.conn)
	// A request stopped by Cancel or the caller's ctx fails with the ctx's
	// error, which says more than the server's reply to the cancel.
	if err != nil && ctx.Err() == nil && w.Redial != nil && w.Broken != nil {
		if lost := w.Broken(err); lost != NotLost {
			again := safe || lost == LostUnsent
			if rerr := w.redial(ctx, again); rerr != nil {
				return rerr
			}
			if again {
				if err = f(ctx, w.conn); err != nil && ctx.Err() == nil && w.Broken(err) != NotLost {
					w.broken = true // lost again: the next request redials, this one is over
				}
			}
		}
	}
	if !w.broken {
		w.tx = TxStatus(w.conn)
	}
	if err != nil && ctx.Err() != nil {
		return ctx.Err()
	}
	return err
}

// redial replaces the connection, rerun saying whether the request goes on
// to run again. A cancel of ctx on the way is not the database's failure.
func (w *Worker) redial(ctx context.Context, rerun bool) error {
	txLost := w.tx == 'T' || w.tx == 'E'
	go w.conn.Close() // a peer that is gone may hold Close for long
	c, err := w.Redial(ctx)
	if err != nil {
		w.broken = true
		if ctx.Err() == nil && w.Notify != nil {
			w.Notify(Event{Err: err})
		}
		return err
	}
	w.conn, w.broken, w.tx = c, false, 'I'
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
