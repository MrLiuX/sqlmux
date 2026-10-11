package db

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// blockConn's Query waits for its ctx, counting how many run at once.
type blockConn struct{ running, most atomic.Int32 }

func (c *blockConn) Query(ctx context.Context, sql string, _ ...Val) (Result, error) {
	n := c.running.Add(1)
	defer c.running.Add(-1)
	if n > c.most.Load() {
		c.most.Store(n)
	}
	if sql == "wait" {
		<-ctx.Done()
		return Result{}, ctx.Err()
	}
	time.Sleep(time.Millisecond)
	return Result{Tag: sql}, nil
}

func (c *blockConn) Exec(context.Context, string, int) ([]Result, error) { return nil, nil }
func (c *blockConn) Close() error                                        { return nil }

func TestWorkerRunsOneRequestAtATime(t *testing.T) {
	c := &blockConn{}
	w := NewWorker(c)
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() { _, _ = w.Query(context.Background(), "x") })
	}
	wg.Wait()
	if c.most.Load() != 1 {
		t.Fatalf("%d requests ran at once", c.most.Load())
	}
}

func TestWorkerCancelsOnlyTheRunningRequest(t *testing.T) {
	w := NewWorker(&blockConn{})
	w.Cancel() // idle: nothing to cancel, and the next request isn't hit
	if r, err := w.Query(context.Background(), "x"); err != nil || r.Tag != "x" {
		t.Fatalf("after an idle cancel: %v %v", r, err)
	}
	done := make(chan error)
	go func() { _, err := w.Query(context.Background(), "wait"); done <- err }()
	for {
		if w.mu.Lock(); w.cancel != nil {
			w.mu.Unlock()
			break
		}
		w.mu.Unlock()
		time.Sleep(time.Millisecond)
	}
	w.Cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled request: %v", err)
	}
}

// lostConn is the nth connection a Worker is given. Its requests fail with
// dead once lives calls have gone through (dead nil: never), and once
// closed with errGone, as database/sql's do.
type lostConn struct {
	n      int
	dead   error
	lives  int
	failAt string // the statement that fails with dead, the connection being good for the rest
	tx     byte
	closed bool      // what Closed says, as PG's after a cancel took too long
	ran    *[]string // "<n>:<sql>", across connections

	mu     sync.Mutex
	gone   bool
	goneCh chan struct{}
}

var (
	errSent   = errors.New("broken pipe")
	errUnsent = errors.New("conn closed")
	errSQL    = errors.New("syntax error")
	errGone   = errors.New("sql: connection is already closed")
)

func newLostConn(n int, ran *[]string) *lostConn {
	return &lostConn{n: n, ran: ran, goneCh: make(chan struct{})}
}

func (c *lostConn) isDead() bool { return c.dead != nil && c.lives <= 0 }

func (c *lostConn) Query(_ context.Context, sql string, _ ...Val) (Result, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	switch {
	case c.gone:
		return Result{}, errGone
	case c.isDead(), sql == c.failAt && c.failAt != "":
		return Result{}, c.dead
	}
	if c.dead != nil {
		c.lives--
	}
	if sql == errSQL.Error() {
		return Result{}, errSQL
	}
	*c.ran = append(*c.ran, fmt.Sprintf("%d:%s", c.n, sql))
	return Result{Tag: sql}, nil
}

func (c *lostConn) Exec(ctx context.Context, sql string, _ int) ([]Result, error) {
	r, err := c.Query(ctx, sql)
	return []Result{r}, err
}

func (c *lostConn) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.gone {
		c.gone = true
		close(c.goneCh)
	}
	return nil
}

func (c *lostConn) Closed() bool { return c.closed }

// TxStatus is 'I' once the connection is gone, as PG's is: the server
// rolled it back.
func (c *lostConn) TxStatus() byte {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.gone || c.isDead() || c.closed {
		return 'I'
	}
	return c.tx
}

