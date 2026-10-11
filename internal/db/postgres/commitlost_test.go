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

// holdAt holds the server's replies once a statement starting with at is sent,
// by Query (a save's) or Exec (a run's), so it lands and its answer does not.
type holdAt struct {
	db.Conn
	p  *db.Proxy
	at string
}

func (h holdAt) Query(ctx context.Context, sql string, args ...db.Val) (db.Result, error) {
	if strings.HasPrefix(sql, h.at) {
		h.p.HoldReplies(true)
	}
	return h.Conn.Query(ctx, sql, args...)
}

func (h holdAt) Exec(ctx context.Context, sql string, maxRows int) ([]db.Result, error) {
	if strings.HasPrefix(sql, h.at) {
		h.p.HoldReplies(true)
	}
	return h.Conn.Exec(ctx, sql, maxRows)
}

func (h holdAt) TxStatus() byte { return db.TxStatus(h.Conn) }

// commitLost runs f on a Worker whose connection goes with the reply of the
// COMMIT f sends, once it has landed on the server, and says how many rows
// zz_cl has by then (F13.1, §8.2).
func commitLost(t *testing.T, at string, f func(ctx context.Context, w *db.Worker, schema string) error) (committed string, err error) {
	t.Helper()
	ctx := context.Background()
	dsn := OwnDB(t)
	own, _, err := Connect(ctx, dsn, "", false)
	if err != nil {
		t.Fatal(err)
	}
	defer own.Close()
	if _, err := own.Exec(ctx, "create table zz_cl (id serial primary key, v text)", 0); err != nil {
		t.Fatal(err)
	}
	if at != "commit" {
		if _, err := own.Exec(ctx, "insert into zz_cl (v) values ('y')", 0); err != nil {
			t.Fatal(err)
		}
	}
	p, via := proxied(t, dsn)
	c, _, err := Connect(ctx, via, "", false)
	if err != nil {
		t.Fatal(err)
	}
	w := db.NewWorker(holdAt{c, p, at})
	w.Redial = func(ctx context.Context) (db.Conn, error) { c, _, err := Connect(ctx, dsn, "", false); return c, err }
	w.Broken = Engine{}.Broken
	defer w.Close()
	done := make(chan error, 1)
	go func() { done <- f(ctx, w, "public") }()
	if at == "commit" {
		for range 100 { // the COMMIT has landed
			if r, err := own.Query(ctx, "select count(*) from zz_cl"); err == nil && r.Rows[0][0].S == "1" {
				break
			}
			time.Sleep(50 * time.Millisecond)
		}
	} else { // an uncommitted statement shows nowhere else: give it time to land
		time.Sleep(500 * time.Millisecond)
	}
	p.Cut(true)
	err = <-done
	r, qerr := own.Query(ctx, "select count(*) from zz_cl")
	if qerr != nil {
		t.Fatal(qerr)
	}
	return r.Rows[0][0].S, err
}

// A save in a transaction of its own whose COMMIT reached the server and its
// reply was lost with the connection says the result is not known, not a plain
// error that reads as rolled back; so does a COMMIT a run typed (F13.1).
func TestCommitLostAfterSent(t *testing.T) {
	rows, err := commitLost(t, "commit", func(ctx context.Context, w *db.Worker, schema string) error {
		ch := db.Changes{Inserts: [][]db.Change{{{Name: "v", Val: db.Val{S: "x"}}}}}
		_, _, err := db.Save(ctx, w, Engine{}, schema, "zz_cl", []string{"id"}, ch, false)
		return err
	})
	if rows != "1" || !errors.Is(err, db.ErrCommitUnknown) {
		t.Errorf("save: %v, rows committed %s", err, rows)
	}
	rows, err = commitLost(t, "commit", func(ctx context.Context, w *db.Worker, _ string) error {
		return w.Run(ctx, func(ctx context.Context, c db.Conn) error {
			_, err := db.ExecEach(ctx, c, []string{"begin", "insert into zz_cl (v) values ('x')", "commit"}, 0, nil)
			return err
		})
	})
	var lost db.TxLost
	if rows != "1" || !errors.As(err, &lost) || !lost.Sent {
		t.Errorf("run: %v, rows committed %s", err, rows)
	}
}

// A save whose UPDATE loses the connection, in the transaction it began, is
// the error of the UPDATE, which the app shows as rolled back; nothing was
// committed (F13.1).
func TestSaveLostBeforeCommit(t *testing.T) {
	rows, err := commitLost(t, "update", func(ctx context.Context, w *db.Worker, schema string) error {
		ch := db.Changes{Updates: []db.Row{{Key: []string{"1"}, Cols: []db.Change{{Name: "v", Val: db.Val{S: "x"}, Old: db.Val{S: "y"}}}}}}
		_, _, err := db.Save(ctx, w, Engine{}, schema, "zz_cl", []string{"id"}, ch, false)
		return err
	})
	var lost db.TxLost
	if rows != "1" || err == nil || errors.Is(err, db.ErrCommitUnknown) || errors.Is(err, db.ErrNotCommitted) || errors.As(err, &lost) {
		t.Errorf("save: %v, rows %s", err, rows)
	}
}
