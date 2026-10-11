//go:build integration

package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/kackerx/sqlmux/internal/db"
)

// holdAtRollback holds the server's replies once a rollback is sent.
type holdAtRollback struct {
	db.Conn
	p *db.Proxy
}

func (h holdAtRollback) Query(ctx context.Context, sql string, args ...db.Val) (db.Result, error) {
	if sql == "rollback" {
		h.p.HoldReplies(true)
	}
	return h.Conn.Query(ctx, sql, args...)
}

// Quick's rollback, its reply held by a server that has stopped answering,
// does not hold Meta forever: it ends in db.End's time, so a cancel and a quit
// are not left waiting for it (F13.6).
func TestQuickRollbackIsBounded(t *testing.T) {
	p, via := proxied(t, IntegrationDSN(t))
	c, _, err := Connect(context.Background(), via, "", true)
	if err != nil {
		t.Fatal(err)
	}
	w := db.NewWorker(holdAtRollback{c, p})
	done := make(chan error, 1)
	go func() {
		_, err := Engine{}.Quick(context.Background(), w, "select 1", "public", 10)
		done <- err
	}()
	time.Sleep(time.Second)
	w.Cancel() // the user's C-c
	select {
	case <-done:
	case <-time.After(20 * time.Second):
		t.Fatal("Quick still holds Meta 20s after the cancel")
	}
	closed := make(chan struct{})
	go func() { w.Close(); close(closed) }() // what quitting does
	select {
	case <-closed:
	case <-time.After(5 * time.Second):
		t.Error("Worker.Close is blocked")
	}
	p.Cut(true)
}
