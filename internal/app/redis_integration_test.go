//go:build integration

package app

import (
	"context"
	"strings"
	"testing"

	"github.com/kackerx/sqlmux/internal/config"
	"github.com/kackerx/sqlmux/internal/db"
	"github.com/kackerx/sqlmux/internal/db/redis"
	"github.com/kackerx/sqlmux/internal/i18n"
)

// A Redis connection is tried by the form's button with PING and says the
// server's version; it dials as any connection does, Main and Meta each,
// through the driver table with no engine; a db of the server is a connection
// of its own by the URL's path; and a Main the server killed dials again
// (F16.1).
func TestRedisConnection(t *testing.T) {
	ctx := context.Background()
	dsn := redis.IntegrationDSN(t)
	spec := config.Connection{Name: "cache", Engine: "redis", DSN: dsn}
	note, err := testConn(spec, "")
	if err != nil || !strings.Contains(note, "Redis ") || !strings.Contains(note, strings.SplitN(i18n.T("已连接 · %s · %s", "", ""), " ", 2)[0]) {
		t.Errorf("the test button: %q %v", note, err)
	}
	if _, err := testConn(config.Connection{Name: "x", Engine: "redis", DSN: "redis://127.0.0.1:1/0"}, ""); err == nil {
		t.Error("a server that is not there is tried")
	}
	a := opened(t, spec)
	c := a.sess.Conns[0]
	if c.State != connReady || c.eng != nil || c.drv == nil || c.DB != "0" || c.Addr == "" {
		t.Fatalf("connected: state %v eng %v drv %v db %q addr %q", c.State, c.eng, c.drv, c.DB, c.Addr)
	}
	for _, w := range []*db.Worker{c.Main, c.Meta} {
		if r, err := w.Query(ctx, "PING"); err != nil || r.Rows[0][0].S != "PONG" {
			t.Errorf("PING: %+v %v", r, err)
		}
	}
	// a db of its own
	child := spec
	child.Database = "3"
	d, err := dial(ctx, child)
	if err != nil {
		t.Fatal(err)
	}
	defer d.discard()()
	if r, err := d.meta.Query(ctx, "CLIENT", db.Val{S: "INFO"}); err != nil || !strings.Contains(r.Rows[0][0].S, "db=3") || d.info.Database != "3" {
		t.Errorf("db 3: %+v %v %+v", r.Rows, err, d.info)
	}
	// the server kills Main: the next request goes on a connection dialed again
	id, err := c.Main.Query(ctx, "CLIENT", db.Val{S: "ID"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Meta.Query(ctx, "CLIENT", db.Val{S: "KILL"}, db.Val{S: "ID"}, db.Val{S: id.Rows[0][0].S}); err != nil {
		t.Fatal(err)
	}
	if r, err := c.Main.Query(ctx, "PING"); err != nil || r.Rows[0][0].S != "PONG" {
		t.Errorf("after the kill: %+v %v", r, err)
	}
	// the error of a command is the server's
	_, err = c.Main.Query(ctx, "HGET", db.Val{S: "str:plain"}, db.Val{S: "f"})
	if e := c.drv.ErrorOf(err); e.Code != "WRONGTYPE" {
		t.Errorf("an error: %v %+v", err, e)
	}
}
