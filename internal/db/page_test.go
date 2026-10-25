package db

import (
	"context"
	"strings"
	"testing"

	"github.com/kackerx/sqlmux/internal/sqlkit"
)

// captureConn is a Conn that keeps what it was asked, and answers no rows:
// what the page and the count ask of it is the news (F18.6).
type captureConn struct{ sql string }

func (c *captureConn) Exec(context.Context, string, int) ([]Result, error) { return nil, nil }
func (c *captureConn) Query(_ context.Context, sql string, _ ...Val) (Result, error) {
	c.sql = sql
	return Result{Rows: [][]Val{{{S: "0"}}}}, nil // the count's row
}
func (c *captureConn) Close() error { return nil }

// dialectEngine is an Engine that is only its dialect, its names joined by
// dots.
type dialectEngine struct {
	Engine
	d sqlkit.Dialect
}

func (e dialectEngine) Dialect() sqlkit.Dialect    { return e.d }
func (dialectEngine) Quote(names ...string) string { return strings.Join(names, ".") }

// The place of a page and of a count, as each dialect writes it (§8.5):
// LIMIT and OFFSET, or SQL Server's OFFSET/FETCH with the ORDER BY it must
// have, and its COUNT_BIG (F18.6).
func TestPlaceOf(t *testing.T) {
	pg := dialectEngine{d: sqlkit.PG}
	ms := dialectEngine{d: sqlkit.MSSQL}
	q := Query{Schema: "s", Table: "t", Order: []OrderCol{{Col: "id"}}, Limit: 100, Offset: 200}
	c := &captureConn{}
	if _, _, err := Page(context.Background(), c, pg, q); err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(c.sql, " order by id asc limit 101 offset 200") {
		t.Errorf("the pg page: %q", c.sql)
	}
	c = &captureConn{}
	if _, _, err := Page(context.Background(), c, ms, q); err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(c.sql, " order by id asc offset 200 rows fetch next 101 rows only") {
		t.Errorf("the mssql page: %q", c.sql)
	}
	// no sort: the (SELECT NULL) takes the ORDER BY the OFFSET must have
	c = &captureConn{}
	if _, _, err := Page(context.Background(), c, ms, Query{Schema: "s", Table: "t", Limit: 3}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(c.sql, " order by (select null) offset 0 rows fetch next 4 rows only") {
		t.Errorf("the unordered page: %q", c.sql)
	}
	if n, err := Count(context.Background(), c, ms, Query{Schema: "s", Table: "t"}); err != nil || n != 0 {
		t.Fatalf("the count: %v, %d", err, n)
	}
	if !strings.Contains(c.sql, "select count_big(*)") {
		t.Errorf("the count: %q", c.sql)
	}
	c = &captureConn{}
	if _, err := Count(context.Background(), c, pg, Query{Schema: "s", Table: "t"}); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(c.sql, "count_big") {
		t.Errorf("the pg count: %q", c.sql)
	}
}
