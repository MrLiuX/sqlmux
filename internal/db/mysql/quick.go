package mysql

import (
	"context"
	"time"

	"sqlmux/internal/db"
	"sqlmux/internal/sqlkit"
)

// Quick runs sql, one statement IsRead takes, for the palette's quick SQL
// (§12): in a read-only transaction that is always rolled back, in
// database schema. With no cursor to stop at, a read gets the LIMIT §9.4
// gives a console's, a row past maxRows; at most maxRows rows come back,
// Truncated when there were more.
// ponytail: one with a LIMIT of its own, or in parentheses, is worked out
// whole
func (e Engine) Quick(ctx context.Context, w *db.Worker, sql, schema string, maxRows int) (r db.Result, err error) {
	err = w.Run(ctx, func(ctx context.Context, c db.Conn) error {
		if _, err := c.Query(ctx, "start transaction read only"); err != nil {
			return err
		}
		// not on ctx: a cancelled request still ends its transaction
		defer c.Query(context.Background(), "rollback")
		if schema != "" {
			if err := e.UseSchema(ctx, c, schema, ""); err != nil {
				return err
			}
		}
		start := time.Now()
		r, err = c.Query(ctx, sqlkit.AutoLimit(sql, sqlkit.MySQL, maxRows+1))
		if len(r.Rows) > maxRows {
			r.Rows, r.Truncated = r.Rows[:maxRows], true
		}
		r.Took = time.Since(start)
		return err
	})
	return r, err
}
