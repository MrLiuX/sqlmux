package mysql

import (
	"context"
	"database/sql/driver"
	"fmt"
	"io"
	"net"
	"syscall"
	"testing"

	"github.com/go-sql-driver/mysql"

	"github.com/kackerx/sqlmux/internal/db"
)

func TestBroken(t *testing.T) {
	for _, tc := range []struct {
		err  error
		want db.Lost
	}{
		{driver.ErrBadConn, db.LostUnsent},
		{fmt.Errorf("x: %w", driver.ErrBadConn), db.LostUnsent},
		{mysql.ErrInvalidConn, db.LostSent},
		{&net.OpError{Op: "write", Err: syscall.EPIPE}, db.LostSent},
		{&net.OpError{Op: "read", Err: syscall.ETIMEDOUT}, db.LostSent},
		{mysql.ErrPktSync, db.LostSent},
		{mysql.ErrPktSyncMul, db.LostSent},
		{context.DeadlineExceeded, db.NotLost},
		{io.EOF, db.LostSent},
		{&mysql.MySQLError{Number: 1053, Message: "Server shutdown in progress"}, db.LostSent},
		{&mysql.MySQLError{Number: 1064, Message: "You have an error in your SQL syntax"}, db.NotLost},
		{&mysql.MySQLError{Number: 1317, Message: "Query execution was interrupted"}, db.NotLost},
		{db.RolledBack{Err: &mysql.MySQLError{Number: 1213}}, db.NotLost},
		{context.Canceled, db.NotLost},
	} {
		if got := (Engine{}).Broken(tc.err); got != tc.want {
			t.Errorf("%v: %d, want %d", tc.err, got, tc.want)
		}
	}
}