// lostWorker is a Worker over a first connection, dead with dead (nil: not),
// which gives a connection each redial; dials counts them, dialErr is the
// next one's failure, events what Notify heard.
type lostWorker struct {
	*Worker
	first   *lostConn
	ran     []string
	dials   int
	dialErr error
	events  []Event
}

func newLostWorker(dead error) *lostWorker {
	l := &lostWorker{}
	l.first = newLostConn(1, &l.ran)
	l.first.dead = dead
	l.Worker = NewWorker(l.first)
	l.Redial = func(context.Context) (Conn, error) {
		if err := l.dialErr; err != nil {
			l.dialErr = nil
			return nil, err
		}
		l.dials++
		return newLostConn(1+l.dials, &l.ran), nil
	}
	l.Broken = func(err error) Lost {
		switch {
		case errors.Is(err, errSent):
			return LostSent
		case errors.Is(err, errUnsent):
			return LostUnsent
		}
		return NotLost
	}
	l.Notify = func(e Event) { l.events = append(l.events, e) }
	return l
}

func query(sqls ...string) func(context.Context, Conn) error {
	return func(ctx context.Context, c Conn) error {
		for _, sql := range sqls {
			if _, err := c.Query(ctx, sql); err != nil {
				return err
			}
		}
		return nil
	}
}

func TestWorkerRedialsAndRunsAgain(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		name  string
		dead  error
		run   func(*Worker) error
		again bool // the request ran on the new connection
	}{
		{"query, sent", errSent, func(w *Worker) error { _, err := w.Query(ctx, "q"); return err }, true},
		{"exec, sent", errSent, func(w *Worker) error { _, err := w.Exec(ctx, "q", 0); return err }, true},
		{"run, sent", errSent, func(w *Worker) error { return w.Run(ctx, query("q")) }, false},
		{"run, unsent", errUnsent, func(w *Worker) error { return w.Run(ctx, query("q")) }, true},
		{"run safe, sent", errSent, func(w *Worker) error { return w.RunSafe(ctx, query("q")) }, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			l := newLostWorker(tc.dead)
			err := tc.run(l.Worker)
			if tc.again && (err != nil || !slices.Equal(l.ran, []string{"2:q"})) {
				t.Fatalf("ran %v, err %v", l.ran, err)
			}
			if !tc.again && (!errors.Is(err, tc.dead) || len(l.ran) != 0) {
				t.Fatalf("a write ran again, or the error changed: %v %v", l.ran, err)
			}
			if !slices.Equal(l.events, []Event{{Rerun: tc.again}}) {
				t.Fatalf("events %v", l.events)
			}
			if err := l.Run(ctx, query("next")); err != nil || l.dials != 1 {
				t.Fatalf("the new connection serves on: %v, %d dials", err, l.dials)
			}
		})
	}
}

// A request that may write runs again only if nothing of it went through:
// with a statement done before the one that failed unsent (autocommitted,
// in a query run), it would be done twice (F8.19).
func TestWorkerRunWithAStatementDoneIsNotRunAgain(t *testing.T) {
	l := newLostWorker(errUnsent)
	l.first.lives = 1
	err := l.Run(context.Background(), query("insert 1", "insert 2"))
	if !errors.Is(err, errUnsent) || !slices.Equal(l.ran, []string{"1:insert 1"}) {
		t.Fatalf("%v, ran %v", err, l.ran)
	}
	if !slices.Equal(l.events, []Event{{}}) {
		t.Fatalf("events %v", l.events)
	}
	// a read goes again, from its start
	l = newLostWorker(errUnsent)
	l.first.lives = 1
	if err := l.RunSafe(context.Background(), query("select 1", "select 2")); err != nil || !slices.Equal(l.ran, []string{"1:select 1", "2:select 1", "2:select 2"}) {
		t.Fatalf("%v, ran %v", err, l.ran)
	}
}

