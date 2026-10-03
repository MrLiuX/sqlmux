//go:build integration

package mysql

import (
	"context"
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/go-sql-driver/mysql"

	"sqlmux/internal/db"
)

// my is the engine the tests go through.
var my Engine

func connect(t *testing.T, readOnly bool) *Conn {
	t.Helper()
	c, _, err := Connect(context.Background(), IntegrationDSN(t), "", readOnly)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.Close() })
	return c
}

// ownDB is a database of the test's own on the test server, seeded as the
// shared one is but for agentable, and dropped when the test ends: for
// tests that write (AGENTS.md「集成测试环境」). It returns its DSN.
func ownDB(t *testing.T) string {
	t.Helper()
	ctx := context.Background()
	admin := connect(t, false)
	name := fmt.Sprintf("sqlmux_worker_%d_%s", os.Getpid(), strings.ToLower(strings.ReplaceAll(t.Name(), "/", "_")))
	drop := func() { admin.Query(ctx, "drop database if exists "+name) }
	drop()
	if _, err := admin.Query(ctx, "create database "+name); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(drop)
	cfg, err := Config(IntegrationDSN(t), "")
	if err != nil {
		t.Fatal(err)
	}
	cfg.DBName = name
	seed, err := os.ReadFile("../../../testdata/seed/mysql.sql")
	if err != nil {
		t.Fatal(err)
	}
	c, _, err := Connect(ctx, cfg.FormatDSN(), "", false)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	for _, s := range strings.Split(string(seed), ";\n") {
		if strings.TrimSpace(s) == "" || strings.Contains(s, "agentable") || strings.Contains(s, "grant ") {
			continue
		}
		if _, err := c.Exec(ctx, s, 0); err != nil {
			t.Fatalf("%v\n%s", err, s)
		}
	}
	return cfg.FormatDSN()
}

func errno(err error) uint16 {
	var me *mysql.MySQLError
	if errors.As(err, &me) {
		return me.Number
	}
	return 0
}

// A URL and the driver's own DSN connect alike; the address is user@host:port.
func TestConnect(t *testing.T) {
	cfg, err := Config(IntegrationDSN(t), "")
	if err != nil {
		t.Fatal(err)
	}
	url := fmt.Sprintf("mysql://%s:%s@%s/%s", cfg.User, cfg.Passwd, cfg.Addr, cfg.DBName)
	for _, dsn := range []string{IntegrationDSN(t), url} {
		c, info, err := Connect(context.Background(), dsn, "", false)
		if err != nil {
			t.Fatalf("%s: %v", dsn, err)
		}
		if info.Addr != cfg.User+"@"+cfg.Addr || info.Database != "" || c.id == "" {
			t.Errorf("%s: %+v, id %q", dsn, info, c.id)
		}
		c.Close()
	}
	if u, _ := Config("mysql://u:p@db.example/x?tls=skip-verify", "pw"); u.Addr != "db.example:3306" || u.Passwd != "pw" || u.DBName != "x" || u.TLSConfig != "skip-verify" {
		t.Errorf("a URL: %+v", u)
	}
}

