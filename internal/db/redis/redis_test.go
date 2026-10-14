//go:build integration

package redis

import (
	"context"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/kackerx/sqlmux/internal/db"
)

// worker is a Worker of connections as the app dials them.
func worker(t *testing.T, dsn string) *db.Worker {
	t.Helper()
	c, _, err := Connect(context.Background(), dsn, "", false)
	if err != nil {
		t.Fatal(err)
	}
	w := db.NewWorker(c)
	w.Broken = Driver{}.Broken
	w.Redial = func(ctx context.Context) (db.Conn, error) {
		c, _, err := Connect(ctx, dsn, "", false)
		return c, err
	}
	t.Cleanup(func() { w.Close() })
	return w
}

// A connection says where it is, runs commands whose replies are tables, and
// its db is the URL's: the db of a child connection (F16.1, §5).
func TestConnect(t *testing.T) {
	dsn := IntegrationDSN(t)
	ctx := context.Background()
	c, info, err := Connect(ctx, dsn, "", false)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	u, _ := url.Parse(dsn)
	if info.Database != "0" || !strings.HasSuffix(info.Addr, u.Host) {
		t.Errorf("info %+v for %s", info, dsn)
	}
	if r, err := c.Query(ctx, "PING"); err != nil || len(r.Rows) != 1 || r.Rows[0][0].S != "PONG" {
		t.Errorf("PING: %+v %v", r, err)
	}
	if r, err := c.Query(ctx, "GET", db.Val{S: "str:plain"}); err != nil || r.Rows[0][0].S != "hello" {
		t.Errorf("GET: %+v %v", r, err)
	}
	if r, err := c.Query(ctx, "GET", db.Val{S: "no:such:key"}); err != nil || !r.Rows[0][0].Null {
		t.Errorf("a nil: %+v %v", r, err)
	}
	if r, err := c.Query(ctx, "HGETALL", db.Val{S: "hash:user"}); err != nil || len(r.Rows) != 4 || r.Cols[0].Name != "field" {
		t.Errorf("a map: %+v %v", r, err)
	}
	if r, err := c.Query(ctx, "GET", db.Val{S: "str:binary"}); err != nil || !db.IsBinaryShown(r.Rows[0][0].S) {
		t.Errorf("bytes: %+v %v", r, err)
	}
	// an error of the server is an error, with the server's words
	_, err = c.Query(ctx, "HGET", db.Val{S: "str:plain"}, db.Val{S: "f"})
	if e := (Driver{}).ErrorOf(err); e.Code != "WRONGTYPE" || (Driver{}).Broken(err) != db.NotLost {
		t.Errorf("a wrong type: %v %+v", err, e)
	}
	// Exec: the lines of a console, each a result; a comment and a blank are none, the first that fails stops it
	rs, err := c.Exec(ctx, "# nothing\n\nPING\nGET str:plain\nNOSUCHCOMMAND x\nPING", 0)
	if len(rs) != 2 || err == nil || !strings.Contains(err.Error(), "unknown command") {
		t.Errorf("exec: %d results, %v", len(rs), err)
	}
	if rs, err := c.Exec(ctx, `ECHO "two words"`, 0); err != nil || rs[0].Rows[0][0].S != "two words" {
		t.Errorf("a quoted word: %+v %v", rs, err)
	}
	if _, err := c.Exec(ctx, `ECHO "open`, 0); err == nil {
		t.Error("an open quote is sent")
	}
	if rs, err := c.Exec(ctx, "LRANGE list:queue 0 -1", 2); err != nil || len(rs[0].Rows) != 2 || !rs[0].Truncated {
		t.Errorf("maxRows: %+v %v", rs, err)
	}
}

