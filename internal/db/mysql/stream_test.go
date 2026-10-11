//go:build integration

package mysql

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kackerx/sqlmux/internal/db"
)

// Stream gives the rows as they come, 6000 of t_order; cancelled in the middle
// of a result that has no end in sight it stops at once, the connection good
// for the next statement; an error of row stops it, the connection too (F12.4).
func TestStream(t *testing.T) {
	c := connect(t, true)
	var cols []db.Col
	n := 0
	err := c.Stream(context.Background(), "select * from t_order", func(cs []db.Col) error { cols = cs; return nil }, func(row []db.Val) error {
		if len(row) != len(cols) {
			t.Fatalf("a row of %d values, %d columns", len(row), len(cols))
		}
		n++
		return nil
	})
	if err != nil || n != 6000 || cols[0].Name != "id" {
		t.Fatalf("t_order: %d rows, %v, %v", n, cols, err)
	}

	big := "select a.n, repeat('x', 100) from (select 1 n union select 2 union select 3 union select 4 union select 5 union select 6 union select 7 union select 8 union select 9 union select 10) a, (select 1 union select 2 union select 3 union select 4 union select 5 union select 6 union select 7 union select 8 union select 9 union select 10) b, (select 1 union select 2 union select 3 union select 4 union select 5 union select 6 union select 7 union select 8 union select 9 union select 10) c, (select 1 union select 2 union select 3 union select 4 union select 5 union select 6 union select 7 union select 8 union select 9 union select 10) d, (select 1 union select 2 union select 3 union select 4 union select 5 union select 6 union select 7 union select 8 union select 9 union select 10) e, (select 1 union select 2 union select 3 union select 4 union select 5 union select 6 union select 7 union select 8 union select 9 union select 10) f, (select 1 union select 2 union select 3 union select 4 union select 5 union select 6 union select 7 union select 8 union select 9 union select 10) g"
	ctx, cancel := context.WithCancel(context.Background())
	n = 0
	start := time.Now()
	err = c.Stream(ctx, big, func([]db.Col) error { return nil }, func([]db.Val) error {
		if n++; n == 100 {
			cancel()
		}
		return nil
	})
	if !errors.Is(err, context.Canceled) || time.Since(start) > 10*time.Second {
		t.Fatalf("cancelled after %d rows, %v: %v", n, time.Since(start), err)
	}
	if r, err := c.Query(context.Background(), "select 1"); err != nil || r.Rows[0][0].S != "1" {
		t.Fatalf("after the cancel: %v %v", r.Rows, err)
	}

	boom := errors.New("boom")
	n = 0
	if err := c.Stream(context.Background(), big, func([]db.Col) error { return nil }, func([]db.Val) error {
		if n++; n == 10 {
			return boom
		}
		return nil
	}); !errors.Is(err, boom) {
		t.Fatalf("an error of row: %v", err)
	}
	if r, err := c.Query(context.Background(), "select 1"); err != nil || r.Rows[0][0].S != "1" {
		t.Fatalf("after the stop: %v %v", r.Rows, err)
	}
}