// Every value comes back as the server's text (§8.1): enums and json as
// written, datetime in ISO order, NULL apart from ""; an UPDATE counts the
// rows it found, one setting what is there too.
func TestQueryReturnsText(t *testing.T) {
	c := connect(t, false)
	r, err := c.Query(context.Background(), "select status, paid, meta, raw, note, created_at, amount, '' from t_order where id = ?", db.Val{S: "3"})
	if err != nil {
		t.Fatal(err)
	}
	want := []db.Val{
		{S: "failed"}, {Null: true}, {S: `{"n": 3, "tags": ["a", "b"]}`}, {S: `{"n": 3}`},
		{Null: true}, {S: "2026-09-01 00:03:00"}, {S: "3.99"}, {S: ""},
	}
	if len(r.Rows) != 1 || r.Tag != "SELECT 1" {
		t.Fatalf("rows %v, tag %q", r.Rows, r.Tag)
	}
	for i, v := range r.Rows[0] {
		if v != want[i] {
			t.Errorf("%s = %+v, want %+v", r.Cols[i].Name, v, want[i])
		}
	}
	types := []string{"enum", "tinyint", "json", "json", "text", "datetime", "decimal"}
	for i, typ := range types {
		if r.Cols[i].Type != typ {
			t.Errorf("%s type %q, want %q", r.Cols[i].Name, r.Cols[i].Type, typ)
		}
	}
	if _, err := c.Query(context.Background(), "select ?", db.Val{Null: true}); err != nil {
		t.Errorf("a NULL argument: %v", err)
	}

	own, _, err := Connect(context.Background(), ownDB(t), "", false)
	if err != nil {
		t.Fatal(err)
	}
	defer own.Close()
	if r, err := own.Query(context.Background(), "update t_user set name = name where id = ?", db.Val{S: "1"}); err != nil || r.Affected != 1 || r.Tag != "UPDATE 1" {
		t.Errorf("found, not changed: %+v %v", r, err)
	}
}

// One statement at a time; more over one call fail.
func TestExecOneStatement(t *testing.T) {
	c := connect(t, false)
	rs, err := c.Exec(context.Background(), "select 1 as a union all select 2", 1)
	if err != nil || len(rs) != 1 || len(rs[0].Rows) != 1 || !rs[0].Truncated {
		t.Fatalf("results %+v %v", rs, err)
	}
	if _, err := c.Exec(context.Background(), "select 1; select 2", 0); errno(err) != 1064 {
		t.Fatalf("two statements: %v", err)
	}
}

// Worker.Cancel kills the statement on the server, over a connection of
// its own, and keeps this one (§8.3).
func TestCancelKills(t *testing.T) {
	c := connect(t, false)
	w := db.NewWorker(c)
	time.AfterFunc(200*time.Millisecond, w.Cancel)
	start := time.Now()
	_, err := w.Query(context.Background(), "select sleep(10)")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled query: %v", err)
	}
	if took := time.Since(start); took > 3*time.Second {
		t.Fatalf("cancel took %v", took)
	}
	r, err := w.Query(context.Background(), "select connection_id()")
	if err != nil || r.Rows[0][0].S != c.id {
		t.Fatalf("after cancel: %v %v", r.Rows, err)
	}
}

// Meta's transactions are read only (§8.2); TxStatus goes by the
// statements that begin and end one.
func TestReadOnlyAndTx(t *testing.T) {
	c := connect(t, true)
	ctx := context.Background()
	if _, err := c.Query(ctx, "update t_user set name = name where id = 1"); errno(err) != 1792 {
		t.Fatalf("a write on a read-only connection: %v", err)
	}
	for _, s := range []struct {
		sql string
		tx  byte
	}{
		{"begin", 'T'}, {"savepoint s", 'T'}, {"rollback to savepoint s", 'T'}, {"commit", 'I'},
		{"start transaction read only", 'T'}, {"rollback", 'I'}, {"select 1", 'I'},
	} {
		if _, err := c.Query(ctx, s.sql); err != nil || c.TxStatus() != s.tx {
			t.Errorf("%s: %c %v", s.sql, c.TxStatus(), err)
		}
	}
}

// USE makes the database the current one; errors keep SQLSTATE and the
// message (§11).
func TestUseSchemaAndErrors(t *testing.T) {
	c := connect(t, false)
	ctx := context.Background()
	if err := my.UseSchema(ctx, c, "agentable", ""); err != nil {
		t.Fatal(err)
	}
	if r, err := c.Query(ctx, "select count(*) from agent"); err != nil || r.Rows[0][0].S != "3" {
		t.Fatalf("agentable's agent: %v %v", r.Rows, err)
	}
	_, err := c.Query(ctx, "select * from no_such")
	if e := my.ErrorOf(err); e.Code != "42S02" || e.Message != "Table 'agentable.no_such' doesn't exist" || e.Severity != "ERROR" {
		t.Errorf("%+v", e)
	}
	if e := my.ErrorOf(errors.New("gone")); e.Code != "" || e.Message != "gone" {
		t.Errorf("no server's: %+v", e)
	}
}

