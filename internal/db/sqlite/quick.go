package sqlite

import (
	"context"
	"time"

	"github.com/kackerx/sqlmux/internal/db"
	"github.com/kackerx/sqlmux/internal/sqlkit"
)

// Quick runs sql, one statement IsRead takes, for the palette's quick SQL
// (§12): in a transaction that is always rolled back, on the Meta connection,
// which is read only. With no cursor to stop at, a read gets the LIMIT §9.4
// gives a query's, a row past maxRows; at most maxRows rows come back,
// Truncated when there were more.
// ponytail: one with a LIMIT of its own, or in parentheses, is worked out
// whole
func (Engine) Quick(ctx context.Context, w *db.Worker, sql, _ string, maxRows int) (r db.Result, err error) {
	err = w.RunSafe(ctx, func(ctx context.Context, c db.Conn) error {
		if _, err := c.Query(ctx, "begin"); err != nil {
			return err
		}
		// a cancelled request still ends its transaction, in a time of its own (db.End, F13.6)
		defer func() { _, _ = db.End(ctx, c, "rollback") }()
		start := time.Now()
		text, _ := sqlkit.AutoLimit(sql, sqlkit.SQLite, maxRows+1, 0)
		r, err = c.Query(ctx, text)
		if len(r.Rows) > maxRows {
			r.Rows, r.Truncated = r.Rows[:maxRows], true
		}
		r.Took = time.Since(start)
		return err
	})
	return r, err
}
