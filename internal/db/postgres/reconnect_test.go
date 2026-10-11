//go:build integration

package postgres

import (
	"context"
	"errors"
	"net/url"
	"slices"
	"testing"
	"time"

	"github.com/kackerx/sqlmux/internal/db"
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
	w.Broken = pg.Broken
	w.Notify = func(e db.Event) { *events = append(*events, e) }
	t.Cleanup(func() { w.Close() })
	return w, events
}

// proxied is dsn's database behind a proxy of the test's own.
func proxied(t *testing.T, dsn string) (*db.Proxy, string) {
	t.Helper()
	u, err := url.Parse(dsn)
	if err != nil {
		t.Fatal(err)
	}
	p := db.NewProxy(t, u.Host)
	u.Host = p.Addr()
	return p, u.String()
}

// kill terminates w's backend from another connection, as a proxy or a
// server dropping it would.
func kill(t *testing.T, w *db.Worker) {
	t.Helper()
	ctx := context.Background()
	r, err := w.Query(ctx, "select pg_backend_pid()")
	if err != nil {
		t.Fatal(err)
	}
	admin := connect(t, false)
	if _, err := admin.Query(ctx, "select pg_terminate_backend("+r.Rows[0][0].S+")"); err != nil {
		t.Fatal(err)
	}
}

// What Meta does comes back by itself after the server drops it: the
// catalog, a page, quick SQL (§8.2「断线重连」).
func TestReconnectReads(t *testing.T) {
	ctx := context.Background()
	w, events := redialing(t, IntegrationDSN(t), true)
	kill(t, w)
	tables, err := pg.Tables(ctx, w)
	if err != nil || len(tables) == 0 {
		t.Fatalf("catalog: %d tables, %v", len(tables), err)
	}
	if !slices.Equal(*events, []db.Event{{Rerun: true}}) {
		t.Fatalf("events %v", *events)
	}
	kill(t, w)
	if r, err := pg.Quick(ctx, w, "select count(*) from agent", "agentable", 100); err != nil || r.Rows[0][0].S != "3" {
		t.Fatalf("quick: %v %v", r.Rows, err)
	}
	kill(t, w)
	var page db.Result
	err = w.RunSafe(ctx, func(ctx context.Context, c db.Conn) (err error) {
		page, _, err = db.Page(ctx, c, pg, db.Query{Schema: "public", Table: "t_order", Key: []string{"id"}, Limit: 5})
		return err
	})
	if err != nil || len(page.Rows) != 5 {
		t.Fatalf("page: %d rows, %v", len(page.Rows), err)
	}
	if r, err := w.Query(ctx, "show default_transaction_read_only"); err != nil || r.Rows[0][0].S != "on" {
		t.Fatalf("the new connection is read only too: %v %v", r.Rows, err)
	}
}

// A write that the lost connection may have run is not run again; the error
// is the first, and the connection is back for the next request.
func TestReconnectWriteIsNotRunAgain(t *testing.T) {
	ctx := context.Background()
	dsn := OwnDB(t)
	w, events := redialing(t, dsn, false)
	if _, err := w.Exec(ctx, "create table t_re (n int)", 0); err != nil {
		t.Fatal(err)
	}
	kill(t, w)
	err := w.Run(ctx, func(ctx context.Context, c db.Conn) error {
		_, err := c.Query(ctx, "insert into t_re values (1)")
		return err
	})
	if sqlState(err) != "57P01" {
		t.Fatalf("the error is the first: %v", err)
	}
	if !slices.Equal(*events, []db.Event{{}}) {
		t.Fatalf("events %v", *events)
	}
	if r, err := w.Query(ctx, "select count(*) from t_re"); err != nil || r.Rows[0][0].S != "0" {
		t.Fatalf("it ran again: %v %v", r.Rows, err)
	}
}

