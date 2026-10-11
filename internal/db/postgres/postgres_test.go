//go:build integration

package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/kackerx/sqlmux/internal/db"
	"github.com/kackerx/sqlmux/internal/sqlkit"
)

func connect(t *testing.T, readOnly bool) *Conn {
	t.Helper()
	c, _ := connectInfo(t, readOnly)
	return c
}

// connectInfo is connect with where it is, as Connect says it.
func connectInfo(t *testing.T, readOnly bool) (*Conn, db.Info) {
	t.Helper()
	c, info, err := Connect(context.Background(), IntegrationDSN(t), "", readOnly)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.Close() })
	return c, info
}

func sqlState(err error) string {
	var pe *pgconn.PgError
	if errors.As(err, &pe) {
		return pe.Code
	}
	return ""
}

func TestConnectSetsStartupParams(t *testing.T) {
	c, info := connectInfo(t, false)
	r, err := c.Query(context.Background(),
		"select current_setting('application_name'), current_setting('DateStyle'), current_setting('default_transaction_read_only')")
	if err != nil {
		t.Fatal(err)
	}
	if got := r.Rows[0]; got[0].S != "sqlmux" || got[1].S != "ISO, YMD" || got[2].S != "off" {
		t.Fatalf("startup params %v", got)
	}
	if info.Path != `"$user", public` {
		t.Fatalf("search_path %q", info.Path)
	}
	if cfg, _ := pgconn.ParseConfig(IntegrationDSN(t)); info.Addr != fmt.Sprintf("%s@%s:%d", cfg.User, cfg.Host, cfg.Port) {
		t.Fatalf("addr %q", info.Addr)
	}
}

// Every value comes back as the server's text (§8.1): enums and json as
// written, timestamptz in ISO order, NULL apart from "".
func TestQueryReturnsText(t *testing.T) {
	c := connect(t, false)
	r, err := c.Query(context.Background(),
		"select status, paid, meta, raw, note, created_at, amount from t_order where id = $1", db.Val{S: "3"})
	if err != nil {
		t.Fatal(err)
	}
	want := []db.Val{
		{S: "failed"}, {Null: true}, {S: `{"n": 3, "tags": ["a", "b"]}`}, {S: `{"n" : 3}`},
		{Null: true}, {S: "2026-09-01 00:03:00+00"}, {S: "3.99"},
	}
	if len(r.Rows) != 1 {
		t.Fatalf("rows %v", r.Rows)
	}
	for i, v := range r.Rows[0] {
		if v != want[i] {
			t.Errorf("%s = %+v, want %+v", r.Cols[i].Name, v, want[i])
		}
	}
	types := []string{"", "bool", "jsonb", "json", "text", "timestamptz", "numeric"} // an enum has no built-in name
	for i, col := range r.Cols {
		if col.Type != types[i] {
			t.Errorf("%s type %q, want %q", col.Name, col.Type, types[i])
		}
	}
	if r.Tag != "SELECT 1" {
		t.Errorf("tag %q", r.Tag)
	}
	if _, err := c.Query(context.Background(), "select $1::int", db.Val{Null: true}); err != nil {
		t.Errorf("a NULL argument: %v", err)
	}
}

func TestExecRunsEveryStatement(t *testing.T) {
	c := connect(t, false)
	rs, err := c.Exec(context.Background(), "select 1 as a union all select 2; select 'x' as b", 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(rs) != 2 || len(rs[0].Rows) != 1 || !rs[0].Truncated || rs[1].Rows[0][0].S != "x" || rs[1].Truncated {
		t.Fatalf("results %+v", rs)
	}
	// Exec's simple protocol takes several statements; Query's extended one refuses them.
	if _, err := c.Query(context.Background(), "select 1; select 2"); sqlState(err) != "42601" {
		t.Fatalf("two statements over Query: %v", err)
	}
}

// Worker.Cancel cancels the statement on the server and keeps the
// connection (§8.3).
func TestCancelKeepsTheConnection(t *testing.T) {
	w := db.NewWorker(connect(t, false))
	time.AfterFunc(200*time.Millisecond, w.Cancel)
	start := time.Now()
	_, err := w.Query(context.Background(), "select pg_sleep(10)")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled query: %v", err)
	}
	if took := time.Since(start); took > 3*time.Second {
		t.Fatalf("cancel took %v", took)
	}
	r, err := w.Query(context.Background(), "select 1")
	if err != nil || r.Rows[0][0].S != "1" {
		t.Fatalf("after cancel: %v %v", r.Rows, err)
	}
}