// The first call of a request that fails unsent, whatever it is, runs the
// request again: also on a connection that served requests before, whose
// calls are not this one's (F8.19).
func TestWorkerFirstCallUnsentRunsAgain(t *testing.T) {
	ctx := context.Background()
	exec := func(sql string) func(context.Context, Conn) error {
		return func(ctx context.Context, c Conn) error { _, err := c.Exec(ctx, sql, 0); return err }
	}
	l := newLostWorker(nil)
	if err := l.Run(ctx, exec("earlier")); err != nil {
		t.Fatal(err)
	}
	l.first.dead = errUnsent
	if err := l.Run(ctx, exec("update")); err != nil || !slices.Equal(l.ran, []string{"1:earlier", "2:update"}) {
		t.Fatalf("%v, ran %v, events %v", err, l.ran, l.events)
	}
}

func TestWorkerSqlErrorIsNoRedial(t *testing.T) {
	l := newLostWorker(nil)
	if _, err := l.Query(context.Background(), errSQL.Error()); !errors.Is(err, errSQL) || l.dials != 0 || len(l.events) != 0 {
		t.Fatalf("%v, %d dials, %v", err, l.dials, l.events)
	}
}

// A request dials at most once: after the dial before it, a loss is the
// request's error, and the next request dials again (F8.19).
func TestWorkerRedialsOncePerRequest(t *testing.T) {
	l := newLostWorker(nil)
	l.first.closed = true // known closed: dialed before the request
	l.Redial = func(context.Context) (Conn, error) {
		l.dials++
		c := newLostConn(1+l.dials, &l.ran)
		c.dead = errSent
		return c, nil
	}
	if err := l.Run(context.Background(), query("q")); !errors.Is(err, errSent) || l.dials != 1 {
		t.Fatalf("%v after %d dials", err, l.dials)
	}
	// the next goes to a connection dialed first, a write that no redial after its failure would run
	l.Redial = func(context.Context) (Conn, error) { l.dials++; return newLostConn(1+l.dials, &l.ran), nil }
	if err := l.Run(context.Background(), query("q2")); err != nil || l.dials != 2 || !slices.Equal(l.ran, []string{"3:q2"}) {
		t.Fatalf("%v, %d dials, ran %v", err, l.dials, l.ran)
	}
}

// The request run again that is lost again is over, the next dials; one
// that fails with a SQL error is not a loss.
func TestWorkerRunAgainLostAgain(t *testing.T) {
	ctx := context.Background()
	l := newLostWorker(errSent)
	l.Redial = func(context.Context) (Conn, error) {
		l.dials++
		c := newLostConn(1+l.dials, &l.ran)
		if l.dials == 1 {
			c.dead = errSent
		}
		return c, nil
	}
	if _, err := l.Query(ctx, "q"); !errors.Is(err, errSent) || l.dials != 1 {
		t.Fatalf("%v after %d dials", err, l.dials)
	}
	if _, err := l.Query(ctx, "q2"); err != nil || l.dials != 2 {
		t.Fatalf("the next dials first: %v, %d dials", err, l.dials)
	}
	l = newLostWorker(errSent)
	if _, err := l.Query(ctx, errSQL.Error()); !errors.Is(err, errSQL) || l.dials != 1 {
		t.Fatalf("a SQL error on the new connection: %v, %d dials", err, l.dials)
	}
	if _, err := l.Query(ctx, "q"); err != nil || l.dials != 1 {
		t.Fatalf("the new connection serves on: %v, %d dials", err, l.dials)
	}
}

func TestWorkerFailedRedialIsTriedAgainNextRequest(t *testing.T) {
	l := newLostWorker(errSent)
	l.dialErr = errors.New("connection refused")
	if _, err := l.Query(context.Background(), "q"); err == nil || err.Error() != "connection refused" {
		t.Fatalf("the redial's error is the request's: %v", err)
	}
	if len(l.events) != 1 || l.events[0].Err == nil {
		t.Fatalf("events %v", l.events)
	}
	// the lost connection is closed now, as database/sql's are: what it
	// says is no loss, and the next request dials all the same
	<-l.first.goneCh
	if _, err := l.Query(context.Background(), "q2"); err != nil || l.dials != 1 || !slices.Equal(l.ran, []string{"2:q2"}) {
		t.Fatalf("next request: %v, %d dials, ran %v", err, l.dials, l.ran)
	}
}