// A transaction does not outlive its connection: the next request says so,
// and finds none of it.
func TestReconnectTransactionLost(t *testing.T) {
	ctx := context.Background()
	w, events := redialing(t, OwnDB(t), false)
	if _, err := w.Exec(ctx, "create table t_re (n int)", 0); err != nil {
		t.Fatal(err)
	}
	err := w.Run(ctx, func(ctx context.Context, c db.Conn) error {
		_, err := c.Exec(ctx, "begin; insert into t_re values (1)", 0)
		return err
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

// A transaction lost with the connection: a write in it is not run, on any
// way the loss shows, and says so; the new connection is in none (F8.19).
func TestReconnectTransactionLostWrite(t *testing.T) {
	ctx := context.Background()
	dsn := OwnDB(t)
	w, events := redialing(t, dsn, false)
	if _, err := w.Exec(ctx, "create table t_re (n int)", 0); err != nil {
		t.Fatal(err)
	}
	if err := w.Run(ctx, func(ctx context.Context, c db.Conn) error {
		_, err := c.Exec(ctx, "begin; insert into t_re values (1)", 0)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	kill(t, w)
	err := w.Run(ctx, func(ctx context.Context, c db.Conn) error {
		_, err := c.Query(ctx, "insert into t_re values (2)")
		return err
	})
	var lost db.TxLost
	if !errors.As(err, &lost) || !lost.Sent || sqlState(lost.Err) != "57P01" {
		t.Fatalf("the write: %v", err)
	}
	if !slices.Equal(*events, []db.Event{{TxLost: true}}) {
		t.Fatalf("events %v", *events)
	}
	if r, err := w.Query(ctx, "select count(*), current_setting('transaction_isolation') from t_re"); err != nil || r.Rows[0][0].S != "0" || w.Run(ctx, func(_ context.Context, c db.Conn) error {
		if c.(interface{ TxStatus() byte }).TxStatus() != 'I' {
			return errors.New("the new connection is in a transaction")
		}
		return nil
	}) != nil {
		t.Fatalf("after: %v %v", r.Rows, err)
	}
}

// What a proxy cut between the statements of a run does: the statements
// done are done, the run is not run again, whichever way tsh dies, with an
// RST or a FIN (F8.19).
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
			if err == nil || pg.Broken(err) == db.NotLost {
				t.Fatalf("the run: %v", err)
			}
			if len(*events) != 1 || (*events)[0].Rerun {
				t.Fatalf("events %v", *events)
			}
			if r, err := w.Query(ctx, "select string_agg(n::text, ',') from t_re"); err != nil || r.Rows[0][0].S != "1" {
				t.Fatalf("rows: %v %v", r.Rows, err)
			}
		})
	}
}

// What an error is after a proxy is cut, or a connection closed: the real
// ones, not those built by hand (F8.19).
func TestBrokenSamples(t *testing.T) {
	ctx := context.Background()
	closed := connect(t, false)
	closed.Close()
	_, err := closed.Query(ctx, "select 1")
	if got := pg.Broken(err); got != db.LostSent { // pgconn says the same of a connection a failed read closed: Worker asks IsClosed before a request
		t.Errorf("closed: %v is %d", err, got)
	}
	gone, cancel := context.WithCancel(ctx)
	cancel()
	_, err = connect(t, false).Query(gone, "select 1")
	if got := pg.Broken(err); got != db.NotLost {
		t.Errorf("cancelled: %v is %d", err, got)
	}
	timedOut, cancel := context.WithTimeout(ctx, 50*time.Millisecond)
	defer cancel()
	_, err = connect(t, false).Query(timedOut, "select pg_sleep(2)")
	if got := pg.Broken(err); got != db.NotLost {
		t.Errorf("timed out: %v is %d", err, got)
	}
	_, err = connect(t, false).Query(ctx, "select 1/0")
	if got := pg.Broken(err); got != db.NotLost {
		t.Errorf("SQL error: %v is %d", err, got)
	}
	// terminated, idle: the FATAL the server sent on its way
	c := connect(t, false)
	r, _ := c.Query(ctx, "select pg_backend_pid()")
	if _, err := connect(t, false).Query(ctx, "select pg_terminate_backend("+r.Rows[0][0].S+")"); err != nil {
		t.Fatal(err)
	}
	time.Sleep(200 * time.Millisecond)
	_, err = c.Query(ctx, "select 1")
	if got := pg.Broken(err); got != db.LostSent {
		t.Errorf("terminated: %v is %d", err, got)
	}
	// the proxy cut idle: nothing from the server, the write may have gone out or not
	for _, rst := range []bool{true, false} {
		p, via := proxied(t, IntegrationDSN(t))
		c, _, err := Connect(ctx, via, "", false)
		if err != nil {
			t.Fatal(err)
		}
		p.Cut(rst)
		time.Sleep(100 * time.Millisecond)
		_, err = c.Query(ctx, "select 1")
		if got := pg.Broken(err); got == db.NotLost {
			t.Errorf("cut (RST %v): %v is %d", rst, err, got)
		}
		c.Close()
	}
}

// The server closing an idle connection by idle_session_timeout, or a
// transaction's by idle_in_transaction_session_timeout: FATALs, read as
// lost; a transaction's loss is told (F8.19).
func TestReconnectServerTimeouts(t *testing.T) {
	ctx := context.Background()
	w, events := redialing(t, IntegrationDSN(t), true)
	if _, err := w.Exec(ctx, "set idle_session_timeout = '1s'", 0); err != nil {
		t.Fatal(err)
	}
	time.Sleep(1500 * time.Millisecond)
	if r, err := w.Query(ctx, "select 1"); err != nil || r.Rows[0][0].S != "1" || !slices.Equal(*events, []db.Event{{Rerun: true}}) {
		t.Fatalf("idle_session_timeout: %v %v, events %v", r.Rows, err, *events)
	}

	dsn := OwnDB(t)
	w, events = redialing(t, dsn, false)
	if _, err := w.Exec(ctx, "create table t_re (n int)", 0); err != nil {
		t.Fatal(err)
	}
	if err := w.Run(ctx, func(ctx context.Context, c db.Conn) error {
		_, err := c.Exec(ctx, "set idle_in_transaction_session_timeout = '1s'; begin; insert into t_re values (1)", 0)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	time.Sleep(1500 * time.Millisecond)
	if r, err := w.Query(ctx, "select count(*) from t_re"); err != nil || r.Rows[0][0].S != "0" || !slices.Equal(*events, []db.Event{{TxLost: true, Rerun: true}}) {
		t.Fatalf("idle_in_transaction_session_timeout: %v %v, events %v", r.Rows, err, *events)
	}
}

// A cancel the server does not act on in 5s drops the connection (§8.3); the
// next request dials first, and says the transaction went with it (F8.19).
func TestReconnectAfterCancelDropsTheConnection(t *testing.T) {
	ctx := context.Background()
	dsn := OwnDB(t)
	p, via := proxied(t, dsn)
	w, events := redialingTo(t, via, dsn, false)
	if err := w.Run(ctx, func(ctx context.Context, c db.Conn) error { _, err := c.Exec(ctx, "begin", 0); return err }); err != nil {
		t.Fatal(err)
	}
	p.Refuse(true) // the CancelRequest's own connection cannot get through
	cctx, cancel := context.WithTimeout(ctx, 300*time.Millisecond)
	defer cancel()
	if _, err := w.Query(cctx, "select pg_sleep(20)"); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("cancelled: %v", err)
	}
	if r, err := w.Query(ctx, "select 1"); err != nil || r.Rows[0][0].S != "1" || !slices.Equal(*events, []db.Event{{TxLost: true, Rerun: true}}) {
		t.Fatalf("next: %v %v, events %v", r.Rows, err, *events)
	}
}

// A write on a connection the proxy cut while it was idle: after an RST it
// found the connection gone before it sent anything, and runs again, once;
// after a FIN it had gone out, and is not run again (F8.19).
func TestReconnectIdleCutWrite(t *testing.T) {
	for _, rst := range []bool{true, false} {
		t.Run(map[bool]string{true: "RST", false: "FIN"}[rst], func(t *testing.T) {
			ctx := context.Background()
			dsn := OwnDB(t)
			p, via := proxied(t, dsn)
			w, events := redialingTo(t, via, dsn, false)
			if _, err := w.Exec(ctx, "create table t_re (n int)", 0); err != nil {
				t.Fatal(err)
			}
			p.Cut(rst)
			time.Sleep(100 * time.Millisecond)
			err := w.Run(ctx, func(ctx context.Context, c db.Conn) error {
				_, err := c.Query(ctx, "insert into t_re values (1)")
				return err
			})
			want := "0"
			if rst {
				want = "1"
			}
			if r, qerr := w.Query(ctx, "select count(*) from t_re"); qerr != nil || r.Rows[0][0].S != want || (err == nil) != rst {
				t.Fatalf("err %v, %v rows, want %s: %v", err, r.Rows, want, qerr)
			}
			if !slices.Equal(*events, []db.Event{{Rerun: rst}}) {
				t.Fatalf("events %v", *events)
			}
		})
	}
}

// A statement the server ran, its answer lost with the connection (an RST
// after it): pgconn closes the connection and reports "conn closed" from the
// next read, which is not "nothing was sent". The insert is not run again
// (F8.19, found in the review of F8.23).
func TestReconnectLostAnswerIsNotRunAgain(t *testing.T) {
	ctx := context.Background()
	dsn := OwnDB(t)
	own, _, err := Connect(ctx, dsn, "", false) // the test's own database, not through the proxy
	if err != nil {
		t.Fatal(err)
	}
	defer own.Close()
	if _, err := own.Exec(ctx, "create table t_la (n int)", 0); err != nil {
		t.Fatal(err)
	}
	p, via := proxied(t, dsn)
	w, events := redialingTo(t, via, dsn, false)
	if _, err := w.Exec(ctx, "select 1", 0); err != nil {
		t.Fatal(err)
	}
	p.HoldReplies(true)
	done := make(chan error, 1)
	go func() {
		done <- w.Run(ctx, func(ctx context.Context, c db.Conn) error {
			_, err := c.Exec(ctx, "insert into t_la values (1)", 0)
			return err
		})
	}()
	for range 100 { // the insert has run on the server
		if r, err := own.Query(ctx, "select count(*) from t_la"); err == nil && r.Rows[0][0].S == "1" {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	p.Cut(true)
	if err := <-done; err == nil {
		t.Fatal("no error")
	}
	if r, err := own.Query(ctx, "select count(*) from t_la"); err != nil || r.Rows[0][0].S != "1" {
		t.Fatalf("the insert ran again: %v %v", r.Rows, err)
	}
	if slices.ContainsFunc(*events, func(e db.Event) bool { return e.Rerun }) {
		t.Errorf("events %v", *events)
	}
}
