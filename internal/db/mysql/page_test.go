//go:build integration

package mysql

import (
	"context"
	"testing"

	"github.com/kackerx/sqlmux/internal/db"
)

// Pages as PG's (§8.5): by the row identity, a composite one in key order,
// none unordered; names quoted with backticks.
func TestPage(t *testing.T) {
	c := connect(t, true)
	ctx := context.Background()
	orders := db.Query{Schema: "sqlmux", Table: "t_order", Key: []string{"id"}, Limit: 100}
	r, next, err := db.Page(ctx, c, my, orders)
	if err != nil || len(r.Rows) != 100 || !next || r.Rows[0][0].S != "1" || r.Rows[99][0].S != "100" {
		t.Fatalf("first page: %d rows, next %v, %v", len(r.Rows), next, err)
	}
	last := orders
	last.Offset = 5900
	if r, next, _ = db.Page(ctx, c, my, last); len(r.Rows) != 100 || next {
		t.Errorf("the last full page: %d rows, next %v", len(r.Rows), next)
	}
	r, _, err = db.Page(ctx, c, my, db.Query{Schema: "sqlmux", Table: "t_order_item", Key: []string{"order_id", "line_no"}, Limit: 3})
	if err != nil || r.Rows[1][0].S != "1" || r.Rows[1][1].S != "2" || r.Rows[2][0].S != "2" {
		t.Errorf("composite key order: %v %v", r.Rows, err)
	}
	// no Order, Desc: the row identity sorts the other way
	if r, _, err = db.Page(ctx, c, my, db.Query{Schema: "sqlmux", Table: "t_order", Key: []string{"id"}, Desc: true, Limit: 2}); err != nil || r.Rows[0][0].S != "6000" || r.Rows[1][0].S != "5999" {
		t.Errorf("default order, down: %v %v", r.Rows, err)
	}
	if r, next, err = db.Page(ctx, c, my, db.Query{Schema: "sqlmux", Table: "t_log", Limit: 2}); err != nil || len(r.Rows) != 2 || !next {
		t.Errorf("t_log: %d rows, next %v, %v", len(r.Rows), next, err)
	}
	if _, _, err := db.Page(ctx, c, my, db.Query{Schema: "sqlmux", Table: "no`such", Limit: 1}); errno(err) != 1146 {
		t.Errorf("a quoted missing table: %v", err)
	}
	if r, _, err := db.Page(ctx, c, my, db.Query{Schema: "agentable", Table: "agent", Key: []string{"id"}, Limit: 5}); err != nil || len(r.Rows) != 3 {
		t.Errorf("another database's: %v %v", r.Rows, err)
	}
}

// WHERE goes in as typed (§9.6), # comments too, and can't smuggle in a
// second statement.
func TestPageWhere(t *testing.T) {
	c := connect(t, true)
	ctx := context.Background()
	q := db.Query{Schema: "sqlmux", Table: "t_order", Key: []string{"id"}, Limit: 5}
	for where, first := range map[string]string{
		"status = 'done' -- 备注":               "2",
		"status = 'done' # 备注":                "2",
		"id = 7 or id = 3":                    "3",
		"deleted_at is not null and id > 150": "200",
	} {
		q.Where = where
		r, _, err := db.Page(ctx, c, my, q)
		if err != nil || r.Rows[0][0].S != first {
			t.Errorf("%q: first id %v, want %s; %v", where, r.Rows, first, err)
		}
	}
	q.Where = "status = 'done'"
	if n, err := db.Count(ctx, c, my, q); n != 1500 || err != nil {
		t.Errorf("count done: %d %v", n, err)
	}
	q.Where = "1=1; drop table t_log"
	if _, _, err := db.Page(ctx, c, my, q); errno(err) != 1064 {
		t.Errorf("two statements: %v", err)
	}
	q.Where = "no_such_column = 1"
	if _, _, err := db.Page(ctx, c, my, q); errno(err) != 1054 {
		t.Errorf("a bad condition says why: %v", err)
	}
	r, _, _ := db.Page(ctx, c, my, db.Query{Schema: "sqlmux", Table: "t_order", Order: "status", Desc: true, Key: []string{"id"}, Limit: 2})
	if r.Rows[0][2].S != "failed" || r.Rows[0][0].S != "3" || r.Rows[1][0].S != "7" {
		t.Errorf("status desc, id: %v", r.Rows)
	}
}

// A query's run: each statement on its own, the first to fail stopping
// the rest; a cancelled run leaves Main usable.
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
	run := func(stmts ...string) (rs []db.Result, err error) {
		err = w.Run(ctx, func(ctx context.Context, c db.Conn) error {
			rs, err = db.ExecEach(ctx, c, stmts, 0, nil)
			return err
		})
		return rs, err
	}
	rs, err := run("insert into t_run values (1)", "insert into t_run values ('two')", "insert into t_run values (3)")
	if errno(err) != 1366 || len(rs) != 1 || rs[0].Tag != "INSERT 1" {
		t.Fatalf("second fails: %v %+v", err, rs)
	}
	if r, err := w.Query(ctx, "select group_concat(n) from t_run"); err != nil || r.Rows[0][0].S != "1" {
		t.Fatalf("the first committed, the third never ran: %v %v", r.Rows, err)
	}
}
