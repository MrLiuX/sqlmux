package postgres

import (
	"context"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"

	"sqlmux/internal/db"
)

// ErrorOf is err as the log and an error bar show it (§11, §7.8「错误栏」):
// a PgError's parts, pgconn's dressing off.
func (Engine) ErrorOf(err error) db.ServerError {
	var pe *pgconn.PgError
	if !errors.As(err, &pe) {
		return db.ServerError{Severity: "ERROR", Message: err.Error()}
	}
	e := db.ServerError{Severity: pe.Severity, Code: pe.Code, Message: pe.Message, Position: int(pe.Position)}
	for _, l := range [][2]string{{"DETAIL", pe.Detail}, {"HINT", pe.Hint}} {
		if l[1] != "" {
			e.More = append(e.More, strings.Split(l[0]+": "+l[1], "\n")...)
		}
	}
	return e
}

// Broken says whether err is a lost connection (§8.2「断线重连」): the
// network's, pgconn's own for a connection it has closed, which sent
// nothing, or the server's FATAL as it goes away, a shutdown, a
// pg_terminate_backend, an idle_session_timeout (pgconn closes the
// connection on any FATAL), or a connection failure (class 08). Any other
// PgError is a SQL error.
func (Engine) Broken(err error) db.Lost {
	var pe *pgconn.PgError
	switch {
	case errors.As(err, &pe):
		if pe.Severity == "FATAL" || pe.Severity == "PANIC" || strings.HasPrefix(pe.Code, "08") {
			return db.LostSent
		}
		return db.NotLost
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return db.NotLost
	case pgconn.SafeToRetry(err):
		return db.LostUnsent
	case db.NetworkError(err):
		return db.LostSent
	}
	return db.NotLost
}
