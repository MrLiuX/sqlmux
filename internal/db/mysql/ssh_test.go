//go:build integration

package mysql

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/kackerx/sqlmux/internal/db"
)

// MySQL through an ssh: the host is a name only the other end knows, queries
// run, and a cancel (a KILL QUERY on another connection) goes the same way (F15.2).
func TestConnectThroughSSH(t *testing.T) {
	dsn := IntegrationDSN(t)
	args := db.NCSSH(t)
	dsn = strings.Replace(dsn, "localhost", "db.internal.invalid", 1)
	if !strings.Contains(dsn, "db.internal.invalid") {
		t.Skip("the test instance is not on localhost")
	}
	ctx := db.WithDialer(context.Background(), db.SSHDialer("me@jump"))
	c, _, err := Connect(ctx, dsn, "", false)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	r, err := c.Query(ctx, "select 1")
	if err != nil || r.Rows[0][0].S != "1" {
		t.Fatalf("%+v %v", r, err)
	}
	if b, _ := os.ReadFile(args); !strings.Contains(string(b), "-W db.internal.invalid:") || !strings.HasSuffix(strings.TrimSpace(string(b)), "me@jump") {
		t.Errorf("ssh was run as %q", b)
	}
	cctx, cancel := context.WithTimeout(ctx, 300*time.Millisecond)
	defer cancel()
	start := time.Now()
	if _, err := c.Query(cctx, "select sleep(30)"); time.Since(start) > 10*time.Second {
		t.Errorf("the sleep was not cancelled: %v after %v", err, time.Since(start))
	}
	if r, err := c.Query(ctx, "select 2"); err != nil || r.Rows[0][0].S != "2" {
		t.Errorf("after the cancel: %+v %v", r, err)
	}
}

// With the ssh failing, the error is what it said.
func TestConnectThroughSSHFails(t *testing.T) {
	dsn := IntegrationDSN(t)
	db.FakeSSH(t, "echo 'me@jump: Permission denied (publickey).' >&2; exit 255")
	ctx := db.WithDialer(context.Background(), db.SSHDialer("me@jump"))
	if _, _, err := Connect(ctx, dsn, "", false); err == nil || !strings.Contains(err.Error(), "Permission denied (publickey).") {
		t.Fatalf("%v", err)
	}
}

// An ssh that is up and never answers is given up on at the DSN's timeout, as
// the dial of one with no ssh is, and the error says that, not that we killed
// the ssh (F15.2).
func TestConnectThroughSSHTimeout(t *testing.T) {
	db.FakeSSH(t, "exec sleep 8")
	ctx := db.WithDialer(context.Background(), db.SSHDialer("me@jump"))
	start := time.Now()
	_, _, err := Connect(ctx, "u:p@tcp(db.invalid:3306)/d?timeout=1s", "", false)
	if took := time.Since(start); err == nil || took > 4*time.Second || !strings.Contains(err.Error(), "deadline exceeded") || strings.Contains(err.Error(), "killed") {
		t.Fatalf("%v after %v", err, took)
	}
}

// Connections through the same machine made at once each have their ssh's words
// for an error: the driver dials by a name it keeps for the process, so each
// connection's dialer is the one in its own ctx (F15.2).
func TestConnectThroughSSHConcurrent(t *testing.T) {
	db.FakeSSH(t, "sleep 0.2; echo 'me@jump: Permission denied (publickey).' >&2; exit 255")
	for range 4 {
		errs := make(chan error, 4)
		for range 4 {
			go func() {
				ctx := db.WithDialer(context.Background(), db.SSHDialer("me@jump"))
				_, _, err := Connect(ctx, "u:p@tcp(db.invalid:3306)/d", "", false)
				errs <- err
			}()
		}
		for range 4 {
			if err := <-errs; err == nil || !strings.Contains(err.Error(), "Permission denied (publickey).") {
				t.Fatalf("%v", err)
			}
		}
	}
}
