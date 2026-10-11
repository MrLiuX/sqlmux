//go:build integration

package postgres

import (
	"context"
	"errors"
	"strings"
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

	big := "select generate_series(1, 100000000), repeat('x', 100)"
	ctx, cancel := context.WithCancel(context.Background())
	n = 0
	start := time.Now()
	err = c.Stream(ctx, big, func([]db.Col) error { return nil }, func([]db.Val) error {
		if n++; n == 100 {
			cancel()
		}
		return nil
	})
	if err == nil || time.Since(start) > 2*time.Second { // the server's own message, 57014; the Worker turns it into the ctx's
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

	// a statement the server refuses describes no column: its message is the error, not the head's
	head := errors.New("no such column")
	err = c.Stream(context.Background(), "select * from no_such_table_x", func([]db.Col) error { return head }, func([]db.Val) error { return nil })
	if err == nil || errors.Is(err, head) || !strings.Contains(err.Error(), "no_such_table_x") {
		t.Fatalf("a statement that fails: %v", err)
	}
}
