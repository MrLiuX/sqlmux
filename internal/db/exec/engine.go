package exec

import (
	"context"
	"errors"
	"path/filepath"
	"time"

	"github.com/kackerx/sqlmux/internal/db"
	"github.com/kackerx/sqlmux/internal/sqlkit"
)

// Engine is the SQL engine Engine, its catalog, paging, quoting and comparisons as
// they are, over a command: only where connections come from is changed, and what
// has no meaning where no run keeps anything for the next: a schema to put first,
// a transaction to read in, a lost connection to dial again (§8.8).
type Engine struct {
	db.Engine
	Command []string
}

// Connect makes the connection of the command, and runs one statement on it to find
// where it landed: it fails here, not at the first thing the tree asks, when the
// command does not work. dsn and password are not used, a read-only request is no
// more than the connection is.
func (e Engine) Connect(ctx context.Context, _, _ string, _ bool) (db.Conn, db.Info, error) {
	c, err := New(e.Command, e.Dialect())
	if err != nil {
		return nil, db.Info{}, err
	}
	info := db.Info{Addr: filepath.Base(e.Command[0])}
	if e.Dialect() != sqlkit.PG {
		_, err = c.Query(ctx, "select 1")
		return c, info, err
	}
	r, err := c.Query(ctx, "select current_database(), current_setting('search_path')")
	if err != nil {
		return nil, db.Info{}, err
	}
	if len(r.Rows) == 1 && len(r.Rows[0]) == 2 {
		info.Database, info.Path = r.Rows[0][0].S, r.Rows[0][1].S
	}
	return c, info, nil
}

// Broken: a run that failed has not lost anything; the next is a new one.
func (Engine) Broken(error) db.Lost { return db.NotLost }

// ErrorOf is what the command said, or the error as it is.
func (Engine) ErrorOf(err error) db.ServerError {
	var ce Error
	if errors.As(err, &ce) {
		return db.ServerError{Severity: "ERROR", Message: ce.Msg}
	}
	return db.ServerError{Severity: "ERROR", Message: err.Error()}
}

// Databases lists none: the command reaches the one database it is made for, and
// the DSN that would name another is not used.
// ponytail: a command that can reach several is one connection each
func (Engine) Databases(context.Context, db.Conn) ([]string, error) { return nil, nil }

// UseSchema does nothing: the next run would not have it.
// ponytail: a query's unqualified names go to the search_path the command
// connects with; qualify them, or let the command set it
func (Engine) UseSchema(context.Context, db.Conn, string, string) error { return nil }

// Quick runs one statement, as it is: there is no transaction to read in, the
// command is read only (§8.8). At most maxRows rows come back, Truncated when
// there were more.
func (Engine) Quick(ctx context.Context, w *db.Worker, sql, _ string, maxRows int) (r db.Result, err error) {
	err = w.RunSafe(ctx, func(ctx context.Context, c db.Conn) (err error) {
		start := time.Now()
		r, err = c.Query(ctx, sql)
		if len(r.Rows) > maxRows {
			r.Rows, r.Truncated = r.Rows[:maxRows], true
		}
		r.Took = time.Since(start)
		return err
	})
	return r, err
}
