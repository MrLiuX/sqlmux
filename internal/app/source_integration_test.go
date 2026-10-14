//go:build integration

package app

import (
	"errors"
	"slices"
	"testing"

	"github.com/kackerx/sqlmux/internal/config"
	"github.com/kackerx/sqlmux/internal/db"
)

// A SQLTable reads and writes what the statements of db.Page, db.Count,
// db.Select and db.Save do on each engine (F16.0): pages by offset and by
// ORDER, the row identity turned, a count, every row of Each, and a save, which
// a stale row and a read-only connection refuse.
func TestSQLTableIsTheStatements(t *testing.T) { eachEngine(t, sqlTable) }

func sqlTable(t *testing.T, eng, _ string) {
	ctx := t.Context()
	a := opened(t, config.Connection{Name: "play", Engine: eng, DSN: ownDB(t, eng)})
	play := a.sess.Conns[0]
	if _, err := play.Main.Query(ctx, "create table w (id integer primary key, v text)"); err != nil {
		t.Fatal(err)
	}
	if _, err := play.Main.Query(ctx, "insert into w values (1, 'a'), (2, 'b'), (3, 'c'), (4, 'd'), (5, 'e')"); err != nil {
		t.Fatal(err)
	}
	src := db.SQLTable{E: play.eng, Schema: play.Schema, Name: "w", Meta: play.Meta, Main: play.Main}
	cols, err := src.Columns(ctx)
	if err != nil || !slices.Equal(cols.Key(), []string{"id"}) {
		t.Fatalf("columns: %+v %v", cols, err)
	}
	ids := func(r db.Result) (ids []string) {
		for _, row := range r.Rows {
			ids = append(ids, row[0].S)
		}
		return ids
	}
	for _, c := range []struct {
		name string
		q    db.PageQuery
		want []string
		next bool
	}{
		{"first", db.PageQuery{Limit: 2}, []string{"1", "2"}, true},
		{"offset", db.PageQuery{Limit: 2, Offset: 4}, []string{"5"}, false},
		{"filter", db.PageQuery{Limit: 10, Filter: "id > 2"}, []string{"3", "4", "5"}, false},
		{"order", db.PageQuery{Limit: 10, Order: orderOf("v-")}, []string{"5", "4", "3", "2", "1"}, false},
		{"turned", db.PageQuery{Limit: 2, Order: []db.OrderCol{{Col: "id", Desc: true}}}, []string{"5", "4"}, true},
		{"live", db.PageQuery{Limit: 2, Live: true}, []string{"1", "2"}, true},
	} {
		c.q.Cols = cols
		got, err := src.Page(ctx, c.q)
		if err != nil || !slices.Equal(ids(got.Result), c.want) || got.Next != c.next || got.Cursor != "" {
			t.Errorf("%s: %v next %v cursor %q, %v; want %v next %v", c.name, ids(got.Result), got.Next, got.Cursor, err, c.want, c.next)
		}
		old, next, err := db.Page(ctx, play.Meta, play.eng, db.Query{Schema: play.Schema, Table: "w", Where: c.q.Filter, Order: c.q.Order, Key: cols.Key(), Limit: c.q.Limit, Offset: c.q.Offset})
		if err != nil || !slices.Equal(ids(old), c.want) || next != c.next {
			t.Errorf("%s: db.Page says %v next %v, %v", c.name, ids(old), next, err)
		}
	}
	// Live reads on Main what a transaction of it wrote and has not committed; the others on Meta do not see it
	for _, sql := range []string{"begin", "insert into w values (6, 'f')"} {
		if _, err := play.Main.Query(ctx, sql); err != nil {
			t.Fatal(sql, err)
		}
	}
	for _, live := range []bool{true, false} {
		got, err := src.Page(ctx, db.PageQuery{Cols: cols, Limit: 10, Offset: 5, Live: live})
		if seen := len(got.Rows) == 1; err != nil || seen != live {
			t.Errorf("live %v: %v %v", live, ids(got.Result), err)
		}
	}
	if _, err := play.Main.Query(ctx, "rollback"); err != nil {
		t.Fatal(err)
	}
	if n, err := src.Count(ctx, "id > 1"); err != nil || n != 4 {
		t.Errorf("count: %d %v", n, err)
	}
	if _, err := src.Page(ctx, db.PageQuery{Cols: cols, Limit: 1, Filter: "nope = 1"}); err == nil {
		t.Error("a WHERE that fails is an error")
	}
	var head []db.Col
	var got []string
	each := func(q db.PageQuery) error {
		head, got = nil, nil
		return src.Each(ctx, q, func(c []db.Col) error { head = c; return nil }, func(r []db.Val) error { got = append(got, r[0].S); return nil })
	}
	if err := each(db.PageQuery{Cols: cols, Order: orderOf("id-")}); err != nil || len(head) != 2 || !slices.Equal(got, []string{"5", "4", "3", "2", "1"}) {
		t.Errorf("each: %v %v %v", head, got, err)
	}
	if err := each(db.PageQuery{Cols: cols, Limit: 2, Offset: 1}); err != nil || !slices.Equal(got, []string{"2", "3"}) {
		t.Errorf("each of a page: %v %v", got, err)
	}
	// a save: an update that finds the row as it was, one that does not, and a refusal
	row := func(old string) db.Changes {
		return db.Changes{Updates: []db.Row{{Key: []string{"2"}, Cols: []db.Change{{Name: "v", Val: db.Val{S: "B"}, Old: db.Val{S: old}}}}}}
	}
	if failed, _, err := src.Save(ctx, row("b"), false); err != nil || failed != -1 {
		t.Fatalf("save: %d %v", failed, err)
	}
	if failed, _, err := src.Save(ctx, row("b"), false); !errors.Is(err, db.ErrStale) || failed != 0 {
		t.Errorf("save of a stale row: %d %v", failed, err)
	}
	if r, err := play.Meta.Query(ctx, "select v from w where id = 2"); err != nil || r.Rows[0][0].S != "B" {
		t.Errorf("in the table: %v %v", r.Rows, err)
	}
	ro := src
	ro.ReadOnly = true
	if _, tx, err := ro.Save(ctx, row("B"), false); !errors.Is(err, db.ErrReadOnly) || tx != 0 {
		t.Errorf("read-only: %c %v", tx, err)
	}
	if r, _ := play.Meta.Query(ctx, "select v from w where id = 2"); r.Rows[0][0].S != "B" {
		t.Error("a read-only save wrote")
	}
}
