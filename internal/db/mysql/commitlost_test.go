//go:build integration

package mysql

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kackerx/sqlmux/internal/db"
)

// holdAtCommit holds the server's replies once the COMMIT is sent, by Query
// (a save's) or Exec (a run's), so it lands and its answer does not.
type holdAtCommit struct {
	db.Conn
	p *db.Proxy
}

func (h holdAtCommit) Query(ctx context.Context, sql string, args ...db.Val) (db.Result, error) {
	if sql == "commit" {
		h.p.HoldReplies(true)
	}
	return h.Conn.Query(ctx, sql, args...)
}

func (h holdAtCommit) Exec(ctx context.Context, sql string, maxRows int) ([]db.Result, error) {
	if sql == "commit" {
		h.p.HoldReplies(true)
	}
	return h.Conn.Exec(ctx, sql, maxRows)
}

func (h holdAtCommit) TxStatus() byte { return db.TxStatus(h.Conn) }
func (h holdAtCommit) Closed() bool   { return h.Conn.(*Conn).Closed() }

// commitLost runs f on a Worker whose connection goes with the reply of the
// COMMIT f sends, once it has landed on the server, and says how many rows
// zz_cl has by then (F13.1, §8.2).
func commitLost(t *testing.T, f func(ctx context.Context, w *db.Worker, schema string) error) (committed string, err error) {
	t.Helper()
	ctx := context.Background()
	dsn := OwnDB(t)
	own, _, err := Connect(ctx, dsn, "", false)
	if err != nil {
		t.Fatal(err)
	}
	defer own.Close()
	if _, err := own.Exec(ctx, "create table zz_cl (id int auto_increment primary key, v text)", 0); err != nil {
		t.Fatal(err)
	}
	r, err := own.Query(ctx, "select database()")
	if err != nil {
		t.Fatal(err)
	}
	schema := r.Rows[0][0].S
	p, via := proxied(t, dsn)
	c, _, err := Connect(ctx, via, "", false)
	if err != nil {
		t.Fatal(err)
	}
	w := db.NewWorker(holdAtCommit{c, p})
	w.Redial = func(ctx context.Context) (db.Conn, error) { c, _, err := Connect(ctx, dsn, "", false); return c, err }
	w.Broken = Engine{}.Broken
	defer w.Close()
	done := make(chan error, 1)
	go func() { done <- f(ctx, w, schema) }()
	for range 100 { // the COMMIT has landed
		if r, err := own.Query(ctx, "select count(*) from zz_cl"); err == nil && r.Rows[0][0].S == "1" {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	p.Cut(true)
	err = <-done
	n, qerr := own.Query(ctx, "select count(*) from zz_cl")
	if qerr != nil {
		t.Fatal(qerr)
	}
	return n.Rows[0][0].S, err
}

// A save in a transaction of its own whose COMMIT reached the server and its
// reply was lost with the connection says the result is not known, not a plain
// error that reads as rolled back; so does a COMMIT a run typed (F13.1).
func TestCommitLostAfterSent(t *testing.T) {
	rows, err := commitLost(t, func(ctx context.Context, w *db.Worker, schema string) error {
		ch := db.Changes{Inserts: [][]db.Change{{{Name: "v", Val: db.Val{S: "x"}}}}}
		_, _, err := db.Save(ctx, w, Engine{}, schema, "zz_cl", []string{"id"}, ch, false)
		return err
	})
	if rows != "1" || !errors.Is(err, db.ErrCommitUnknown) {
		t.Errorf("save: %v, rows committed %s", err, rows)
	}
	rows, err = commitLost(t, func(ctx context.Context, w *db.Worker, _ string) error {
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