func TestReadOnlyRefusesWrites(t *testing.T) {
	c := connect(t, true)
	_, err := c.Query(context.Background(), "select nextval('t_order_id_seq')")
	if sqlState(err) != "25006" || !strings.Contains(err.Error(), "read-only transaction") {
		t.Fatalf("nextval on a read-only connection: %v", err)
	}
}

// execEach is a query's run on w, as one request.
func execEach(ctx context.Context, w *db.Worker, stmts ...string) (rs []db.Result, err error) {
	err = w.Run(ctx, func(ctx context.Context, c db.Conn) error {
		rs, err = db.ExecEach(ctx, c, stmts, 0, nil)
		return err
	})
	return rs, err
}

// A query's schema goes first on search_path, the started one after it:
// agentable's tables need no prefix, public's still resolve. The started
// path goes as a parameter: "" (the server's search_path set empty) and
// $user,public (a DSN's) take no quoting (§8.6).
func TestSetSearchPath(t *testing.T) {
	c, info := connectInfo(t, false)
	ctx := context.Background()
	for _, started := range []string{info.Path, `""`, "$user,public", ""} {
		if err := pg.UseSchema(ctx, c, "agentable", started); err != nil {
			t.Fatalf("%s: %v", started, err)
		}
	}
	if err := pg.UseSchema(ctx, c, "agentable", info.Path); err != nil {
		t.Fatal(err)
	}
	r, err := c.Query(ctx, "select current_schemas(false)::text, (select count(*) from agent) >= 0, (select count(*) from t_order) >= 0")
	if err != nil || r.Rows[0][0].S != "{agentable,public}" {
		t.Fatalf("%v %v", r.Rows, err)
	}
	if info.Database != "sqlmux" || c.TxStatus() != 'I' {
		t.Errorf("database %q, tx %c", info.Database, c.TxStatus())
	}
}

// A query's run (§11「执行」): each statement commits on its own, and the
// first to fail stops the rest; a cancelled run leaves Main usable.
func TestExecEach(t *testing.T) {
	c, _, err := Connect(context.Background(), OwnDB(t), "", false)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	w := db.NewWorker(c)
	ctx := context.Background()
	if _, err := w.Exec(ctx, "create table t_run (n int)", 0); err != nil {
		t.Fatal(err)
	}
	rs, err := execEach(ctx, w, "insert into t_run values (1)", "insert into t_run values ('two')", "insert into t_run values (3)")
	if sqlState(err) != "22P02" || len(rs) != 1 || rs[0].Tag != "INSERT 0 1" {
		t.Fatalf("second fails: %v %+v", err, rs)
	}
	if r, err := w.Query(ctx, "select string_agg(n::text, ',') from t_run"); err != nil || r.Rows[0][0].S != "1" {
		t.Fatalf("the first committed, the third never ran: %v %v", r.Rows, err)
	}
	time.AfterFunc(200*time.Millisecond, w.Cancel)
	rs, err = execEach(ctx, w, "select 1", "select pg_sleep(10)", "insert into t_run values (4)")
	if !errors.Is(err, context.Canceled) || len(rs) != 1 {
		t.Fatalf("cancelled: %v %+v", err, rs)
	}
	if r, err := w.Query(ctx, "select count(*) from t_run"); err != nil || r.Rows[0][0].S != "1" {
		t.Fatalf("after the cancel: %v %v", r.Rows, err)
	}
}

