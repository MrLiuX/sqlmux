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

// lostConn is the nth connection a Worker is given: its requests fail with
// err (errSent or errUnsent) from the first on, once dead, or run.
type lostConn struct {
	n      int
	dead   error
	closed bool
	tx     byte
	ran    *[]string // "<n>:<sql>", across connections
}

var (
	errSent   = errors.New("broken pipe")
	errUnsent = errors.New("conn closed")
)

func (c *lostConn) Query(_ context.Context, sql string, _ ...Val) (Result, error) {
	if c.dead != nil {
		return Result{}, c.dead
	}
	*c.ran = append(*c.ran, fmt.Sprintf("%d:%s", c.n, sql))
	return Result{Tag: sql}, nil
}
func (c *lostConn) Exec(ctx context.Context, sql string, _ int) ([]Result, error) {
	r, err := c.Query(ctx, sql)
	return []Result{r}, err
}
func (c *lostConn) Close() error   { return nil }
func (c *lostConn) Closed() bool   { return c.closed }
func (c *lostConn) TxStatus() byte { return c.tx }

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
	l.first = &lostConn{n: 1, dead: dead, ran: &l.ran}
	l.Worker = NewWorker(l.first)
	l.Redial = func(context.Context) (Conn, error) {
		if err := l.dialErr; err != nil {
			l.dialErr = nil
			return nil, err
		}
		l.dials++
		return &lostConn{n: 1 + l.dials, ran: &l.ran}, nil
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

func query(sql string) func(context.Context, Conn) error {
	return func(ctx context.Context, c Conn) error { _, err := c.Query(ctx, sql); return err }
}

func TestWorkerRedialsAndRunsAgain(t *testing.T) {
	for _, tc := range []struct {
		name  string
		dead  error
		run   func(*Worker) error
		again bool // the request ran on the new connection
	}{
		{"query, sent", errSent, func(w *Worker) error { _, err := w.Query(context.Background(), "q"); return err }, true},
		{"exec, sent", errSent, func(w *Worker) error { _, err := w.Exec(context.Background(), "q", 0); return err }, true},
		{"run, sent", errSent, func(w *Worker) error { return w.Run(context.Background(), query("q")) }, false},
		{"run, unsent", errUnsent, func(w *Worker) error { return w.Run(context.Background(), query("q")) }, true},
		{"run safe, sent", errSent, func(w *Worker) error { return w.RunSafe(context.Background(), query("q")) }, true},
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
			if err := l.Run(context.Background(), query("next")); err != nil || l.dials != 1 {
				t.Fatalf("the new connection serves on: %v, %d dials", err, l.dials)
			}
		})
	}
}

func TestWorkerSqlErrorIsNoRedial(t *testing.T) {
	l := newLostWorker(errors.New("syntax error"))
	if _, err := l.Query(context.Background(), "q"); err == nil || l.dials != 0 || len(l.events) != 0 {
		t.Fatalf("%v, %d dials, %v", err, l.dials, l.events)
	}
}

func TestWorkerRedialsOncePerRequest(t *testing.T) {
	l := newLostWorker(errSent)
	l.Redial = func(context.Context) (Conn, error) {
		l.dials++
		return &lostConn{n: 1 + l.dials, dead: errSent, ran: &l.ran}, nil
	}
	if _, err := l.Query(context.Background(), "q"); !errors.Is(err, errSent) || l.dials != 1 {
		t.Fatalf("%v after %d dials", err, l.dials)
	}
	// that one is over; the next goes to a connection dialed first
	l.Redial = func(context.Context) (Conn, error) { l.dials++; return &lostConn{n: 1 + l.dials, ran: &l.ran}, nil }
	// a write, which a redial after its failure would not run again
	if err := l.Run(context.Background(), query("q2")); err != nil || l.dials != 2 || !slices.Equal(l.ran, []string{"3:q2"}) {
		t.Fatalf("%v, %d dials, ran %v", err, l.dials, l.ran)
	}
}

// The transaction was lost with the first connection: a second loss in the
// same request, or the next, has none to lose.
func TestWorkerSaysTheTransactionLostOnce(t *testing.T) {
	l := newLostWorker(nil)
	l.first.tx = 'T'
	if err := l.Run(context.Background(), query("update")); err != nil {
		t.Fatal(err)
	}
	l.first.dead = errSent
	l.Redial = func(context.Context) (Conn, error) {
		l.dials++
		return &lostConn{n: 1 + l.dials, dead: errSent, ran: &l.ran}, nil
	}
	if _, err := l.Query(context.Background(), "q"); !errors.Is(err, errSent) {
		t.Fatal(err)
	}
	l.Redial = func(context.Context) (Conn, error) { l.dials++; return &lostConn{n: 1 + l.dials, ran: &l.ran}, nil }
	if err := l.Run(context.Background(), query("q")); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(l.events, []Event{{TxLost: true, Rerun: true}, {Rerun: true}}) {
		t.Fatalf("events %v", l.events)
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

func TestWorkerFailedRedialIsTriedAgainNextRequest(t *testing.T) {
	l := newLostWorker(errSent)
	l.dialErr = errors.New("connection refused")
	if _, err := l.Query(context.Background(), "q"); err == nil || err.Error() != "connection refused" {
		t.Fatalf("the redial's error is the request's: %v", err)
	}
	if len(l.events) != 1 || l.events[0].Err == nil {
		t.Fatalf("events %v", l.events)
	}
	if _, err := l.Query(context.Background(), "q2"); err != nil || l.dials != 1 || !slices.Equal(l.ran, []string{"2:q2"}) {
		t.Fatalf("next request: %v, %d dials, ran %v", err, l.dials, l.ran)
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

func TestWorkerSaysTheTransactionLost(t *testing.T) {
	l := newLostWorker(nil)
	l.first.tx = 'T'
	if err := l.Run(context.Background(), query("update")); err != nil { // ends in a transaction
		t.Fatal(err)
	}
	l.first.dead = errSent
	if err := l.Run(context.Background(), query("update")); !errors.Is(err, errSent) {
		t.Fatal(err)
	}
	if !slices.Equal(l.events, []Event{{TxLost: true}}) {
		t.Fatalf("events %v", l.events)
	}
}

func TestWorkerWithoutRedialStaysLost(t *testing.T) {
	c := &lostConn{n: 1, dead: errSent, ran: new([]string)}
	if _, err := NewWorker(c).Query(context.Background(), "q"); !errors.Is(err, errSent) {
		t.Fatal(err)
	}
}
