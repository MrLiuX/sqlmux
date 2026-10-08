//go:build integration

package mysql

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"sqlmux/internal/db"
)

// redialing is a Worker over a connection of dsn that dials again, with
// the events it heard.
func redialing(t *testing.T, dsn string, readOnly bool) (w *db.Worker, events *[]db.Event) {
	t.Helper()
	return redialingTo(t, dsn, dsn, readOnly)
}

// redialingTo is redialing with a first connection to dsn, and the next to
// again: a proxy that is cut, then the database itself.
func redialingTo(t *testing.T, dsn, again string, readOnly bool) (w *db.Worker, events *[]db.Event) {
	t.Helper()
	c, _, err := Connect(context.Background(), dsn, "", readOnly)
	if err != nil {
		t.Fatal(err)
	}
	w, events = db.NewWorker(c), new([]db.Event)
	w.Redial = func(ctx context.Context) (db.Conn, error) {
		c, _, err := Connect(ctx, again, "", readOnly)
		return c, err
	}
	w.Broken = my.Broken
	w.Notify = func(e db.Event) { *events = append(*events, e) }
	t.Cleanup(func() { w.Close() })
	return w, events
}

// proxied is dsn's database behind a proxy of the test's own.
func proxied(t *testing.T, dsn string) (*db.Proxy, string) {
	t.Helper()
	cfg, err := Config(dsn, "")
	if err != nil {
		t.Fatal(err)
	}
	p := db.NewProxy(t, cfg.Addr)
	return p, strings.Replace(dsn, cfg.Addr, p.Addr(), 1)
}

// kill ends w's connection from another, as a proxy or wait_timeout would.
func kill(t *testing.T, w *db.Worker) {
	t.Helper()
	r, err := w.Query(context.Background(), "select connection_id()")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := connect(t, false).Query(context.Background(), "kill "+r.Rows[0][0].S); err != nil {
		t.Fatal(err)
	}
}

// What Meta does comes back by itself after the server drops it: the
// catalog, a page, quick SQL (§8.2「断线重连」).
func TestReconnectReads(t *testing.T) {
	ctx := context.Background()
	w, events := redialing(t, IntegrationDSN(t), true)
	kill(t, w)
	tables, err := my.Tables(ctx, w)
	if err != nil || len(tables) == 0 {
		t.Fatalf("catalog: %d tables, %v", len(tables), err)
	}
	if !slices.Equal(*events, []db.Event{{Rerun: true}}) {
		t.Fatalf("events %v", *events)
	}
	kill(t, w)
	if r, err := my.Quick(ctx, w, "select count(*) from agent", "agentable", 100); err != nil || r.Rows[0][0].S != "3" {
		t.Fatalf("quick: %v %v", r.Rows, err)
	}
	kill(t, w)
	var page db.Result
	err = w.RunSafe(ctx, func(ctx context.Context, c db.Conn) (err error) {
		page, _, err = db.Page(ctx, c, my, db.Query{Schema: "sqlmux", Table: "t_order", Key: []string{"id"}, Limit: 5})
		return err
	})
	if err != nil || len(page.Rows) != 5 {
		t.Fatalf("page: %d rows, %v", len(page.Rows), err)
	}
	if r, err := w.Query(ctx, "select @@session.transaction_read_only"); err != nil || r.Rows[0][0].S != "1" {
		t.Fatalf("the new connection is read only too: %v %v", r.Rows, err)
	}
}

// A write that the lost connection may have run is not run again, and the
// connection is back for the next request.
func TestReconnectWriteIsNotRunAgain(t *testing.T) {
	ctx := context.Background()
	w, events := redialing(t, OwnDB(t), false)
	if _, err := w.Exec(ctx, "create table t_re (n int)", 0); err != nil {
		t.Fatal(err)
	}
	kill(t, w)
	err := w.Run(ctx, func(ctx context.Context, c db.Conn) error {
		_, err := c.Query(ctx, "insert into t_re values (1)")
		return err
	})
	if err == nil || my.Broken(err) == db.NotLost {
		t.Fatalf("the error is the first: %v", err)
	}
	if !slices.Equal(*events, []db.Event{{}}) {
		t.Fatalf("events %v", *events)
	}
	if r, err := w.Query(ctx, "select count(*) from t_re"); err != nil || r.Rows[0][0].S != "0" {
		t.Fatalf("it ran again: %v %v", r.Rows, err)
	}
}

