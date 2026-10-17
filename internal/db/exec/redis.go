package exec

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	goredis "github.com/redis/go-redis/v9"

	"github.com/kackerx/sqlmux/internal/db"
	"github.com/kackerx/sqlmux/internal/db/redis"
)

// RedisDriver is the Redis driver over a command (§8.8, F17.8): every typed call of
// the key tab, the tree and the console is go-redis's, and what answers go-redis is
// not a server but this package: it reads the commands off a pipe as RESP2, runs
// the command for each one, and writes what it printed back as the reply it has the
// shape of. The command's template has {cmd} in an argument for the command's name,
// and {args...} as an argument of its own for the rest of its words. One db, 0:
// SELECT, AUTH and the like are answered here, and INFO keyspace says db0.
type RedisDriver struct{ Command []string }

func (d RedisDriver) Connect(ctx context.Context, _, _ string, _ bool) (db.Conn, db.Info, error) {
	if len(d.Command) == 0 {
		return nil, db.Info{}, errors.New("command is empty")
	}
	s := &respServer{argv: d.Command}
	cl := goredis.NewClient(&goredis.Options{
		Addr: "exec", Protocol: 2, PoolSize: 1, MaxRetries: -1, DialerRetries: 1,
		DialTimeout: 10 * time.Second, ReadTimeout: -1, WriteTimeout: -1, DisableIdentity: true,
		Dialer: func(context.Context, string, string) (net.Conn, error) {
			a, b := net.Pipe()
			go s.serve(b)
			return a, nil
		},
	})
	c := redis.NewConn(cl)
	// one run of the command tells it works
	if err := cl.Ping(ctx).Err(); err != nil {
		c.Close()
		return nil, db.Info{}, err
	}
	return c, db.Info{Addr: filepath.Base(d.Command[0]), Database: "0"}, nil
}

func (RedisDriver) Broken(err error) db.Lost         { return redis.Driver{}.Broken(err) }
func (RedisDriver) ErrorOf(err error) db.ServerError { return redis.Driver{}.ErrorOf(err) }

// respServer answers the client on one end of a pipe by running the command.
type respServer struct{ argv []string }

// serve answers each request in turn; a read that fails, the client having closed
// the pipe, cancels the run that is out.
func (s *respServer) serve(conn net.Conn) {
	defer conn.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	reqs := make(chan []string)
	go func() {
		defer cancel()
		defer close(reqs)
		r := bufio.NewReader(conn)
		for {
			req, err := readRequest(r)
			if err != nil {
				return
			}
			select {
			case reqs <- req:
			case <-ctx.Done():
				return
			}
		}
	}()
	w := bufio.NewWriter(conn)
	for req := range reqs {
		s.answer(ctx, w, req)
		if w.Flush() != nil {
			return
		}
	}
}

// readRequest is a command of the client: an array of bulk strings.
func readRequest(r *bufio.Reader) ([]string, error) {
	line, err := r.ReadString('\n')
	if err != nil {
		return nil, err
	}
	n, err := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(line, "*")))
	if err != nil || !strings.HasPrefix(line, "*") || n < 0 {
		return nil, fmt.Errorf("not a command: %q", line)
	}
	words := make([]string, n)
	for i := range words {
		head, err := r.ReadString('\n')
		if err != nil {
			return nil, err
		}
		size, err := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(head, "$")))
		if err != nil || !strings.HasPrefix(head, "$") || size < 0 {
			return nil, fmt.Errorf("not a word: %q", head)
		}
		buf := make([]byte, size+2)
		if _, err := io.ReadFull(r, buf); err != nil {
			return nil, err
		}
		words[i] = string(buf[:size])
	}
	return words, nil
}

