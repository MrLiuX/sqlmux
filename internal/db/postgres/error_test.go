package postgres

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"slices"
	"syscall"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"

	"sqlmux/internal/db"
)

// pg is the engine the tests go through.
var pg Engine

func TestErrorOf(t *testing.T) {
	if e := pg.ErrorOf(errors.New("conn closed")); e.Severity != "ERROR" || e.Message != "conn closed" || e.Code != "" || e.More != nil {
		t.Errorf("no server's: %+v", e)
	}
	pe := &pgconn.PgError{Severity: "ERROR", Code: "42703", Message: "column \"x\" does not exist", Detail: "one\ntwo", Hint: "try y", Position: 8}
	if e := pg.ErrorOf(pe); e.Code != "42703" || e.Position != 8 || !slices.Equal(e.More, []string{"DETAIL: one", "two", "HINT: try y"}) {
		t.Errorf("%+v", e)
	}
}

// The hand-built errors: pgconn's own, and the server's, by Severity. The
// real ones, from a killed, cut or closed connection, are TestBrokenSamples's.
func TestBroken(t *testing.T) {
	for _, tc := range []struct {
		err  error
		want db.Lost
	}{
		{&pgconn.PgError{Severity: "FATAL", Code: "57P01", Message: "terminating connection due to administrator command"}, db.LostSent},
		{&pgconn.PgError{Severity: "FATAL", Code: "57P03"}, db.LostSent},
		{&pgconn.PgError{Severity: "FATAL", Code: "57P05", Message: "terminating connection due to idle-session timeout"}, db.LostSent},
		{&pgconn.PgError{Severity: "FATAL", Code: "25P03", Message: "terminating connection due to idle-in-transaction timeout"}, db.LostSent},
		{&pgconn.PgError{Severity: "PANIC", Code: "XX000"}, db.LostSent},
		{&pgconn.PgError{Severity: "ERROR", Code: "08006"}, db.LostSent},
		{&pgconn.PgError{Severity: "ERROR", Code: "42601", Message: "syntax error"}, db.NotLost},
		{&pgconn.PgError{Severity: "ERROR", Code: "57014", Message: "canceling statement due to user request"}, db.NotLost},
		{fmt.Errorf("failed to write: %w", &net.OpError{Op: "write", Err: syscall.ECONNRESET}), db.LostSent},
		{&net.OpError{Op: "read", Err: syscall.ETIMEDOUT}, db.LostSent},
		{io.EOF, db.LostSent},
		{fmt.Errorf("read: %w", io.ErrUnexpectedEOF), db.LostSent},
		{context.Canceled, db.NotLost},
		{context.DeadlineExceeded, db.NotLost},
		{errors.New("something else"), db.NotLost},
	} {
		if got := pg.Broken(tc.err); got != tc.want {
			t.Errorf("%v: %d, want %d", tc.err, got, tc.want)
		}
	}
}
