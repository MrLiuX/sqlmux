// Package redis is db.Conn over go-redis (tech-design §8.7): one connection,
// not a pool, so what a connection holds stays on it, running one command at a
// time for a db.Worker. Exec takes the lines of a console, Query one command
// with its arguments, and a reply is a db.Result; what the typed calls of the
// key tab need beyond them they take from Use.
package redis

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	goredis "github.com/redis/go-redis/v9"

	"github.com/kackerx/sqlmux/internal/db"
	"github.com/kackerx/sqlmux/internal/i18n"
)

// go-redis logs what it retries and what it drops to stderr, which is the
// terminal the program draws on: the errors it returns are all there is to say.
func init() { goredis.SetLogger(quiet{}) }

type quiet struct{}

func (quiet) Printf(context.Context, string, ...any) {}

// Conn is one connection to a server.
type Conn struct{ c *goredis.Client }

// NewConn is the connection of a client that something other than a server on the
// network answers, as exec's command is (§8.8).
func NewConn(c *goredis.Client) *Conn { return &Conn{c: c} }

// Driver is db.Driver for Redis (§5「连接缝」): no Engine, nothing in it is
// SQL.
type Driver struct{}

func (Driver) Connect(ctx context.Context, dsn, password string, readOnly bool) (db.Conn, db.Info, error) {
	c, info, err := Connect(ctx, dsn, password, readOnly)
	if err != nil {
		return nil, info, err
	}
	return c, info, nil
}

// connectTimeout bounds the dial and the first commands, as the SQL drivers'.
const connectTimeout = 10 * time.Second

// options are what go-redis connects with for the server dsn, a redis:// or
// rediss:// URL that it reads (the db in its path, the user and password in it);
// a password that is not "" wins over the URL's. The commands ask for RESP3. The
// connection is one, not a pool; it has no retries, which would run a write
// twice (the Worker decides what a lost connection comes to, §8.2), and no
// timeout on reads, the blocking commands being the user's to cancel.
func options(ctx context.Context, dsn, password string) (*goredis.Options, error) {
	opt, err := goredis.ParseURL(dsn)
	if err != nil {
		return nil, err
	}
	if password != "" {
		opt.Password = password
	}
	opt.PoolSize, opt.MaxRetries, opt.DialerRetries = 1, -1, 1
	opt.DialTimeout, opt.ReadTimeout, opt.WriteTimeout = connectTimeout, -1, -1
	opt.ClientName = "sqlmux"
	if d, ok := db.DialerOf(ctx); ok { // through ssh (F15.2)
		opt.Dialer = d.Dial
	}
	return opt, nil
}

// Connect opens one connection to the server dsn, and says where it is. readOnly
// has no server side (§8.7): the app keeps back what writes (§13).
func Connect(ctx context.Context, dsn, password string, _ bool) (*Conn, db.Info, error) {
	opt, err := options(ctx, dsn, password)
	if err != nil {
		return nil, db.Info{}, err
	}
	c := &Conn{c: goredis.NewClient(opt)}
	ctx, cancel := context.WithTimeout(ctx, connectTimeout)
	defer cancel()
	text, err := c.c.Info(ctx, "server").Result()
	if err != nil {
		c.Close()
		return nil, db.Info{}, err
	}
	if err := checkVersion(text); err != nil {
		c.Close()
		return nil, db.Info{}, err
	}
	addr := opt.Addr
	if opt.Username != "" {
		addr = opt.Username + "@" + addr
	}
	return c, db.Info{Addr: addr, Database: strconv.Itoa(opt.DB)}, nil
}

// WithDB is dsn naming db n instead of the one it names: the path of the URL,
// its query's db dropped, which go-redis reads as well. Each db of a server is
// a connection of its own, the way each database of a PG server is (§5).
func WithDB(dsn, n string) (string, error) {
	u, err := url.Parse(dsn)
	if err != nil {
		return "", err
	}
	q := u.Query()
	q.Del("db")
	u.Path, u.RawQuery = "/"+n, q.Encode()
	return u.String(), nil
}

// Version is the redis_version of the text INFO server answers.
func Version(info string) string { return infoField(info, "redis_version") }

// checkVersion is an error when the INFO text tells of a server before 6, which
// has no RESP3 and no ACL users to speak of (§8.7).
func checkVersion(info string) error {
	server := infoField(info, "redis_version")
	if major, _, _ := strings.Cut(server, "."); atoi(major) < 6 {
		return fmt.Errorf("%s", i18n.T("需要 Redis 6.0 以上，这个服务器是 %s", server))
	}
	return nil
}

// infoField is the value of name in the text INFO answers, "" if it is not there.
func infoField(info, name string) string {
	for l := range strings.SplitSeq(info, "\n") {
		if v, ok := strings.CutPrefix(strings.TrimSpace(l), name+":"); ok {
			return v
		}
	}
	return ""
}

func atoi(s string) int { n, _ := strconv.Atoi(s); return n }

// Use runs f on c's client, a Conn a Worker gave a request, as one call that a
// cancel of ctx ends: go-redis cannot stop a command already sent, a BLPOP with
// no timeout, so the cancel closes the connection, and the next request finds
// it closed (Driver.Broken: nothing was sent) and the Worker dials again (§8.7). It is the way the typed calls of the key tab go
// to the server.
func Use(ctx context.Context, c db.Conn, f func(*goredis.Client) error) error {
	r := Of(c)
	stop := context.AfterFunc(ctx, func() { r.Close() })
	defer stop()
	return f(r.c)
}

// Of is the Conn behind c, which a Worker wraps for counting.
func Of(c db.Conn) *Conn {
	for {
		switch v := c.(type) {
		case *Conn:
			return v
		case interface{ Unwrap() db.Conn }:
			c = v.Unwrap()
		default:
			panic(fmt.Sprintf("redis: %T is no redis connection", c))
		}
	}
}

func (c *Conn) Close() error { return c.c.Close() }

// Query runs the command cmd with args, every one of them a string (Val.Null
// does not come into it), and lays its reply out as a table (§11「Redis 控制台」).
func (c *Conn) Query(ctx context.Context, cmd string, args ...db.Val) (db.Result, error) {
	words := make([]any, 0, 1+len(args))
	words = append(words, cmd)
	for _, a := range args {
		words = append(words, a.S)
	}
	return c.run(ctx, words, 0)
}

// Exec runs the lines of text as commands, one at a time, each a Result; it stops at
// the first that fails (§9.8). A line that is empty or a comment is none.
func (c *Conn) Exec(ctx context.Context, text string, maxRows int) ([]db.Result, error) {
	var out []db.Result
	for line := range strings.SplitSeq(text, "\n") {
		words, err := SplitLine(line)
		if err != nil {
			return out, err
		}
		if len(words) == 0 {
			continue
		}
		args := make([]any, len(words))
		for i, w := range words {
			args[i] = w
		}
		r, err := c.run(ctx, args, maxRows)
		if err != nil {
			return out, err
		}
		out = append(out, r)
	}
	return out, nil
}

// run sends one command and lays its reply out.
func (c *Conn) run(ctx context.Context, words []any, maxRows int) (db.Result, error) {
	start := time.Now()
	var reply any
	err := Use(ctx, c, func(cl *goredis.Client) (err error) {
		reply, err = cl.Do(ctx, words...).Result()
		return err
	})
	if err != nil && !errors.Is(err, goredis.Nil) {
		return db.Result{}, err
	}
	r := Layout(fmt.Sprint(words[0]), reply, maxRows)
	r.Took = time.Since(start)
	return r, nil
}
