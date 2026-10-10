//go:build integration

package mysql

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"sqlmux/internal/db"
	"sqlmux/internal/i18n"
	"sqlmux/internal/sqlkit"
)

// The tests read the Chinese texts: the language they run in is not the
// default, English (F7.6).
func init() { i18n.Set("zh") }

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
	types := []string{"char", "tinyint", "json", "json", "text", "datetime", "decimal"} // v1.7.1 names an enum char
	for i, typ := range types {
		if r.Cols[i].Type != typ {
			t.Errorf("%s type %q, want %q", r.Cols[i].Name, r.Cols[i].Type, typ)
		}
	}
	if _, err := c.Query(context.Background(), "select ?", db.Val{Null: true}); err != nil {
		t.Errorf("a NULL argument: %v", err)
	}

	own, _, err := Connect(context.Background(), OwnDB(t), "", false)
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

// What turns MySQL 8.4's read only off is what BreaksReadOnly keeps
// back, statement by statement, a write after each (§13); SET GLOBAL and
// PERSIST are left out, the server being shared.
func TestReadOnlyUnlocks(t *testing.T) {
	ctx := context.Background()
	dsn := OwnDB(t)
	admin, _, err := Connect(ctx, dsn, "", false)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	if _, err := admin.Query(ctx, "create table probe (a int)"); err != nil {
		t.Fatal(err)
	}
	cfg, _ := Config(dsn, "")
	probe := my.Quote(cfg.DBName, "probe") // USE goes elsewhere
	for _, c := range []struct {
		stmt          string
		unlocks, kept bool
	}{
		{"set session transaction read write", true, true},
		{"SET LOCAL TRANSACTION READ WRITE", true, true},
		{"set transaction read write", true, true}, // the next transaction's: the write's
		{"set transaction_read_only = off", true, true},
		{"SET @@SESSION.transaction_read_only = 0", true, true},
		{"set @@transaction_read_only = 0", true, true},
		{"set local `transaction_read_only` = 0", true, true},
		{"set @a = 1, transaction_read_only = 0", true, true},
		{"/*!40101 SET SESSION transaction_read_only = 0 */", true, true},
		{"start transaction read write", true, true},
		// kept back as their kind, harmless
		{"set session transaction isolation level read committed", false, true},
		{"set transaction read only", false, true},
		// let go
		{"set @transaction_read_only = 0", false, false},
		{"set names utf8mb4, @@session.sql_mode = default", false, false},
		{"set autocommit = 0", false, false},
		{"use sqlmux", false, false},
		{"begin", false, false},
		{"start transaction read only", false, false},
	} {
		conn, _, err := Connect(ctx, dsn, "", true)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := conn.Exec(ctx, c.stmt, 0); err != nil {
			t.Fatalf("%s: %v", c.stmt, err)
		}
		_, err = conn.Exec(ctx, "insert into "+probe+" values (1)", 0)
		if unlocked := err == nil; unlocked != c.unlocks {
			t.Errorf("%s: unlocked %v: %v", c.stmt, unlocked, err)
		}
		if kept := sqlkit.BreaksReadOnly(c.stmt, sqlkit.MySQL); kept != c.kept {
			t.Errorf("%s: BreaksReadOnly %v", c.stmt, kept)
		}
		conn.Close()
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

// DDL is SHOW CREATE's, a table's or a view's, the table's keys in it (F9.7).
func TestDDL(t *testing.T) {
	c := connect(t, true)
	if got, err := my.DDL(context.Background(), c, "sqlmux", "t_sku"); err != nil || !strings.Contains(got, "UNIQUE KEY `t_sku_code` (`code`)") {
		t.Errorf("t_sku's index: %v\n%s", err, got)
	}
	if got, err := my.DDL(context.Background(), c, "sqlmux", "t_order"); err != nil || strings.Count(got, "PRIMARY KEY") != 1 {
		t.Errorf("t_order's primary key, once: %v\n%s", err, got)
	}
	for name, want := range map[string]string{"t_log": "CREATE TABLE `t_log` (", "v_paid_order": "CREATE ALGORITHM="} {
		if got, err := my.DDL(context.Background(), c, "sqlmux", name); err != nil || !strings.HasPrefix(got, want) || !strings.HasSuffix(got, ";") {
			t.Errorf("%s: %v\n%s", name, err, got)
		}
	}
}

// A transaction ends with what commits it implicitly, a DDL, and with a
// deadlock, the server taking it back whole, the error saying so; a
// statement cancelled before it runs does not run (§8.2, §8.3).
func TestTxEnds(t *testing.T) {
	ctx := context.Background()
	dsn := OwnDB(t)
	conn := func() *Conn {
		c, _, err := Connect(ctx, dsn, "", false)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { c.Close() })
		return c
	}
	a, b := conn(), conn()
	for _, s := range []string{"create table k (id int primary key, n int)", "insert into k values (1, 0), (2, 0)", "begin", "update k set n = 1 where id = 1"} {
		if _, err := a.Query(ctx, s); err != nil {
			t.Fatalf("%s: %v", s, err)
		}
	}
	if _, err := a.Query(ctx, "create table x (i int)"); err != nil || a.TxStatus() != 'I' {
		t.Fatalf("a DDL: %c %v", a.TxStatus(), err)
	}
	if r, _ := b.Query(ctx, "select n from k where id = 1"); r.Rows[0][0].S != "1" {
		t.Errorf("the update before the DDL: %v", r.Rows)
	}
	// a and b lock 1 and 2 the other way round: one of them is taken back
	for _, s := range []string{"begin", "update k set n = 2 where id = 1"} {
		_, _ = a.Query(ctx, s)
	}
	for _, s := range []string{"begin", "update k set n = 2 where id = 2"} {
		_, _ = b.Query(ctx, s)
	}
	errA := make(chan error, 1)
	go func() { _, err := a.Query(ctx, "update k set n = 3 where id = 2"); errA <- err }()
	time.Sleep(300 * time.Millisecond)
	_, errB := b.Query(ctx, "update k set n = 3 where id = 1")
	victim, err := b, errB
	if errA := <-errA; err == nil { // a done either way, before it is used again
		victim, err = a, errA
	}
	if !errors.As(err, new(db.RolledBack)) || errno(err) != 1213 || victim.TxStatus() != 'I' {
		t.Errorf("deadlock: %v, %c", err, victim.TxStatus())
	}
	_, _ = a.Query(ctx, "rollback")
	_, _ = b.Query(ctx, "rollback")
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := a.Exec(cancelled, "insert into k values (9, 9)", 0); !errors.Is(err, context.Canceled) {
		t.Errorf("cancelled: %v", err)
	}
	if r, _ := b.Query(ctx, "select count(*) from k where id = 9"); r.Rows[0][0].S != "0" {
		t.Error("a cancelled insert ran")
	}
}

// A procedure's rows come back; a WITH that writes counts what it hit
// (§11).
func TestCallAndWithWrites(t *testing.T) {
	ctx := context.Background()
	c, _, err := Connect(ctx, OwnDB(t), "", false)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	for _, s := range []string{"create procedure p() select 1 as one, 2 as two", "create table k (id int primary key, n int)", "insert into k values (1, 0), (2, 0)"} {
		if _, err := c.Query(ctx, s); err != nil {
			t.Fatalf("%s: %v", s, err)
		}
	}
	if r, err := c.Query(ctx, "call p()"); err != nil || len(r.Cols) != 2 || len(r.Rows) != 1 || r.Rows[0][1].S != "2" {
		t.Errorf("call: %+v %v", r, err)
	}
	if r, err := c.Query(ctx, "with x as (select 1 as id) update k set n = 7 where id in (select id from x)"); err != nil || r.Affected != 1 || r.Tag != "WITH 1" {
		t.Errorf("with … update: %+v %v", r, err)
	}
}

// A table is read only with a binary row identity, or on an engine with
// no transactions; a BIT column is Fixed (§10.1, §8.1).
func TestReadOnlyTables(t *testing.T) {
	ctx := context.Background()
	dsn := OwnDB(t)
	c, _, err := Connect(ctx, dsn, "", false)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	for _, s := range []string{
		"create table m (id int primary key) engine = myisam",
		"create table bk (id binary(16) primary key)",
		"create table bt (id int primary key, f bit(1))",
		"create table gc (id int primary key, a int, v int as (a + 1), s int as (a + 2) stored, d int default (a))",
	} {
		if _, err := c.Query(ctx, s); err != nil {
			t.Fatalf("%s: %v", s, err)
		}
	}
	cfg, _ := Config(dsn, "")
	if cs, err := my.TableColumns(ctx, c, cfg.DBName, "gc"); err != nil || cs.Cols[1].Generated || !cs.Cols[2].Generated || !cs.Cols[3].Generated || cs.Cols[4].Generated {
		t.Errorf("Generated (plain, virtual, stored, expression default): %+v %v", cs.Cols, err)
	}
	for table, want := range map[string]string{"m": "引擎 MyISAM 不支持事务", "bk": "行标识列 id 是 binary(16)", "bt": ""} {
		cs, err := my.TableColumns(ctx, c, cfg.DBName, table)
		if err != nil || cs.ReadOnly != want {
			t.Errorf("%s: %q %v", table, cs.ReadOnly, err)
		}
		if table == "bt" && !cs.Cols[1].Fixed {
			t.Error("a BIT is to edit")
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
	if _, err := c.Query(ctx, "create table t_lit (n int auto_increment primary key, s text)"); err != nil {
		t.Fatal(err)
	}
	for _, s := range []string{"done", "it's", `a\b'c\`, `\`, `\'`, "two\nlines", "中文 ' \"", "''"} {
		if _, err := c.Query(ctx, "insert into t_lit (s) values ("+sqlkit.Literal(s, sqlkit.MySQL)+")"); err != nil {
			t.Fatalf("%q: %v", s, err)
		}
		if r, err := c.Query(ctx, "select s from t_lit order by n desc limit 1"); err != nil || r.Rows[0][0].S != s {
			t.Errorf("%q came back %v, %v", s, r.Rows, err)
		}
	}
}