// A transaction does not outlive its connection (the engine's own
// bookkeeping of it: MySQL has no status to ask).
func TestReconnectTransactionLost(t *testing.T) {
	ctx := context.Background()
	w, events := redialing(t, OwnDB(t), false)
	if _, err := w.Exec(ctx, "create table t_re (n int) engine=innodb", 0); err != nil {
		t.Fatal(err)
	}
	err := w.Run(ctx, func(ctx context.Context, c db.Conn) error {
		for _, s := range []string{"begin", "insert into t_re values (1)"} {
			if _, err := c.Query(ctx, s); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	kill(t, w)
	if r, err := w.Query(ctx, "select count(*) from t_re"); err != nil || r.Rows[0][0].S != "0" {
		t.Fatalf("%v %v", r.Rows, err)
	}
	if !slices.Equal(*events, []db.Event{{TxLost: true, Rerun: true}}) {
		t.Fatalf("events %v", *events)
	}
}

// A transaction lost with the connection: a write in it is not run and says
// so; the new connection is in none (F8.19).
func TestReconnectTransactionLostWrite(t *testing.T) {
	ctx := context.Background()
	w, events := redialing(t, OwnDB(t), false)
	if _, err := w.Exec(ctx, "create table t_re (n int) engine=innodb", 0); err != nil {
		t.Fatal(err)
	}
	if err := w.Run(ctx, func(ctx context.Context, c db.Conn) error {
		for _, s := range []string{"begin", "insert into t_re values (1)"} {
			if _, err := c.Query(ctx, s); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	kill(t, w)
	var tx byte
	err := w.Run(ctx, func(ctx context.Context, c db.Conn) error {
		defer func() { tx = db.TxStatus(c) }()
		_, err := c.Query(ctx, "insert into t_re values (2)")
		return err
	})
	var lost db.TxLost
	if !errors.As(err, &lost) || !lost.Sent {
		t.Fatalf("the write: %v", err)
	}
	if tx != 'I' { // the connection's own account, what the app takes back
		t.Errorf("after the loss the status is %c", tx)
	}
	if !slices.Equal(*events, []db.Event{{TxLost: true}}) {
		t.Fatalf("events %v", *events)
	}
	if r, err := w.Query(ctx, "select count(*) from t_re"); err != nil || r.Rows[0][0].S != "0" {
		t.Fatalf("after: %v %v", r.Rows, err)
	}
}

// What a proxy cut between the statements of a run does: the statements
// done are done, the run is not run again (F8.19).
func TestReconnectBetweenStatements(t *testing.T) {
	for _, rst := range []bool{true, false} {
		t.Run(map[bool]string{true: "RST", false: "FIN"}[rst], func(t *testing.T) {
			ctx := context.Background()
			dsn := OwnDB(t)
			p, via := proxied(t, dsn)
			w, events := redialingTo(t, via, dsn, false)
			if _, err := w.Exec(ctx, "create table t_re (n int)", 0); err != nil {
				t.Fatal(err)
			}
			stmts := []string{"insert into t_re values (1)", "insert into t_re values (2)"}
			err := w.Run(ctx, func(ctx context.Context, c db.Conn) error {
				_, err := db.ExecEach(ctx, c, stmts, 0, func(s string) bool {
					if s == stmts[1] {
						p.Cut(rst)
						time.Sleep(100 * time.Millisecond)
					}
					return false
				})
				return err
			})
			if err == nil || my.Broken(err) == db.NotLost {
				t.Fatalf("the run: %v", err)
			}
			if len(*events) != 1 || (*events)[0].Rerun {
				t.Fatalf("events %v", *events)
			}
			if r, err := w.Query(ctx, "select group_concat(n) from t_re"); err != nil || r.Rows[0][0].S != "1" {
				t.Fatalf("rows: %v %v", r.Rows, err)
			}
		})
	}
}

// What an error is after a connection is closed, killed, cut, or timed out:
// the real ones (F8.19).
func TestBrokenSamples(t *testing.T) {
	ctx := context.Background()
	c := connect(t, false)
	gone, cancel := context.WithCancel(ctx)
	cancel()
	_, err := c.Query(gone, "select 1")
	if got := my.Broken(err); got != db.NotLost {
		t.Errorf("cancelled: %v is %d", err, got)
	}
	timedOut, cancel := context.WithTimeout(ctx, 50*time.Millisecond)
	defer cancel()
	_, err = c.Query(timedOut, "select sleep(2)")
	if got := my.Broken(err); got != db.NotLost {
		t.Errorf("timed out: %v is %d", err, got)
	}
	_, err = c.Query(ctx, "select 1/0 from nowhere")
	if got := my.Broken(err); got != db.NotLost {
		t.Errorf("SQL error: %v is %d", err, got)
	}
	// killed, idle: the first request finds it out, the next is the driver's bad connection
	k := connect(t, false)
	r, _ := k.Query(ctx, "select connection_id()")
	if _, err := c.Query(ctx, "kill "+r.Rows[0][0].S); err != nil {
		t.Fatal(err)
	}
	time.Sleep(200 * time.Millisecond)
	_, err = k.Query(ctx, "select 1")
	if got := my.Broken(err); got != db.LostSent {
		t.Errorf("killed: %v is %d", err, got)
	}
	_, err = k.Query(ctx, "select 1")
	if got := my.Broken(err); got != db.LostUnsent {
		t.Errorf("killed, again: %v is %d", err, got)
	}
	// the proxy cut idle
	for _, rst := range []bool{true, false} {
		p, via := proxied(t, IntegrationDSN(t))
		pc, _, err := Connect(ctx, via, "", false)
		if err != nil {
			t.Fatal(err)
		}
		p.Cut(rst)
		time.Sleep(100 * time.Millisecond)
		_, err = pc.Query(ctx, "select 1")
		if got := my.Broken(err); got == db.NotLost {
			t.Errorf("cut (RST %v): %v is %d", rst, err, got)
		}
		pc.Close()
	}
}

// What wait_timeout closes is found out by the first request after it, the
// driver reading its packets out of sync: lost, redialed, the read goes
// again (F8.19).
func TestReconnectWaitTimeout(t *testing.T) {
	ctx := context.Background()
	w, events := redialing(t, IntegrationDSN(t), true)
	if _, err := w.Query(ctx, "set session wait_timeout = 1"); err != nil {
		t.Fatal(err)
	}
	time.Sleep(2 * time.Second)
	if r, err := w.Query(ctx, "select 1"); err != nil || r.Rows[0][0].S != "1" || !slices.Equal(*events, []db.Event{{Rerun: true}}) {
		t.Fatalf("%v %v, events %v", r.Rows, err, *events)
	}
}

// A redial that fails leaves the connection closed, which database/sql says
// is no loss; the next request dials again all the same (F8.19).
func TestReconnectFailedRedialThenNext(t *testing.T) {
	ctx := context.Background()
	dsn := IntegrationDSN(t)
	w, _ := redialing(t, dsn, true)
	up := true
	redial := w.Redial
	w.Redial = func(ctx context.Context) (db.Conn, error) {
		if !up {
			return nil, errors.New("connection refused")
		}
		return redial(ctx)
	}
	kill(t, w)
	up = false
	if _, err := w.Query(ctx, "select 1"); err == nil {
		t.Fatal("the redial failed")
	}
	up = true
	if r, err := w.Query(ctx, "select 1"); err != nil || r.Rows[0][0].S != "1" {
		t.Fatalf("next: %v %v", r.Rows, err)
	}
}