// A URL's db is the db of the connection, and the password that is given wins
// over the URL's; ACL users come in as themselves.
func TestConnectDBAndACL(t *testing.T) {
	dsn := IntegrationDSN(t)
	ctx := context.Background()
	u, _ := url.Parse(dsn)
	u.Path = "/3"
	c, info, err := Connect(ctx, u.String(), "", false)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if r, err := c.Query(ctx, "CLIENT", db.Val{S: "INFO"}); err != nil || !strings.Contains(r.Rows[0][0].S, "db=3") || info.Database != "3" {
		t.Errorf("db 3: %+v %v %+v", r.Rows, err, info)
	}
	admin, _, err := Connect(ctx, dsn, "", false)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { admin.Close() }) // after the user is deleted below
	if _, err := admin.Query(ctx, "ACL", db.Val{S: "SETUSER"}, db.Val{S: "sqlmuxacl"}, db.Val{S: "on"}, db.Val{S: ">secret"}, db.Val{S: "~acl:*"}, db.Val{S: "+get"}, db.Val{S: "+ping"}, db.Val{S: "+hello"}, db.Val{S: "+info"}, db.Val{S: "+client|setname"}, db.Val{S: "+client|setinfo"}); err != nil {
		t.Skip("no ACL: ", err)
	}
	t.Cleanup(func() { _, _ = admin.Query(ctx, "ACL", db.Val{S: "DELUSER"}, db.Val{S: "sqlmuxacl"}) })
	as := func(userinfo string) string {
		v := *u
		v.User = nil
		v.Path = "/0"
		return strings.Replace(v.String(), "redis://", "redis://"+userinfo+"@", 1)
	}
	if c, info, err := Connect(ctx, as("sqlmuxacl:secret"), "", false); err != nil || info.Addr != "sqlmuxacl@"+u.Host {
		t.Errorf("the ACL user: %v %+v", err, info)
	} else {
		if _, err := c.Query(ctx, "SET", db.Val{S: "acl:x"}, db.Val{S: "1"}); err == nil || (Driver{}).ErrorOf(err).Code != "NOPERM" {
			t.Errorf("a command it may not run: %v", err)
		}
		c.Close()
	}
	if c, _, err := Connect(ctx, as("sqlmuxacl:wrong"), "", false); err == nil {
		c.Close()
		t.Error("a wrong password connected")
	}
	if c, _, err := Connect(ctx, as("sqlmuxacl:wrong"), "secret", false); err != nil { // the given password wins
		t.Errorf("the password given: %v", err)
	} else {
		c.Close()
	}
}

// A BLPOP with no timeout blocks the connection, which go-redis cannot cancel:
// the cancel closes it, the request ends at once, and the next one goes on a
// connection dialed again (§8.7, F16.1).
func TestCancelBlockingCommand(t *testing.T) {
	w := worker(t, OwnDB(t))
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err := w.Query(ctx, "BLPOP", db.Val{S: "nolist"}, db.Val{S: "0"})
	if err == nil || time.Since(start) > 2*time.Second {
		t.Fatalf("BLPOP ended after %v with %v", time.Since(start), err)
	}
	if r, err := w.Query(context.Background(), "PING"); err != nil || r.Rows[0][0].S != "PONG" {
		t.Errorf("after the cancel: %+v %v", r, err)
	}
	// Cancel() too, as C-c does
	done := make(chan error, 1)
	go func() {
		_, err := w.Query(context.Background(), "BLPOP", db.Val{S: "nolist"}, db.Val{S: "0"})
		done <- err
	}()
	time.Sleep(200 * time.Millisecond)
	w.Cancel()
	select {
	case err := <-done:
		if err == nil {
			t.Error("a cancelled BLPOP gave a reply")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("C-c did not end BLPOP")
	}
	if r, err := w.Query(context.Background(), "PING"); err != nil || r.Rows[0][0].S != "PONG" {
		t.Errorf("after C-c: %+v %v", r, err)
	}
}

// The connection is lost, the server killing it: the next request dials again
// and runs (F8.19).
func TestRedialAfterKill(t *testing.T) {
	dsn := OwnDB(t)
	w := worker(t, dsn)
	if _, err := w.Query(context.Background(), "CLIENT", db.Val{S: "SETNAME"}, db.Val{S: "victim"}); err != nil {
		t.Fatal(err)
	}
	admin, _, err := Connect(context.Background(), dsn, "", false)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	if _, err := admin.Query(context.Background(), "CLIENT", db.Val{S: "KILL"}, db.Val{S: "USER"}, db.Val{S: "default"}, db.Val{S: "SKIPME"}, db.Val{S: "yes"}); err != nil {
		t.Fatal(err)
	}
	if r, err := w.Query(context.Background(), "PING"); err != nil || r.Rows[0][0].S != "PONG" {
		t.Errorf("after the kill: %+v %v", r, err)
	}
}

// Through an ssh the server is reached by its stdin and stdout, and the name of
// its host is the other end's to look up (F15.2, F16.1).
func TestConnectThroughSSH(t *testing.T) {
	dsn := IntegrationDSN(t)
	args := db.NCSSH(t)
	u, _ := url.Parse(dsn)
	if u.Hostname() != "localhost" {
		t.Skip("the test instance is not on localhost")
	}
	u.Host = "redis.internal.invalid:" + u.Port()
	ctx := db.WithDialer(context.Background(), db.SSHDialer("me@jump"))
	c, _, err := Connect(ctx, u.String(), "", false)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if r, err := c.Query(ctx, "PING"); err != nil || r.Rows[0][0].S != "PONG" {
		t.Fatalf("%+v %v", r, err)
	}
	b, _ := os.ReadFile(args)
	if !strings.Contains(string(b), "-W redis.internal.invalid:") || !strings.HasSuffix(strings.TrimSpace(string(b)), "me@jump") {
		t.Errorf("ssh was run as %q", b)
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
