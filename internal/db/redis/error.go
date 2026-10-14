package redis

import (
	"context"
	"errors"
	"net"
	"strings"

	goredis "github.com/redis/go-redis/v9"

	"github.com/kackerx/sqlmux/internal/db"
)

// ErrorOf is err as the log and an error bar show it (§11): a server's error
// is its first word, WRONGTYPE or ERR, and the rest; any other is its text.
func (Driver) ErrorOf(err error) db.ServerError {
	var re goredis.Error
	if !errors.As(err, &re) {
		return db.ServerError{Severity: "ERROR", Message: err.Error()}
	}
	code, msg, _ := strings.Cut(re.Error(), " ")
	return db.ServerError{Severity: "ERROR", Code: code, Message: msg}
}

// Broken says whether err is a lost connection (§8.2「断线重连」, §8.7): the
// network's, a dial that failed, the client's own for a connection closed. What
// the server answers is not, LOADING, MASTERDOWN and READONLY included.
func (Driver) Broken(err error) db.Lost {
	var re goredis.Error
	var op *net.OpError
	switch {
	case errors.As(err, &re):
		return db.NotLost
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return db.NotLost
	case errors.Is(err, goredis.ErrClosed): // nothing went out
		return db.LostUnsent
	case errors.As(err, &op) && op.Op == "dial":
		return db.LostUnsent
	case db.NetworkError(err):
		return db.LostSent
	}
	return db.NotLost
}
