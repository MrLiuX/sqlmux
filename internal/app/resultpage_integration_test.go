//go:build integration

package app

import (
	"strconv"
	"testing"

	"github.com/kackerx/sqlmux/internal/config"
)

// A query's select over 1203 rows is three pages of 500, 500 and 203, the
// ids in order and none twice, on either engine (F10.4).
func TestResultPagingOnServer(t *testing.T) { eachEngine(t, resultPagingOnServer) }

func resultPagingOnServer(t *testing.T, eng, _ string) {
	a := opened(t, config.Connection{Name: "play", Engine: eng, DSN: ownDB(t, eng)})
	c := queryOf(a.win().Root.B.Pane)
	run := func(sql string) {
		t.Helper()
		c.ed.Load(sql)
		settle(a, a.runSQL(c, c.conn, c.schema, "query_1", sql, 0, c.ver))
	}
	if eng == "mysql" {
		run("set session cte_max_recursion_depth = 5000") // the default 1000 is short of the rows
		run("create table paged (id int primary key)")
		run("insert into paged with recursive s(n) as (select 1 union all select n + 1 from s where n < 1203) select n from s")
	} else if eng == "sqlite" {
		run("create table paged as with recursive s(n) as (select 1 union all select n + 1 from s where n < 1203) select n as id from s")
	} else {
		run("create table paged as select g as id from generate_series(1, 1203) g")
	}
	run("select * from paged order by id")
	p := a.win().Result
	a.win().focus(p.ID)
	rt := resultOf(p)
	if rt == nil || !rt.pageable || len(rt.page.Rows) != 500 || !rt.page.Truncated {
		t.Fatalf("the first page: %+v", rt)
	}
	next := 1
	for page, want := range []int{500, 500, 203} {
		if page > 0 {
			_, cmd := a.Update(teaKey("]"))
			settle(a, cmd)
		}
		rt = resultOf(p)
		if len(rt.page.Rows) != want || rt.page.Truncated != (page < 2) || rt.offset != page*500 {
			t.Fatalf("page %d: %d rows, truncated %v, offset %d", page+1, len(rt.page.Rows), rt.page.Truncated, rt.offset)
		}
		for _, row := range rt.page.Rows {
			if row[0].S != strconv.Itoa(next) {
				t.Fatalf("page %d: id %s, want %d", page+1, row[0].S, next)
			}
			next++
		}
	}
	if next != 1204 {
		t.Errorf("%d rows over the pages", next-1)
	}
	if _, cmd := a.Update(teaKey("]")); cmd != nil {
		t.Error("] past the last page")
	}
}
