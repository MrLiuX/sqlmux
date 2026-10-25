//go:build integration

package mssql

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/kackerx/sqlmux/internal/db"
	"github.com/kackerx/sqlmux/internal/sqlkit"
)

func connect(t *testing.T) *Conn {
	t.Helper()
	c, _ := connectInfo(t, false)
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

// The URL the form composes and the keyword form connect alike; the address
// is user@host:port, the database the DSN's.
func TestConnect(t *testing.T) {
	for _, dsn := range []string{IntegrationDSN(t), "server=localhost,51433;user id=sa;password=Sqlmux!2026;database=sqlmux"} {
		c, info, err := Connect(context.Background(), dsn, "", false)
		if err != nil {
			t.Fatalf("%s: %v", dsn, err)
		}
		if info.Addr != "sa@localhost:51433" || info.Database != "sqlmux" || info.Path != "" {
			t.Errorf("%s: %+v", dsn, info)
		}
		if c.TxStatus() != 'I' {
			t.Errorf("%s: tx %c", dsn, c.TxStatus())
		}
		c.Close()
	}
}

// Every value comes back as the driver's text of its type (§8.1): the
// datetime family with the T between its parts and the column's scale, a
// bit as true or false, money and decimal as written, NULL apart from "".
func TestQueryReturnsText(t *testing.T) {
	c := connect(t)
	r, err := c.Query(context.Background(), "select status, paid, meta, raw, note, created_at, amount, '' from t_order where id = @p1", db.Val{S: "3"})
	if err != nil {
		t.Fatal(err)
	}
	want := []db.Val{
		{S: "failed"}, {Null: true}, {S: `{"n": 3, "tags": ["a", "b"]}`}, {S: `{"n": 3}`},
		{Null: true}, {S: "2026-09-01T00:03:00.0000000"}, {S: "3.99"}, {S: ""},
	}
	if len(r.Rows) != 1 || r.Tag != "SELECT 1" {
		t.Fatalf("rows %v, tag %q", r.Rows, r.Tag)
	}
	for i, v := range r.Rows[0] {
		if v != want[i] {
			t.Errorf("%s = %+v, want %+v", r.Cols[i].Name, v, want[i])
		}
	}
	types := []string{"nvarchar", "bit", "nvarchar", "nvarchar", "nvarchar", "datetime2", "decimal", "varchar"}
	for i, typ := range types {
		if r.Cols[i].Type != typ {
			t.Errorf("%s type %q, want %q", r.Cols[i].Name, r.Cols[i].Type, typ)
		}
	}
	// a money and a decimal read as the text try_convert(nvarchar, …, 126) has
	r, err = c.Query(context.Background(), "select cast(12.3456 as money), cast(12.3456 as decimal(10, 4)), cast('2026-01-02T03:04:05.123' as datetime2(3)), cast('2026-01-02T03:04:05.1234567+08:00' as datetimeoffset), cast('03:04:05.1234' as time(4)), cast('2026-01-02' as date), cast(1 as bit), cast('6F9619FF-8B86-D011-B42D-00C04FC964FF' as uniqueidentifier)")
	if err != nil {
		t.Fatal(err)
	}
	want = []db.Val{
		{S: "12.3456"}, {S: "12.3456"}, {S: "2026-01-02T03:04:05.123"},
		{S: "2026-01-02T03:04:05.1234567+08:00"}, {S: "03:04:05.1234"}, {S: "2026-01-02"}, {S: "true"},
		{S: "6F9619FF-8B86-D011-B42D-00C04FC964FF"},
	}
	for i, v := range r.Rows[0] {
		if v != want[i] {
			t.Errorf("%s = %+v, want %+v", r.Cols[i].Name, v, want[i])
		}
	}
	// bytes that are no UTF-8 by their size (§8.1)
	r, err = c.Query(context.Background(), "select cast(0xABCD as varbinary(2))")
	if err != nil {
		t.Fatal(err)
	}
	if r.Rows[0][0].S != db.BinaryShown(2) {
		t.Errorf("varbinary = %q", r.Rows[0][0].S)
	}
	if _, err := c.Query(context.Background(), "select @p1", db.Val{Null: true}); err != nil {
		t.Errorf("a NULL argument: %v", err)
	}
}

// A cancel is the driver's attention: the statement stops within the
// second, and the connection goes with it — the driver closes one whose ctx
// was cancelled, whatever the pool's callers see — so the Worker dials for
// the next request, and in a transaction of the user's the loss is told
// (§8.2, §8.3, F18.6).
func TestCancelStops(t *testing.T) {
	c := connect(t)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	start := time.Now()
	if _, err := c.Query(ctx, "waitfor delay '00:00:10'"); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("the waitfor: %v", err)
	}
	if time.Since(start) > 3*time.Second {
		t.Fatalf("the cancel took %v", time.Since(start))
	}
	if !c.Closed() {
		t.Fatal("after the cancel the connection is not marked gone")
	}
}