func TestSchemas(t *testing.T) {
	schemas, current, err := my.Schemas(context.Background(), connect(t, true))
	if err != nil || current != "sqlmux" || !slices.Contains(schemas, "agentable") || !slices.Contains(schemas, "sqlmux") || slices.Contains(schemas, "mysql") {
		t.Fatalf("%v %q %v", schemas, current, err)
	}
}

// Views and tables, a partitioned one once; a view has no row count.
func TestTables(t *testing.T) {
	ts, err := my.Tables(context.Background(), connect(t, true))
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]db.Table{}
	for _, tb := range ts {
		got[tb.Schema+"."+tb.Name] = tb
	}
	if v := got["sqlmux.v_paid_order"]; !v.View() || v.Rows != -1 {
		t.Errorf("view: %+v", v)
	}
	if e := got["sqlmux.t_event"]; e.Kind != "r" || e.View() {
		t.Errorf("partitioned: %+v", e)
	}
	if o := got["sqlmux.t_order"]; o.Rows < 5000 {
		t.Errorf("t_order rows: %+v", o)
	}
	if _, ok := got["agentable.agent"]; !ok {
		t.Errorf("agentable: %v", ts)
	}
}

// Columns with their types as COLUMN_TYPE has them, enums' labels, keys:
// the primary key in key order, unique indexes over not-null columns, not
// a nullable or an expression one (§10.1).
func TestTableColumns(t *testing.T) {
	c := connect(t, true)
	ctx := context.Background()
	cs, err := my.TableColumns(ctx, c, "sqlmux", "t_order")
	if err != nil {
		t.Fatal(err)
	}
	if id := cs.Cols[0]; id.Name != "id" || id.Type != "bigint" || !id.NotNull || id.Default != "auto_increment" {
		t.Errorf("id: %+v", id)
	}
	if st := cs.Cols[2]; st.Default != "pending" || !slices.Equal(st.Enum, []string{"pending", "queued", "running", "done", "failed"}) {
		t.Errorf("status: %+v", st)
	}
	if paid := cs.Cols[4]; paid.Type != "tinyint(1)" || paid.NotNull || paid.Default != "" {
		t.Errorf("paid: %+v", paid)
	}
	if !slices.Equal(cs.PK, []string{"id"}) || cs.Unique != nil {
		t.Errorf("keys: %v %v", cs.PK, cs.Unique)
	}
	for table, want := range map[string]string{
		"t_order_item": "[order_id line_no] []",
		"t_event":      "[occurred_at id] []",
		"t_sku":        "[] [[code] [title code]]",
		"t_log":        "[] []",
	} {
		cs, err := my.TableColumns(ctx, c, "sqlmux", table)
		if got := fmt.Sprint(cs.PK, cs.Unique); err != nil || strings.ReplaceAll(got, "[[]]", "[]") != want && got != want {
			t.Errorf("%s: %s, want %s (%v)", table, got, want, err)
		}
	}
	if got := enumOf(`enum('a','it''s','')`); !slices.Equal(got, []string{"a", "it's", ""}) {
		t.Errorf("enumOf: %q", got)
	}
}

// DDL is SHOW CREATE's, a table's or a view's.
func TestDDL(t *testing.T) {
	c := connect(t, true)
	for name, want := range map[string]string{"t_log": "CREATE TABLE `t_log` (", "v_paid_order": "CREATE ALGORITHM="} {
		if got, err := my.DDL(context.Background(), c, "sqlmux", name); err != nil || !strings.HasPrefix(got, want) || !strings.HasSuffix(got, ";") {
			t.Errorf("%s: %v\n%s", name, err, got)
		}
	}
}