// answer writes the reply to req: what the connection says for itself here, or
// what the command printed.
func (s *respServer) answer(ctx context.Context, w *bufio.Writer, req []string) {
	if len(req) == 0 {
		writeError(w, "empty command")
		return
	}
	name := strings.ToUpper(req[0])
	switch {
	case name == "CLIENT" || name == "AUTH" || name == "READONLY":
		put(w, "+OK\r\n")
	case name == "HELLO": // go-redis's first: RESP2, a server that says little of itself
		writeValue(w, []any{"server", "exec", "version", "0", "proto", json.Number("2"), "id", json.Number("1"), "mode", "standalone", "role", "master", "modules", []any{}})
	case name == "SELECT":
		if len(req) == 2 && req[1] == "0" {
			put(w, "+OK\r\n")
		} else {
			writeError(w, "an exec connection has db 0 only")
		}
	case name == "INFO" && len(req) == 2 && strings.EqualFold(req[1], "keyspace"):
		writeBulk(w, "# Keyspace\r\ndb0:keys=0,expires=0,avg_ttl=0\r\n")
	default:
		v, err := s.run(ctx, req)
		if err != nil {
			writeError(w, err.Error())
			return
		}
		writeValue(w, v)
	}
}

// run is one run of the command for req, its output as the value it holds.
func (s *respServer) run(ctx context.Context, req []string) (any, error) {
	var args []string
	for _, a := range s.argv {
		switch {
		case a == "{args...}":
			args = append(args, req[1:]...)
		default:
			args = append(args, strings.ReplaceAll(a, "{cmd}", req[0]))
		}
	}
	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	var out bytes.Buffer
	var errOut tail
	cmd.Stdout, cmd.Stderr = &out, &errOut
	cmd.WaitDelay = 500 * time.Millisecond
	err := cmd.Run()
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if err != nil {
		if msg := strings.TrimSpace(errOut.buf.String()); msg != "" {
			return nil, errors.New(msg)
		}
		return nil, fmt.Errorf("%s: %w", args[0], err)
	}
	dec := json.NewDecoder(&out)
	dec.UseNumber()
	v, err := ordered(dec)
	if err != nil {
		return nil, fmt.Errorf("the command did not print JSON: %w", err)
	}
	return v, nil
}

// ordered reads the next JSON value, an object as the array of its keys and values
// in the order they were written, which is how RESP2 has a map.
func ordered(dec *json.Decoder) (any, error) {
	tok, err := dec.Token()
	if err != nil {
		return nil, err
	}
	d, ok := tok.(json.Delim)
	if !ok {
		return tok, nil
	}
	var out []any
	for dec.More() {
		if d == '{' {
			k, err := dec.Token()
			if err != nil {
				return nil, err
			}
			out = append(out, k)
		}
		v, err := ordered(dec)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	if _, err := dec.Token(); err != nil { // the closing bracket
		return nil, err
	}
	if out == nil {
		out = []any{}
	}
	return out, nil
}

// put writes s: a bufio.Writer keeps its first error, which the Flush after the reply says.
func put(w *bufio.Writer, s string) { _, _ = w.WriteString(s) }

func writeError(w *bufio.Writer, msg string) {
	fmt.Fprintf(w, "-ERR %s\r\n", strings.NewReplacer("\r", " ", "\n", " ").Replace(msg))
}

func writeBulk(w *bufio.Writer, s string) { fmt.Fprintf(w, "$%d\r\n%s\r\n", len(s), s) }

// writeValue writes v, a value of ordered, as RESP2: null nil, a string a bulk string, an
// integer an integer, any other number its text, a boolean 1 or 0, an array an array.
func writeValue(w *bufio.Writer, v any) {
	switch v := v.(type) {
	case nil:
		put(w, "$-1\r\n")
	case string:
		writeBulk(w, v)
	case json.Number:
		if _, err := strconv.ParseInt(v.String(), 10, 64); err == nil {
			fmt.Fprintf(w, ":%s\r\n", v)
		} else {
			writeBulk(w, v.String())
		}
	case bool:
		put(w, map[bool]string{true: ":1\r\n", false: ":0\r\n"}[v])
	case []any:
		fmt.Fprintf(w, "*%d\r\n", len(v))
		for _, e := range v {
			writeValue(w, e)
		}
	}
}
