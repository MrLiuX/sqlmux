package redis

import (
	"context"
	"net"
	"strconv"
	"strings"
	"testing"

	"github.com/kackerx/sqlmux/internal/db"
)

// A connection is one, with no retries (a write would run twice), no timeout
// on a read (a blocking command is the user's to cancel), the db and the user
// of the URL, a password given over the URL's, and the dialer of an ssh (F16.1).
func TestOptions(t *testing.T) {
	ctx := context.Background()
	opt, err := options(ctx, "redis://ann:fromurl@example.com:6380/3", "")
	if err != nil {
		t.Fatal(err)
	}
	if opt.PoolSize != 1 || opt.MaxRetries != -1 || opt.DialerRetries != 1 || opt.ReadTimeout != -1 || opt.WriteTimeout != -1 || opt.DialTimeout != connectTimeout {
		t.Errorf("one connection, no retries, no read timeout: %+v", opt)
	}
	if opt.DB != 3 || opt.Username != "ann" || opt.Password != "fromurl" || opt.Addr != "example.com:6380" || opt.Dialer != nil {
		t.Errorf("the URL's: %+v", opt)
	}
	if opt, _ = options(ctx, "redis://ann:fromurl@example.com/0", "given"); opt.Password != "given" {
		t.Errorf("the password given wins: %q", opt.Password)
	}
	if _, err := options(ctx, "mysql://x", ""); err == nil {
		t.Error("a URL that is no redis one")
	}
	d := db.Dialer{Dial: func(context.Context, string, string) (net.Conn, error) { return nil, net.ErrClosed }}
	if opt, _ = options(db.WithDialer(ctx, d), "rediss://example.com", ""); opt.Dialer == nil || opt.TLSConfig == nil {
		t.Errorf("through an ssh, and TLS for rediss: %+v", opt)
	}
}

// A server before 6 is turned away with its version.
func TestCheckVersion(t *testing.T) {
	for _, c := range []struct {
		info string
		ok   bool
	}{
		{"# Server\r\nredis_version:7.4.0\r\nredis_mode:standalone\r\n", true},
		{"redis_version:6.0.0\r\n", true},
		{"redis_version:5.0.7\r\n", false},
		{"redis_version:10.1.0\r\n", true},
		{"nothing", false},
	} {
		if err := checkVersion(c.info); (err == nil) != c.ok || err != nil && !strings.Contains(err.Error(), infoField(c.info, "redis_version")) {
			t.Errorf("%q: %v", c.info, err)
		}
	}
}

// A db of a server is the URL with another path, its query's db gone (F16.1, §5).
func TestWithDB(t *testing.T) {
	for _, c := range []struct{ dsn, n, want string }{
		{"redis://localhost:6379/0", "3", "redis://localhost:6379/3"},
		{"redis://u:p@h/0?db=5&dial_timeout=3s", "7", "redis://u:p@h/7?dial_timeout=3s"},
		{"rediss://h:1", "2", "rediss://h:1/2"},
	} {
		got, err := WithDB(c.dsn, c.n)
		if err != nil || got != c.want {
			t.Errorf("%s %s: %q %v, want %q", c.dsn, c.n, got, err, c.want)
		}
		if opt, err := options(context.Background(), got, ""); err != nil || strconv.Itoa(opt.DB) != c.n {
			t.Errorf("%s: db %d %v", got, opt.DB, err)
		}
	}
	if _, err := WithDB("redis://[::1", "1"); err == nil {
		t.Error("a URL that is no URL")
	}
}