// A save goes in its own transaction, is found stale when the row changed
// under it, and a delete of a gone row says so (§10.3).
func TestSave(t *testing.T) {
	dsn := OwnDB(t)
	w, e := workerOf(t, dsn), Engine{}
	ctx := context.Background()
	r, err := w.Query(ctx, "select name, email from dbo.t_user where id = 2")
	if err != nil {
		t.Fatal(err)
	}
	ch := db.Changes{Updates: []db.Row{{Key: []string{"2"}, Cols: []db.Change{
		{Name: "email", Val: db.Val{S: "user_2@elsewhere.example"}, Old: r.Rows[0][1]},
	}}}}
	if failed, _, err := db.Save(ctx, w, e, "dbo", "t_user", []string{"id"}, ch, false); failed != -1 || err != nil {
		t.Fatalf("the update: failed %d, err %v", failed, err)
	}
	// the same old value twice: stale, the row holding the new one now
	if failed, _, err := db.Save(ctx, w, e, "dbo", "t_user", []string{"id"}, ch, false); failed != 0 || !errors.Is(err, db.ErrStale) {
		t.Fatalf("a stale update: failed %d, err %v", failed, err)
	}
	// a default insert, a delete: the table with a default on every column
	if _, err := w.Query(ctx, "create table dbo.t_default (id int identity(1, 1) primary key, note nvarchar(64) default 'n', created_at datetime2 default sysdatetime())"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = w.Query(ctx, "drop table dbo.t_default") })
	if failed, _, err := db.Save(ctx, w, e, "dbo", "t_default", nil, db.Changes{Inserts: [][]db.Change{{}}}, false); failed != -1 || err != nil {
		t.Fatalf("a default row: failed %d, err %v", failed, err)
	}
	// the default row's row is the one to delete, a table with no dependent
	if failed, _, err := db.Save(ctx, w, e, "dbo", "t_default", []string{"id"}, db.Changes{Deletes: [][]string{{"1"}}}, false); failed != -1 || err != nil {
		t.Fatalf("the delete: failed %d, err %v", failed, err)
	}
	if failed, _, err := db.Save(ctx, w, e, "dbo", "t_default", []string{"id"}, db.Changes{Deletes: [][]string{{"1"}}}, false); failed != 0 || !errors.Is(err, db.ErrGone) {
		t.Fatalf("a gone row: failed %d, err %v", failed, err)
	}
}

// workerOf is the Worker of a connection of dsn's, for Save.
func workerOf(t *testing.T, dsn string) *db.Worker {
	t.Helper()
	c, _, err := Connect(context.Background(), dsn, "", false)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.Close() })
	return db.NewWorker(c)
}

// The schemas are dbo's kind, the system's out; the databases are the
// server's the user may reach, the system's four out (§8.4, F18.6).
func TestSchemasAndDatabases(t *testing.T) {
	c := connect(t)
	schemas, current, err := (Engine{}).Schemas(context.Background(), c)
	if err != nil {
		t.Fatal(err)
	}
	if current != "dbo" || len(schemas) == 0 || schemas[0] != "dbo" {
		t.Errorf("schemas %v, current %q", schemas, current)
	}
	for _, s := range schemas {
		if s == "sys" || s == "INFORMATION_SCHEMA" || s == "guest" || strings.HasPrefix(s, "db_") {
			t.Errorf("the system schema %s is in the list", s)
		}
	}
	dbs, err := (Engine{}).Databases(context.Background(), c)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"sqlmux", "agentable"} {
		if !slices.Contains(dbs, want) {
			t.Errorf("%s is not among the databases %v", want, dbs)
		}
	}
	for _, s := range []string{"master", "tempdb", "model", "msdb"} {
		if slices.Contains(dbs, s) {
			t.Errorf("the system database %s is in the list", s)
		}
	}
}