// The transaction is lost when the connection is, not when a redial
// succeeds: after one that failed, the next tells.
func TestWorkerFailedRedialKeepsTheTransaction(t *testing.T) {
	ctx := context.Background()
	l := newLostWorker(nil)
	l.first.tx = 'T'
	if err := l.Run(ctx, query("begin")); err != nil {
		t.Fatal(err)
	}
	l.first.dead, l.dialErr = errSent, errors.New("connection refused")
	if err := l.RunSafe(ctx, query("select")); err == nil {
		t.Fatal("the redial failed")
	}
	if err := l.RunSafe(ctx, query("select")); err != nil || !slices.Equal(l.events[1:], []Event{{TxLost: true, Rerun: true}}) {
		t.Fatalf("%v, events %v", err, l.events)
	}
}

func TestWorkerCancelIsNoRedial(t *testing.T) {
	l := newLostWorker(errSent)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := l.Query(ctx, "q"); !errors.Is(err, context.Canceled) || l.dials != 0 {
		t.Fatalf("%v, %d dials", err, l.dials)
	}
}

func TestWorkerCancelDuringRedialIsNoFailure(t *testing.T) {
	l := newLostWorker(errSent)
	ctx, cancel := context.WithCancel(context.Background())
	l.Redial = func(ctx context.Context) (Conn, error) { cancel(); return nil, ctx.Err() }
	if _, err := l.Query(ctx, "q"); !errors.Is(err, context.Canceled) || len(l.events) != 0 {
		t.Fatalf("%v, events %v", err, l.events)
	}
}

func TestWorkerRedialsBeforeRequestOnClosedConn(t *testing.T) {
	l := newLostWorker(nil)
	l.first.closed = true
	if err := l.Run(context.Background(), query("q")); err != nil || !slices.Equal(l.ran, []string{"2:q"}) {
		t.Fatalf("%v, ran %v", err, l.ran)
	}
	if !slices.Equal(l.events, []Event{{Rerun: true}}) {
		t.Fatalf("events %v", l.events)
	}
}

// No redial once the Worker is closed: the new connection would be no
// one's, a queued request finding the closed one gone (F8.19).
func TestWorkerClosedDoesNotRedial(t *testing.T) {
	l := newLostWorker(nil)
	l.first.closed = true
	if err := l.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := l.Query(context.Background(), "q"); err == nil || l.dials != 0 {
		t.Fatalf("%v, %d dials", err, l.dials)
	}
}

// What a lost transaction does to the request (F8.19): it says so, on every
// way the loss shows, and a request that may write is not run at all, or
// not again, a read as ever; a COMMIT can tell if it had gone out.
func TestWorkerTransactionLost(t *testing.T) {
	ctx := context.Background()
	for _, tx := range []byte{'T', 'E'} {
		for _, how := range []string{"known closed", "unsent", "sent"} {
			l := newLostWorker(nil)
			l.first.tx = tx
			if err := l.Run(ctx, query("begin")); err != nil { // the Worker learns it is in one
				t.Fatal(err)
			}
			switch how {
			case "known closed":
				l.first.closed = true
			case "unsent":
				l.first.dead = errUnsent
			case "sent":
				l.first.dead = errSent
			}
			err := l.Run(ctx, query("update"))
			var lost TxLost
			if !errors.As(err, &lost) || lost.Sent != (how == "sent") || (how == "known closed") != (lost.Err == nil) {
				t.Fatalf("%c %s: %v", tx, how, err)
			}
			if got := l.ran; !slices.Equal(got, []string{"1:begin"}) {
				t.Fatalf("%c %s: the write ran: %v", tx, how, got)
			}
			if !slices.Equal(l.events, []Event{{TxLost: true}}) {
				t.Fatalf("%c %s: events %v", tx, how, l.events)
			}
		}
	}
	// a read runs, as ever
	l := newLostWorker(nil)
	l.first.tx = 'T'
	_ = l.Run(ctx, query("begin"))
	l.first.dead = errSent
	if err := l.RunSafe(ctx, query("select")); err != nil || !slices.Equal(l.events, []Event{{TxLost: true, Rerun: true}}) {
		t.Fatalf("%v, events %v", err, l.events)
	}
}

