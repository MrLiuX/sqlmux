package mssql

import (
	"context"
	"errors"
	"time"

	"github.com/kackerx/sqlmux/internal/db"
	"github.com/kackerx/sqlmux/internal/i18n"
	"github.com/kackerx/sqlmux/internal/sqlkit"
)

// Quick runs sql, one statement, for the palette's quick SQL (§12). Out of
// a transaction of the user's it runs in one that is always rolled back —
// SQL Server has no read-only one (F18.6) — and a statement the words call
// a write is not sent at all, which is where the database itself says no
// for the other engines; in one of the user's, a write is the user's to
// commit, as PG's begin read only in there is a warning and no wall. The
// read gets the LIMIT §9.4 gives a query's, a row past maxRows, so a big
// one is not worked out whole; what AutoLimit does not append to runs as it
// is. At most maxRows rows come back, Truncated when there were more. The
// schema is nothing: SQL Server has no search_path, its unqualified names
// go to the user's default schema (§8.6).
// ponytail: what the words miss runs and is rolled back, identity values
// and all; a statement AutoLimit skips is read whole
func (e Engine) Quick(ctx context.Context, w *db.Worker, sql, _ string, maxRows int) (r db.Result, err error) {
	err = w.RunSafe(ctx, func(ctx context.Context, c db.Conn) error {
		if db.TxStatus(c) == 'I' {
			if sqlkit.IsWrite(sql, sqlkit.MSSQL) {
				return errors.New(i18n.T("快速 SQL 是只读的：这条是写语句"))
			}
			if _, err := c.Query(ctx, "begin transaction"); err != nil {
				return err
			}
			// a cancelled request still ends its transaction, in a time of its own (db.End, F13.6)
			defer func() { _, _ = db.End(ctx, c, "rollback") }()
		}
		text, _ := sqlkit.AutoLimit(sql, sqlkit.MSSQL, maxRows+1, 0)
		start := time.Now()
		r, err = c.Query(ctx, text)
		if len(r.Rows) > maxRows {
			r.Rows, r.Truncated = r.Rows[:maxRows], true
		}
		r.Took = time.Since(start)
		return err
	})
	return r, err
}
