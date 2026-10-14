package redis

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"testing"

	goredis "github.com/redis/go-redis/v9"

	"github.com/kackerx/sqlmux/internal/db"
)

// redisError is what go-redis gives for an error reply of the server.
type redisError string

func (e redisError) Error() string { return string(e) }
func (redisError) RedisError()     {}

// What an error says is the server's first word and the rest, a lost connection
// is the network's or the client's own for a closed one, and what the server
// answers is none (§8.7).
func TestErrorOfAndBroken(t *testing.T) {
	d := Driver{}
	wrong := redisError("WRONGTYPE Operation against a key holding the wrong kind of value")
	if e := d.ErrorOf(fmt.Errorf("wrapped: %w", wrong)); e.Code != "WRONGTYPE" || e.Message != "Operation against a key holding the wrong kind of value" || e.Severity != "ERROR" {
		t.Errorf("a server's error: %+v", e)
	}
	if e := d.ErrorOf(io.EOF); e.Code != "" || e.Message != "EOF" {
		t.Errorf("any other: %+v", e)
	}
	dial := &net.OpError{Op: "dial", Net: "tcp", Err: errors.New("connection refused")}
	read := &net.OpError{Op: "read", Net: "tcp", Err: errors.New("connection reset by peer")}
	for _, c := range []struct {
		name string
		err  error
		want db.Lost
	}{
		{"WRONGTYPE", wrong, db.NotLost},
		{"LOADING", redisError("LOADING Redis is loading the dataset in memory"), db.NotLost},
		{"READONLY", redisError("READONLY You can't write against a read only replica."), db.NotLost},
		{"a cancel", context.Canceled, db.NotLost},
		{"a deadline", context.DeadlineExceeded, db.NotLost},
		{"a dial that was cancelled", &net.OpError{Op: "dial", Net: "tcp", Err: context.Canceled}, db.NotLost},
		{"a client that is closed", goredis.ErrClosed, db.LostUnsent},
		{"a dial that failed", fmt.Errorf("x: %w", dial), db.LostUnsent},
		{"a reset", read, db.LostSent},
		{"EOF", io.EOF, db.LostSent},
		{"any other", errors.New("something"), db.NotLost},
	} {
		if got := d.Broken(c.err); got != c.want {
			t.Errorf("%s: %v, want %v", c.name, got, c.want)
		}
	}
}
