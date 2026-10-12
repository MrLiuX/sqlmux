// Package sqlite is the SQLite engine (§8.1, F14.1): a file, one connection
// of modernc.org/sqlite, which is Go all through (CGO_ENABLED=0).
package sqlite

import (
	"context"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"modernc.org/sqlite"

	"github.com/kackerx/sqlmux/internal/db"
	"github.com/kackerx/sqlmux/internal/sqlkit"
)

// Conn is one connection to the file. The file is its own server: nothing
// to kill (a cancelled ctx interrupts the statement and the connection
// stays), and nothing to lose.
type Conn struct {
	pool *sql.DB
	c    *sql.Conn
	tx   byte // 'I' or 'T', as the statements run tell (txAfter)
}

// Path is the file spec names: ~ at its start is the home directory, a
// relative one is relative to where sqlmux started; ?mode=ro and the like are
// the driver's, which Config passes on.
func Path(spec string) (path, query string, err error) {
	path, query, _ = strings.Cut(spec, "?")
	if rest, ok := strings.CutPrefix(path, "~"); ok && (rest == "" || rest[0] == '/') {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", "", err
		}
		path = home + rest
	}
	path, err = filepath.Abs(path)
	return path, query, err
}

// DSN is spec as the driver takes it: the file with its parameters, and a
// busy timeout, as Main writes while Meta reads the same file.
func DSN(spec string) (path, dsn string, err error) {
	path, query, err := Path(spec)
	if err != nil {
		return "", "", err
	}
	q, err := url.ParseQuery(query)
	if err != nil {
		return "", "", err
	}
	if !q.Has("_pragma") {
		q.Add("_pragma", "busy_timeout(5000)")
	}
	u := url.URL{Scheme: "file", Path: path, RawQuery: q.Encode()}
	return path, u.String(), nil
}

// Connect opens the file spec names (a new one if there is none). readOnly
// makes every transaction read only, as the Meta connection is (§8.2). The
// password is of no use here.
func Connect(ctx context.Context, spec, _ string, readOnly bool) (*Conn, db.Info, error) {
	path, dsn, err := DSN(spec)
	if err != nil {
		return nil, db.Info{}, err
	}
	pool, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, db.Info{}, err
	}
	pool.SetMaxOpenConns(1)
	conn, err := pool.Conn(ctx)
	if err != nil {
		pool.Close()
		return nil, db.Info{}, err
	}
	c := &Conn{pool: pool, c: conn, tx: 'I'}
	if _, err = c.Query(ctx, "select 1"); err == nil && readOnly { // the file is opened and read here: one that is no database fails now
		_, err = c.Query(ctx, "pragma query_only = on")
	}
	if err != nil {
		c.Close()
		return nil, db.Info{}, err
	}
	return c, db.Info{Addr: path, Path: "main"}, nil // no database before main on a query's title, as MySQL has none
}

func (c *Conn) Close() error {
	c.c.Close()
	return c.pool.Close()
}

// TxStatus is 'T' after a BEGIN and 'I' after a COMMIT, END or ROLLBACK: the
// driver tells no one.
// ponytail: a SAVEPOINT outside a transaction, which begins one, goes unseen
func (c *Conn) TxStatus() byte { return c.tx }

// Exec runs sql, one statement.
func (c *Conn) Exec(ctx context.Context, sql string, maxRows int) ([]db.Result, error) {
	r, err := c.run(ctx, sql, nil, maxRows)
	if err != nil {
		return nil, err
	}
	return []db.Result{r}, nil
}

func (c *Conn) Query(ctx context.Context, sql string, args ...db.Val) (db.Result, error) {
	vals := make([]any, len(args))
	for i, a := range args {
		if !a.Null {
			vals[i] = a.S
		}
	}
	return c.run(ctx, sql, vals, 0)
}

// run runs query, keeping at most maxRows rows (0: all), the rest read and
// dropped. Every statement goes through Query, whether it answers with rows
// or not (a RETURNING one does): one with no columns has changes() asked
// afterwards.
func (c *Conn) run(ctx context.Context, query string, args []any, maxRows int) (r db.Result, err error) {
	start := time.Now()
	if err := ctx.Err(); err != nil {
		return r, err
	}
	defer func() {
		if err == nil {
			c.tx = txAfter(query, c.tx)
		}
	}()
	rows, err := c.c.QueryContext(ctx, query, args...)
	if err != nil {
		return r, err
	}
	defer rows.Close()
	err = each(rows, shown, func(cols []db.Col) error {
		if len(cols) > 0 { // none for a statement that answers with none
			r.Cols = cols
		}
		return nil
	},
		func() bool {
			if maxRows > 0 && len(r.Rows) == maxRows {
				r.Truncated = true
				return false
			}
			return true
		},
		func(vals []db.Val) error { r.Rows = append(r.Rows, vals); return nil })
	if err != nil {
		return r, err
	}
	rows.Close()
	word := sqlkit.FirstWord(query, sqlkit.SQLite)
	if len(r.Cols) == 0 {
		r.Affected, err = c.changes(ctx)
		r.Tag = strings.ToUpper(word)
		switch word {
		case "insert", "update", "delete", "replace":
			r.Tag += fmt.Sprintf(" %d", r.Affected)
		}
	} else {
		r.Tag = fmt.Sprintf("%s %d", strings.ToUpper(word), len(r.Rows))
	}
	r.Took = time.Since(start)
	return r, err
}

// changes is the rows the last INSERT, UPDATE or DELETE hit, as found,
// whether or not they changed.
func (c *Conn) changes(ctx context.Context) (n int64, err error) {
	err = c.c.QueryRowContext(ctx, "select changes()").Scan(&n)
	return n, err
}