// The tree's tables, their kinds and the rows the partition stats estimate;
// UseSchema is nothing and changes nothing (§8.6).
func TestTables(t *testing.T) {
	c := connect(t)
	e := Engine{}
	ts, err := e.Tables(context.Background(), c, nil)
	if err != nil {
		t.Fatal(err)
	}
	byName := map[string]db.Table{}
	for _, tb := range ts {
		byName[tb.Schema+"."+tb.Name] = tb
	}
	if u := byName["dbo.t_user"]; u.Kind != "r" || u.Rows < 40 {
		t.Errorf("t_user: %+v", u)
	}
	if v := byName["dbo.v_paid_order"]; v.Kind != "v" || v.Rows >= 0 {
		t.Errorf("v_paid_order: %+v", v)
	}
	// agentable's objects are the agentable database's to see, as PG's are
	// per database; its database is a child of the tree (§8.4)
	if err := e.UseSchema(context.Background(), c, "dbo", ""); err != nil {
		t.Errorf("use schema: %v", err)
	}
}

// The columns as the server formats their types, the identity the server
// makes, the key of the unique indexes that can stand in, a filtered one
// none (§10.1).
func TestTableColumns(t *testing.T) {
	c := connect(t)
	e := Engine{}
	cols, err := e.TableColumns(context.Background(), c, "dbo", "t_user")
	if err != nil {
		t.Fatal(err)
	}
	if cols.PK == nil || len(cols.PK) != 1 || cols.PK[0] != "id" {
		t.Errorf("the key: %v", cols.PK)
	}
	id := cols.Cols[0]
	if id.Name != "id" || id.Type != "bigint" || !id.NotNull || !id.Generated || id.Default != "identity(1, 1)" {
		t.Errorf("id: %+v", id)
	}
	if name := cols.Cols[1]; name.Type != "nvarchar(64)" || !name.NotNull {
		t.Errorf("name: %+v", name)
	}
	if email := cols.Cols[2]; email.NotNull || email.Generated || email.Default != "" {
		t.Errorf("email: %+v", email)
	}
	// a unique index stands in for the key; a filtered one does not
	cols, err = e.TableColumns(context.Background(), c, "dbo", "t_sku")
	if err != nil {
		t.Fatal(err)
	}
	if cols.PK != nil {
		t.Errorf("t_sku has no primary key: %v", cols.PK)
	}
	if len(cols.Unique) != 2 || cols.Unique[0][0] != "code" || cols.Unique[1][0] != "title" || cols.Unique[1][1] != "code" {
		t.Errorf("the unique indexes: %v", cols.Unique)
	}
	// a composite primary key in key order, not the columns'
	cols, err = e.TableColumns(context.Background(), c, "dbo", "t_event")
	if err != nil {
		t.Fatal(err)
	}
	if len(cols.PK) != 2 || cols.PK[0] != "occurred_at" || cols.PK[1] != "id" {
		t.Errorf("t_event's key: %v", cols.PK)
	}
}

// A table's DDL is put together from the catalog, a view's is the server's
// own (§12「预览」, F18.6).
func TestDDL(t *testing.T) {
	c := connect(t)
	e := Engine{}
	d, err := e.DDL(context.Background(), c, "dbo", "t_user")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"create table [dbo].[t_user] (", "[id] bigint not null identity", "[name] nvarchar(64) not null", "primary key ([id])"} {
		if !strings.Contains(d, want) {
			t.Errorf("the ddl misses %q:\n%s", want, d)
		}
	}
	d, err = e.DDL(context.Background(), c, "dbo", "v_paid_order")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(d, "create view") {
		t.Errorf("the view's ddl: %q", d)
	}
}

// The indexes with the statement that makes them, the primary key and the
// foreign keys (F14.3).
func TestIndexesAndKeys(t *testing.T) {
	c := connect(t)
	e := Engine{}
	ix, err := e.Indexes(context.Background(), c, "dbo", "t_order")
	if err != nil {
		t.Fatal(err)
	}
	var def string
	for _, i := range ix {
		if i.Name == "t_order_user_created" {
			def = i.Def
		}
	}
	if want := "create nonclustered index [t_order_user_created] on [dbo].[t_order] ([user_id], [created_at]);"; def != want {
		t.Errorf("the index: %q, want %q", def, want)
	}
	keys, err := e.Keys(context.Background(), c, "dbo", "t_order")
	if err != nil {
		t.Fatal(err)
	}
	if len(keys) != 2 || keys[0].Name != "PRIMARY" || keys[1].Ref == nil ||
		keys[1].Ref.Table != "t_user" || keys[1].Ref.Cols[0] != "id" || keys[1].Cols[0] != "user_id" {
		t.Errorf("the keys: %+v", keys)
	}
}

