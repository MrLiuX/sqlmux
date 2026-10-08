//go:build integration

package postgres

import (
	"context"
	"slices"
	"testing"

	"sqlmux/internal/db"
)

// redialing is a Worker over a connection of dsn that dials again, with
// the events it heard.
func redialing(t *testing.T, dsn string, readOnly bool) (w *db.Worker, events *[]db.Event) {
	t.Helper()
	c, err := Connect(context.Background(), dsn, "", readOnly)
	if err != nil {
		t.Fatal(err)
	}
	w, events = db.NewWorker(c), new([]db.Event)
	w.Redial = func(ctx context.Context) (db.Conn, error) { return Connect(ctx, dsn, "", readOnly) }
	w.Broken = pg.Broken
	w.Notify = func(e db.Event) { *events = append(*events, e) }
	t.Cleanup(func() { w.Close() })
	return w, events
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