// txAfter is where a transaction is after statement s ran in it, tx before.
func txAfter(s string, tx byte) byte {
	switch {
	case sqlkit.FirstWord(s, sqlkit.SQLite) == "begin":
		return 'T'
	case sqlkit.EndsTx(s, sqlkit.SQLite) != "":
		return 'I'
	}
	return tx
}

// Stream reads the rows of a statement as they come, one at a time (F12.4),
// a stop of either callback leaving the rest unread.
func (c *Conn) Stream(ctx context.Context, query string, head func([]db.Col) error, row func([]db.Val) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	rows, err := c.c.QueryContext(ctx, query)
	if err != nil {
		return err
	}
	defer rows.Close()
	return each(rows, hexed, head, func() bool { return true }, row)
}

// each reads the result in rows: head with its columns, then row with each of
// the rows want says to keep, the others read and dropped; text tells a
// blob (the grid's and the file's differ, F12.4). Columns are typed as the
// table declared them, "" for none, an expression's.
func each(rows *sql.Rows, text func([]byte) string, head func([]db.Col) error, want func() bool, row func([]db.Val) error) error {
	types, err := rows.ColumnTypes()
	if err != nil {
		return err
	}
	raw, dest, cols := make([]any, len(types)), make([]any, len(types)), make([]db.Col, len(types))
	for i, t := range types {
		cols[i], dest[i] = db.Col{Name: t.Name(), Type: strings.ToLower(t.DatabaseTypeName())}, &raw[i]
	}
	if err := head(cols); err != nil {
		return err
	}
	for rows.Next() {
		if !want() {
			continue
		}
		if err := rows.Scan(dest...); err != nil {
			return err
		}
		vals := make([]db.Val, len(raw))
		for i, v := range raw {
			vals[i] = cell(v, text)
		}
		if err := row(vals); err != nil {
			return err
		}
	}
	return rows.Err()
}

// cell is the text of value v the driver read: an integer, a real as SQLite
// writes it, a string, a blob as text tells.
func cell(v any, text func([]byte) string) db.Val {
	switch v := v.(type) {
	case nil:
		return db.Val{Null: true}
	case int64:
		return db.Val{S: strconv.FormatInt(v, 10)}
	case float64:
		return db.Val{S: realText(v)}
	case string:
		return db.Val{S: text([]byte(v))}
	case []byte:
		return db.Val{S: text(v)}
	}
	return db.Val{S: fmt.Sprint(v)} // none the driver gives with its defaults
}

// realText is f as SQLite writes a REAL as text: the digits that read back as
// f, a point always (3.0), an exponent from 1e17 and under 1e-4 (1.0e+22).
func realText(f float64) string {
	switch {
	case f == 0:
		return "0.0"
	case math.IsInf(f, 0):
		return strings.Replace(strconv.FormatFloat(f, 'g', -1, 64), "+", "", 1)
	}
	form := byte('f')
	if a := math.Abs(f); a < 1e-4 || a >= 1e17 {
		form = 'e'
	}
	s := strconv.FormatFloat(f, form, -1, 64)
	mantissa, exp, _ := strings.Cut(s, "e")
	if !strings.Contains(mantissa, ".") {
		mantissa += ".0"
	}
	if exp != "" {
		exp = "e" + exp
	}
	return mantissa + exp
}

// shown is a value as the grid shows it: a text or blob that is no UTF-8 by
// its size (§8.1).
func shown(v []byte) string {
	if !utf8.Valid(v) {
		return db.BinaryShown(len(v))
	}
	return string(v)
}

// hexed is a value as a file has it: a text or blob that is no UTF-8 as the
// blob literal X'…', not the grid's placeholder, which is no data (F12.4).
func hexed(v []byte) string {
	if !utf8.Valid(v) {
		return "X'" + strings.ToUpper(hex.EncodeToString(v)) + "'"
	}
	return string(v)
}

// Engine is SQLite's way with the catalog, SQL and errors (§8.1).
type Engine struct{}

func (Engine) Connect(ctx context.Context, dsn, password string, readOnly bool) (db.Conn, db.Info, error) {
	c, info, err := Connect(ctx, dsn, password, readOnly)
	if err != nil {
		return nil, info, err
	}
	return c, info, nil
}

func (Engine) Dialect() sqlkit.Dialect { return sqlkit.SQLite }
func (Engine) Arg(int) string          { return "?" }

func (Engine) Quote(names ...string) string {
	quoted := make([]string, len(names))
	for i, n := range names {
		quoted[i] = `"` + strings.ReplaceAll(n, `"`, `""`) + `"`
	}
	return strings.Join(quoted, ".")
}

// Same compares the column's text, which is what was read.
// ponytail: a REAL SQLite writes with other digits than realText does is a row
// changed, to a save
func (Engine) Same(col, arg string) string { return "cast(" + col + " as text) = " + arg }

// UseSchema does nothing: the file is the one schema, main.
func (Engine) UseSchema(context.Context, db.Conn, string, string) error { return nil }

// Broken is none: a file is no connection to lose.
func (Engine) Broken(error) db.Lost { return db.NotLost }

// ErrorOf is err as the log and an error bar show it (§11): the library's
// message.
func (Engine) ErrorOf(err error) db.ServerError {
	e := db.ServerError{Severity: "ERROR", Message: err.Error()}
	var se *sqlite.Error
	if errors.As(err, &se) {
		e.Code = strconv.Itoa(se.Code())
	}
	return e
}