// A connection the server drops in a transaction is in none: the server
// rolled it back, though pgconn's status stays 'T' (F5.11).
func TestTxStatusDropped(t *testing.T) {
	ctx := context.Background()
	c, other := connect(t, false), connect(t, false)
	if _, err := c.Exec(ctx, "begin", 0); err != nil || c.TxStatus() != 'T' {
		t.Fatalf("begin: %v %c", err, c.TxStatus())
	}
	pid, err := c.Query(ctx, "select pg_backend_pid()")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := other.Query(ctx, "select pg_terminate_backend($1::int)", pid.Rows[0][0]); err != nil {
		t.Fatal(err)
	}
	_, _ = c.Query(ctx, "select 1") // finds it gone
	if got := c.TxStatus(); got != 'I' {
		t.Errorf("dropped: %c", got)
	}
}

// What turns a read-only connection's read only off, by trying a write
// after it, and what doesn't, against what sqlkit.BreaksReadOnly keeps
// back (§13): all that does, and the SETs of §13's kinds that don't, kept
// back as a kind. A transaction's settings need one begun.
func TestReadOnlyUnlocks(t *testing.T) {
	ctx := context.Background()
	for _, c := range []struct {
		begin         bool
		stmt          string
		unlocks, kept bool
	}{
		{true, "set transaction read write", true, true},
		{true, "set transaction_read_only = off", true, true},
		{true, "set local transaction_read_only to off", true, true},
		{false, `set "default_transaction_read_only" to off`, true, true},
		{false, "SET SESSION default_transaction_read_only = off", true, true},
		{false, "set session characteristics as transaction read write", true, true},
		{false, "SET SESSION SESSION CHARACTERISTICS AS TRANSACTION READ WRITE", true, true},
		{false, "begin read write", true, true},
		{false, "start transaction isolation level serializable, read write", true, true},
		// kept back as their kind, harmless
		{true, "set transaction read only", false, true},
		{false, "set transaction_read_only = off", false, true}, // autocommit: its own transaction alone
		{false, "set default_transaction_read_only to default", false, true},
		{false, "set default_transaction_read_only = on", false, true},
		// let go
		{false, "set search_path to public", false, false},
		{false, "set statement_timeout = 0", false, false},
		{false, "set session authorization default", false, false},
		{false, "reset default_transaction_read_only", false, false},
		{false, "reset all", false, false},
		{false, "discard all", false, false},
		{false, "begin", false, false},
	} {
		conn := connect(t, true)
		if c.begin {
			_, _ = conn.Exec(ctx, "begin", 0)
		}
		if _, err := conn.Exec(ctx, c.stmt, 0); err != nil {
			t.Fatalf("%s: %v", c.stmt, err)
		}
		_, err := conn.Exec(ctx, "create temp table probe (a int)", 0)
		if unlocked := err == nil; unlocked != c.unlocks {
			t.Errorf("%s: unlocked %v: %v", c.stmt, unlocked, err)
		}
		if kept := sqlkit.BreaksReadOnly(c.stmt, sqlkit.PG); kept != c.kept {
			t.Errorf("%s: BreaksReadOnly %v", c.stmt, kept)
		}
	}
}

// What sqlkit.Literal writes is read back as it was, the nasty ones too
// (F8.7).
func TestLiteralRoundTrip(t *testing.T) {
	c, _, err := Connect(context.Background(), OwnDB(t), "", false)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	ctx := context.Background()
	if _, err := c.Query(ctx, "create table t_lit (s text)"); err != nil {
		t.Fatal(err)
	}
	for _, s := range []string{"done", "it's", `a\b'c\`, `\`, `\'`, "two\nlines", "中文 ' \"", "''"} {
		if _, err := c.Query(ctx, "insert into t_lit values ("+sqlkit.Literal(s, sqlkit.PG)+")"); err != nil {
			t.Fatalf("%q: %v", s, err)
		}
		if r, err := c.Query(ctx, "select s from t_lit order by ctid desc limit 1"); err != nil || r.Rows[0][0].S != s {
			t.Errorf("%q came back %v, %v", s, r.Rows, err)
		}
	}
}
