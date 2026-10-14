//go:build integration

package postgres

import (
	"context"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/kackerx/sqlmux/internal/db"
)

func TestPage(t *testing.T) {
	c := connect(t, true)
	ctx := context.Background()
	orders := db.Query{Schema: "public", Table: "t_order", Key: []string{"id"}, Limit: 100}
	r, next, err := db.Page(ctx, c, pg, orders)
	if err != nil || len(r.Rows) != 100 || !next || r.Rows[0][0].S != "1" || r.Rows[99][0].S != "100" {
		t.Fatalf("first page: %d rows, next %v, %v", len(r.Rows), next, err)
	}
	last := orders
	last.Offset = 5900
	if r, next, _ = db.Page(ctx, c, pg, last); len(r.Rows) != 100 || next {
		t.Errorf("the last full page: %d rows, next %v", len(r.Rows), next)
	}
	// a composite key orders by its columns in key order
	r, _, err = db.Page(ctx, c, pg, db.Query{Schema: "public", Table: "t_order_item", Key: []string{"order_id", "line_no"}, Limit: 3})
	if err != nil || r.Rows[1][0].S != "1" || r.Rows[1][1].S != "2" || r.Rows[2][0].S != "2" {
		t.Errorf("composite key order: %v %v", r.Rows, err)
	}
	// no Order, Desc: the row identity sorts, every column of it the other way
	desc := db.Query{Schema: "public", Table: "t_order", Key: []string{"id"}, Order: []db.OrderCol{{Col: "id", Desc: true}}, Limit: 2}
	if r, _, err = db.Page(ctx, c, pg, desc); err != nil || r.Rows[0][0].S != "6000" || r.Rows[1][0].S != "5999" {
		t.Errorf("default order, down: %v %v", r.Rows, err)
	}
	want, err := c.Query(ctx, "select order_id, line_no from t_order_item order by order_id desc, line_no desc limit 3 offset 1") // every column of the key the other way, from the second row on
	if err != nil {
		t.Fatal(err)
	}
	r, _, err = db.Page(ctx, c, pg, db.Query{Schema: "public", Table: "t_order_item", Key: []string{"order_id", "line_no"}, Order: []db.OrderCol{{Col: "order_id", Desc: true}, {Col: "line_no", Desc: true}}, Limit: 3, Offset: 1})
	if err != nil || len(r.Rows) != 3 {
		t.Fatalf("composite key, down: %v %v", r.Rows, err)
	}
	for i := range r.Rows {
		if !reflect.DeepEqual(r.Rows[i][:2], want.Rows[i]) {
			t.Errorf("composite key, down: row %d %v, want %v", i, r.Rows[i][:2], want.Rows[i])
		}
	}
	// no key: no order, still paged; names needing quotes are quoted
	if r, next, err = db.Page(ctx, c, pg, db.Query{Schema: "public", Table: "t_log", Limit: 2}); err != nil || len(r.Rows) != 2 || !next {
		t.Errorf("t_log: %d rows, next %v, %v", len(r.Rows), next, err)
	}
	if _, _, err := db.Page(ctx, c, pg, db.Query{Schema: "public", Table: `no"such`, Limit: 1}); sqlState(err) != "42P01" {
		t.Errorf("a quoted missing table: %v", err)
	}
}

// WHERE goes in as typed (§9.6): its OR stays inside, a trailing comment
// comments nothing else out, and it can't smuggle in a second statement.
func TestPageWhere(t *testing.T) {
	c := connect(t, true)
	ctx := context.Background()
	q := db.Query{Schema: "public", Table: "t_order", Key: []string{"id"}, Limit: 5}
	for where, first := range map[string]string{
		"status = 'done' -- 备注":               "2",
		"id = 7 or id = 3":                    "3",
		"deleted_at is not null and id > 150": "200",
		"   ":                                 "1",
	} {
		q.Where = where
		r, _, err := db.Page(ctx, c, pg, q)
		if err != nil || r.Rows[0][0].S != first {
			t.Errorf("%q: first id %v, want %s; %v", where, r.Rows, first, err)
		}
		if n, err := db.Count(ctx, c, pg, q); err != nil || n == 0 {
			t.Errorf("%q: count %d %v", where, n, err)
		}
	}
	q.Where = "status = 'done'"
	if n, _ := db.Count(ctx, c, pg, q); n != 1500 {
		t.Errorf("count done: %d", n)
	}
	q.Where = "1=1; drop table t_log"
	if _, _, err := db.Page(ctx, c, pg, q); sqlState(err) != "42601" {
		t.Errorf("two statements: %v", err)
	}
	if _, _, err := db.Page(ctx, c, pg, db.Query{Schema: "public", Table: "t_log", Where: "1=1); select (1", Limit: 1}); err == nil || !strings.Contains(err.Error(), "SQLSTATE") {
		t.Errorf("closing the parenthesis early: %v", err)
	}
	q.Where = "no_such_column = 1"
	if _, _, err := db.Page(ctx, c, pg, q); sqlState(err) != "42703" {
		t.Errorf("a bad condition says why: %v", err)
	}
	// ordered by a column, the row identity breaks its ties
	r, _, _ := db.Page(ctx, c, pg, db.Query{Schema: "public", Table: "t_order", Order: []db.OrderCol{{Col: "status", Desc: true}}, Key: []string{"id"}, Limit: 2})
	if r.Rows[0][2].S != "failed" || r.Rows[0][0].S != "3" || r.Rows[1][0].S != "7" {
		t.Errorf("status desc, id: %v", r.Rows)
	}
	// two columns, each its way (F12.1)
	r, _, err := db.Page(ctx, c, pg, db.Query{Schema: "public", Table: "t_order", Order: []db.OrderCol{{Col: "status", Desc: true}, {Col: "id", Desc: true}}, Key: []string{"id"}, Limit: 50})
	if err != nil || len(r.Rows) < 10 {
		t.Fatalf("status desc, id desc: %d rows, %v", len(r.Rows), err)
	}
	for i := 1; i < len(r.Rows); i++ {
		prev, cur := r.Rows[i-1], r.Rows[i]
		pid, _ := strconv.Atoi(prev[0].S)
		cid, _ := strconv.Atoi(cur[0].S)
		if prev[2].S < cur[2].S || prev[2].S == cur[2].S && pid < cid {
			t.Fatalf("row %d: %s %s after %s %s", i, cur[2].S, cur[0].S, prev[2].S, prev[0].S)
		}
	}
}