// The functions with their arguments, the definition the server keeps; a
// sequence of the schema's own (F14.3).
func TestFunctionsAndSequences(t *testing.T) {
	c := connect(t)
	e := Engine{}
	fs, err := e.Functions(context.Background(), c, nil)
	if err != nil {
		t.Fatal(err)
	}
	var greet db.Function
	for _, f := range fs {
		if f.Name == "fn_greet" {
			greet = f
		}
	}
	if greet.Schema != "dbo" || !strings.Contains(greet.Args, "who") || greet.ID == "" {
		t.Fatalf("fn_greet: %+v", greet)
	}
	d, err := e.FunctionDef(context.Background(), c, greet)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(d, "create function") || !strings.Contains(d, "hello") {
		t.Errorf("the definition: %q", d)
	}
}

// One batch, several result sets (§8.1); GO is the client's, and a batch of
// GO statements runs as the statements of it (F18.6).
func TestGOBatches(t *testing.T) {
	c := connect(t)
	rs, err := c.Exec(context.Background(), "select 1 as a; select 2 as b", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(rs) != 2 || len(rs[0].Rows) != 1 || rs[0].Rows[0][0].S != "1" || rs[1].Rows[0][0].S != "2" {
		t.Errorf("the batch: %+v", rs)
	}
	// variables live within a statement; the text as GO cut it runs
	if _, err := c.Exec(context.Background(), "declare @x int = 41\nselect @x + 1", 0); err != nil {
		t.Errorf("a batch of two statements: %v", err)
	}
	if stmts := sqlkit.Statements("select 1;\ngo\nselect 2;", sqlkit.MSSQL); len(stmts) != 2 {
		t.Errorf("GO cut: %d statements", len(stmts))
	}
}

// Quick SQL runs in a transaction that is rolled back, a write the words
// call is not sent at all, and the rows are capped (§12, F18.6).
func TestQuick(t *testing.T) {
	dsn := OwnDB(t)
	c, _, err := Connect(context.Background(), dsn, "", false)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.Close() })
	w := db.NewWorker(c)
	e := Engine{}
	if _, err := e.Quick(context.Background(), w, "delete from dbo.t_user", "dbo", 10); err == nil {
		t.Fatalf("a write: %v", err)
	}
	r, err := e.Quick(context.Background(), w, "select id from dbo.t_user", "dbo", 10)
	if err != nil || len(r.Rows) != 10 || !r.Truncated {
		t.Fatalf("the read: %v, rows %d, truncated %v", err, len(r.Rows), r.Truncated)
	}
	n, err := db.Count(context.Background(), w, e, db.Query{Schema: "dbo", Table: "t_user"})
	if err != nil || n != 50 {
		t.Fatalf("count_big: %v, %d", err, n)
	}
}

// The pages go by the OFFSET/FETCH of §8.5, the count by count_big.
func TestPage(t *testing.T) {
	c := connect(t)
	e := Engine{}
	q := db.Query{Schema: "dbo", Table: "t_order", Order: []db.OrderCol{{Col: "id"}}, Key: []string{"id"}, Limit: 100}
	r, next, err := db.Page(context.Background(), c, e, q)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Rows) != 100 || !next || r.Rows[0][0].S != "1" || r.Rows[99][0].S != "100" {
		t.Fatalf("the page: %d rows, next %v, first %q", len(r.Rows), next, r.Rows[0][0].S)
	}
	q.Offset = 100
	r, _, err = db.Page(context.Background(), c, e, q)
	if err != nil || r.Rows[0][0].S != "101" {
		t.Fatalf("the second page: %v, first %q", err, r.Rows[0][0].S)
	}
	// no sort, the (SELECT NULL) takes it
	q2 := db.Query{Schema: "dbo", Table: "t_log", Limit: 2}
	if _, _, err := db.Page(context.Background(), c, e, q2); err != nil {
		t.Fatalf("an unordered page: %v", err)
	}
}

// The transaction words: begin transaction puts it in, commit and rollback
// take it out, one naming a savepoint leaves it (§8.2, F18.6).
func TestTxEnds(t *testing.T) {
	c := connect(t)
	ctx := context.Background()
	if _, err := c.Query(ctx, "begin transaction"); err != nil {
		t.Fatal(err)
	}
	if c.TxStatus() != 'T' {
		t.Fatalf("after begin: %c", c.TxStatus())
	}
	if _, err := c.Query(ctx, "save transaction sqlmux_test"); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Query(ctx, "update dbo.t_user set email = email where id = 1"); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Query(ctx, "rollback transaction sqlmux_test"); err != nil {
		t.Fatal(err)
	}
	if c.TxStatus() != 'T' {
		t.Fatalf("after a savepoint's rollback: %c", c.TxStatus())
	}
	if _, err := c.Query(ctx, "commit transaction"); err != nil {
		t.Fatal(err)
	}
	if c.TxStatus() != 'I' {
		t.Fatalf("after commit: %c", c.TxStatus())
	}
}
