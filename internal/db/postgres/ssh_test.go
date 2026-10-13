//go:build integration

package postgres

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/kackerx/sqlmux/internal/db"
)

// Through an ssh the server is reached by its stdin and stdout, the name of
// its host looked up by the other end: one no resolver here knows is dialed
// all the same, the ssh gets the address, and queries run (F15.2).
func TestConnectThroughSSH(t *testing.T) {
	dsn := IntegrationDSN(t)
	args := db.NCSSH(t)
	dsn = strings.Replace(dsn, "localhost", "db.internal.invalid", 1)
	if !strings.Contains(dsn, "db.internal.invalid") {
		t.Skip("the test instance is not on localhost")
	}
	ctx := db.WithDialer(context.Background(), db.SSHDialer("me@jump"))
	c, info, err := Connect(ctx, dsn, "", false)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	r, err := c.Query(ctx, "select 1, current_database()")
	if err != nil || r.Rows[0][0].S != "1" || r.Rows[0][1].S != info.Database {
		t.Fatalf("%+v %v", r, err)
	}
	if !strings.Contains(readArgs(t, args), "-W db.internal.invalid:") || !strings.HasSuffix(strings.TrimSpace(readArgs(t, args)), "me@jump") {
		t.Errorf("ssh was run as %q", readArgs(t, args))
	}
	// a cancel makes another connection, through the ssh as well
	cctx, cancel := context.WithCancel(ctx)
	go cancel()
	if _, err := c.Query(cctx, "select pg_sleep(30)"); err == nil {
		t.Error("the sleep ended without a cancel")
	}
	if r, err := c.Query(ctx, "select 2"); err != nil || r.Rows[0][0].S != "2" {
		t.Errorf("after the cancel: %+v %v", r, err)
	}
}

// Without the ssh saying yes the error is what it said.
func TestConnectThroughSSHFails(t *testing.T) {
	dsn := IntegrationDSN(t)
	db.FakeSSH(t, "echo 'me@jump: Permission denied (publickey).' >&2; exit 255")
	ctx := db.WithDialer(context.Background(), db.SSHDialer("me@jump"))
	if _, _, err := Connect(ctx, dsn, "", false); err == nil || !strings.Contains(err.Error(), "Permission denied (publickey).") {
		t.Fatalf("%v", err)
	}
}

func readArgs(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