// A request that begins a transaction itself and loses the connection in it
// has lost one, as one begun before it: the write is not run again, the
// transaction not there (F13.1).
func TestWorkerTransactionBegunInTheRequestIsLost(t *testing.T) {
	for _, dead := range []error{errSent, errUnsent} {
		l := newLostWorker(dead)
		l.first.tx, l.first.lives, l.first.failAt = 'T', 100, "update" // begin goes through, the update is the call that fails
		err := l.Run(context.Background(), query("begin", "update"))
		var lost TxLost
		if !errors.As(err, &lost) || lost.Sent != errors.Is(dead, errSent) || !slices.Equal(l.ran, []string{"1:begin"}) {
			t.Errorf("%v: %v, ran %v", dead, err, l.ran)
		}
	}
}

// A read that begins a transaction of its own, quick SQL's, and runs again on
// the new connection, has lost nothing the user had: no TxLost to tell the app
// (F13.1).
func TestWorkerReadsOwnTransactionIsNoLoss(t *testing.T) {
	l := newLostWorker(errSent)
	l.first.tx, l.first.lives, l.first.failAt = 'T', 100, "select"
	if err := l.RunSafe(context.Background(), query("begin read only", "select")); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(l.events, []Event{{Rerun: true}}) || !slices.Equal(l.ran, []string{"1:begin read only", "2:begin read only", "2:select"}) {
		t.Errorf("events %v, ran %v", l.events, l.ran)
	}
}

// The transaction is lost once: with the first connection a second loss in
// the same request or the next has none to lose; and a connection that says
// it is out of one because it is closed does not make the Worker forget it.
func TestWorkerSaysTheTransactionLostOnce(t *testing.T) {
	ctx := context.Background()
	l := newLostWorker(nil)
	l.first.tx = 'T'
	if err := l.Run(ctx, query("begin")); err != nil {
		t.Fatal(err)
	}
	l.first.dead = errSent
	l.Redial = func(context.Context) (Conn, error) {
		l.dials++
		c := newLostConn(1+l.dials, &l.ran)
		c.dead = errSent
		return c, nil
	}
	if _, err := l.Query(ctx, "q"); !errors.Is(err, errSent) {
		t.Fatal(err)
	}
	l.Redial = func(context.Context) (Conn, error) { l.dials++; return newLostConn(1+l.dials, &l.ran), nil }
	if err := l.Run(ctx, query("q")); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(l.events, []Event{{TxLost: true, Rerun: true}, {Rerun: true}}) {
		t.Fatalf("events %v", l.events)
	}
}

func TestWorkerClosedConnKeepsTheTransaction(t *testing.T) {
	ctx := context.Background()
	l := newLostWorker(nil)
	l.first.tx = 'T'
	if err := l.Run(ctx, query("begin")); err != nil {
		t.Fatal(err)
	}
	// a request that ends with the connection closed under it (a cancel that
	// took too long, §8.3): PG then says 'I'
	if err := l.RunSafe(ctx, func(ctx context.Context, c Conn) error {
		_, err := c.Query(ctx, "slow")
		l.first.closed = true
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if err := l.RunSafe(ctx, query("next")); err != nil || !slices.Equal(l.events, []Event{{TxLost: true, Rerun: true}}) {
		t.Fatalf("%v, events %v", err, l.events)
	}
}

func TestWorkerWithoutRedialStaysLost(t *testing.T) {
	l := newLostWorker(errSent)
	l.Redial = nil
	if _, err := l.Query(context.Background(), "q"); !errors.Is(err, errSent) {
		t.Fatal(err)
	}
}
