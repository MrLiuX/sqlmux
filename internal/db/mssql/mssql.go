// Package mssql is db.Conn over go-mssqldb (tech-design §8.1): every value
// as text, one connection held out of a pool of one, a batch of statements
// sent whole and its result sets collected.
package mssql

import (
	"context"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	mssqldb "github.com/microsoft/go-mssqldb"
	"github.com/microsoft/go-mssqldb/msdsn"

	"github.com/kackerx/sqlmux/internal/db"
	"github.com/kackerx/sqlmux/internal/sqlkit"
)

type Conn struct {
	pool *sql.DB
	c    *sql.Conn
	tx   byte // 'I' or 'T', as the statements run tell (sqlkit.MSSQLTx)
	dead bool // a request found the connection lost: the Worker, told so by Closed, dials before the next
}

// Config is dsn as the driver takes it, the sqlserver:// URL it documents
// or its keyword form; password, when not "", wins (§8.1).
func Config(dsn, password string) (msdsn.Config, error) {
	cfg, err := msdsn.Parse(dsn)
	if err != nil {
		return msdsn.Config{}, err
	}
	if password != "" {
		cfg.Password = password
	}
	if cfg.DialTimeout == 0 {
		cfg.DialTimeout = 10 * time.Second
	}
	return cfg, nil
}

// Connect opens one connection, and says where it is: the address for the
// status bar (§7.8), the database as db_name() has it, the DSN's or the
// server's default; Path is "", there being no search_path (§8.6). readOnly
// sets nothing on the server: SQL Server has no read-only transactions, a
// read-only connection is the app's interception (§13, F18.6).
func Connect(ctx context.Context, dsn, password string, _ bool) (*Conn, db.Info, error) {
	cfg, err := Config(dsn, password)
	if err != nil {
		return nil, db.Info{}, err
	}
	pool := sql.OpenDB(mssqldb.NewConnectorConfig(cfg))
	pool.SetMaxOpenConns(1)
	conn, err := pool.Conn(ctx)
	if err != nil {
		pool.Close()
		return nil, db.Info{}, err
	}
	c := &Conn{pool: pool, c: conn, tx: 'I'}
	// "…" is an identifier here as everywhere else in sqlmux (§9.1), not a
	// string, which a session's default is not always enough for (F18.6)
	if _, err := conn.ExecContext(ctx, "set quoted_identifier on"); err != nil {
		c.Close()
		return nil, db.Info{}, err
	}
	r, err := c.Query(ctx, "select db_name(), suser_sname()")
	if err != nil {
		c.Close()
		return nil, db.Info{}, err
	}
	user := cfg.User
	if user == "" {
		user = r.Rows[0][1].S // what a trusted connection came as
	}
	info := db.Info{Addr: user + "@" + cfg.Host + ":" + strconv.FormatUint(cfg.Port, 10), Database: r.Rows[0][0].S}
	return c, info, nil
}

// WithDatabase is dsn naming database name instead of the one it names: the
// URL's database (or initial catalog) parameter, or the keyword form's
// (M15 F15.1).
func WithDatabase(dsn, name string) (string, error) {
	if strings.HasPrefix(dsn, "sqlserver://") {
		u, err := url.Parse(dsn)
		if err != nil {
			return "", err
		}
		q := u.Query()
		q.Del("database") // and the keyword form's name for it, which the URL takes too
		q.Del("initial catalog")
		q.Set("database", name)
		u.RawQuery = q.Encode()
		return u.String(), nil
	}
	return dsn + ";database=" + name, nil
}

func (c *Conn) Close() error {
	c.c.Close()
	return c.pool.Close()
}

// Closed is whether a request found the connection lost (Broken).
func (c *Conn) Closed() bool { return c.dead }

// TxStatus is 'T' after a begin transaction and 'I' after a commit or a
// rollback: SQL Server tells the driver, which tells no one.
func (c *Conn) TxStatus() byte { return c.tx }

// rowWords start the statements that answer with rows; the rest are run for
// what they hit, as database/sql tells the one or the other. An EXEC may
// answer with rows: its first set is read.
var rowWords = map[string]bool{"select": true, "with": true, "exec": true, "execute": true}

// Exec runs sql, one batch: the driver sends several statements whole, and
// every result set that answers is one of the Results (§8.1).
func (c *Conn) Exec(ctx context.Context, sql string, maxRows int) ([]db.Result, error) {
	return c.run(ctx, sql, nil, maxRows)
}

// Query runs one statement, args as text: the driver's named parameters, as
// Arg writes them (§8.1).
func (c *Conn) Query(ctx context.Context, sql string, args ...db.Val) (db.Result, error) {
	rs, err := c.run(ctx, sql, db.SQLArgs(args), 0)
	if len(rs) > 0 {
		return rs[0], err
	}
	return db.Result{}, err
}

// run runs query, keeping at most maxRows rows (0: all) of every result set
// it answers with, the rest read and dropped. A cancel of ctx is the
// attention the driver sends, and the connection stays: SQL Server stops
// the statement, not the session (§8.3, tried on a real one, F18.6).
func (c *Conn) run(ctx context.Context, query string, args []any, maxRows int) (rs []db.Result, err error) {
	if err := ctx.Err(); err != nil { // a cancel finds nothing running yet
		return nil, err
	}
	start := time.Now()
	word := sqlkit.FirstWord(query, sqlkit.MSSQL)
	if !rowWords[word] || word == "with" && !sqlkit.IsRead(query, sqlkit.MSSQL) {
		res, err := c.c.ExecContext(ctx, query, args...)
		if err != nil {
			return nil, c.noted(query, err)
		}
		n, _ := res.RowsAffected()
		r := db.Result{Affected: n}
		r.Tag = strings.ToUpper(word)
		switch word {
		case "insert", "update", "delete", "merge":
			r.Tag += fmt.Sprintf(" %d", r.Affected)
		}
		r.Took = time.Since(start)
		return []db.Result{r}, c.noted(query, nil)
	}
	rows, err := c.c.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, c.noted(query, err)
	}
	defer rows.Close()
	for {
		var one db.Result
		if err = readSet(rows, &one, db.Capped(&one, maxRows)); err != nil {
			break
		}
		one.Tag = fmt.Sprintf("%s %d", strings.ToUpper(word), len(one.Rows))
		one.Took = time.Since(start)
		rs = append(rs, one)
		if !rows.NextResultSet() {
			break
		}
	}
	// ponytail: a batch that starts reading and writes after its first
	// result set comes back with the sets and none of the writes' counts,
	// as the rows of database/sql have no RowsAffected of their own
	return rs, c.noted(query, err)
}

// noted is what a statement's outcome does to c: the transaction it is in,
// or the connection it lost, and err as the caller has it. A cancel is one
// of the lost: the driver sends the attention and the server stops, but the
// driver closes the connection with the ctx, as the pool's callers never
// see — it dials them another — so the Worker redials (F8.19, §8.3, F18.6).
func (c *Conn) noted(query string, err error) error {
	switch {
	case err == nil:
		c.tx = sqlkit.MSSQLTx(query, c.tx)
	case errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded),
		(Engine{}).Broken(err) != db.NotLost: // the server took the transaction with the connection
		c.tx, c.dead = 'I', true
	}
	return err
}

// readSet reads the result set rows is on into r: its columns, then each of
// the rows want says to keep, the others read and dropped.
func readSet(rows *sql.Rows, r *db.Result, want func() bool) error {
	ts, err := rows.ColumnTypes()
	if err != nil {
		return err
	}
	raw, dest, cols := make([]any, len(ts)), make([]any, len(ts)), make([]db.Col, len(ts))
	for i, t := range ts {
		cols[i], dest[i] = db.Col{Name: t.Name(), Type: strings.ToLower(t.DatabaseTypeName())}, &raw[i]
	}
	if err := rows.Err(); err != nil {
		return err
	}
	r.Cols = cols
	for rows.Next() {
		if !want() {
			continue
		}
		if err := rows.Scan(dest...); err != nil {
			return err
		}
		vals := make([]db.Val, len(raw))
		for i, v := range raw {
			vals[i] = cellOf(v, ts[i])
		}
		r.Rows = append(r.Rows, vals)
	}
	return rows.Err()
}

// Stream reads the rows of a statement that answers with them as they come,
// one at a time, head with the columns first (F12.4). A cancel of ctx is
// the driver's attention, and the connection stays (§8.3).
func (c *Conn) Stream(ctx context.Context, query string, head func([]db.Col) error, row func([]db.Val) error) (err error) {
	if err := ctx.Err(); err != nil {
		return err
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	rows, err := c.c.QueryContext(ctx, query)
	if err != nil {
		return c.noted(query, err)
	}
	defer rows.Close()
	err = streamSet(rows, head, func() bool { return true }, row)
	if cerr := ctx.Err(); cerr != nil { // cancelled: the rows so far were all of it
		return cerr
	}
	return c.noted(query, err)
}

// streamSet gives the rows of the result set rows is on to row one at a
// time, head with the columns first; head or row failing, the rest is left
// and the statement cancelled by the ctx the caller wrapped (F12.4).
func streamSet(rows *sql.Rows, head func([]db.Col) error, want func() bool, row func([]db.Val) error) error {
	ts, err := rows.ColumnTypes()
	if err != nil {
		return err
	}
	raw, dest, cols := make([]any, len(ts)), make([]any, len(ts)), make([]db.Col, len(ts))
	for i, t := range ts {
		cols[i], dest[i] = db.Col{Name: t.Name(), Type: strings.ToLower(t.DatabaseTypeName())}, &raw[i]
	}
	if err := rows.Err(); err != nil {
		return err
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
			vals[i] = hexCellOf(v, ts[i])
		}
		if err := row(vals); err != nil {
			return err
		}
	}
	return rows.Err()
}

// cellOf is a value as the grid shows it (§8.1): what the driver decoded,
// as its type's text says — a datetime family's fixed-scale form, which
// try_convert(nvarchar, the column, 126) reads back the same (F18.6); a bit
// as true or false; a uniqueidentifier as the standard GUID text the server
// writes, upper case; a type that holds bytes by its size, whatever the
// bytes are.
func cellOf(v any, col columnMeta) db.Val {
	typ := strings.ToLower(col.DatabaseTypeName())
	switch x := v.(type) {
	case nil:
		return db.Val{Null: true}
	case bool:
		return db.Val{S: strconv.FormatBool(x)}
	case time.Time:
		return db.Val{S: timeText(x, typ, col)}
	case []byte:
		switch {
		case typ == "uniqueidentifier":
			return db.Val{S: guidOf(x)}
		case bytesTypes[typ] || !utf8.Valid(x):
			return db.Val{S: db.BinaryShown(len(x))}
		}
		return db.Val{S: string(x)}
	case string:
		return db.Val{S: x}
	case int64:
		return db.Val{S: strconv.FormatInt(x, 10)}
	case float64:
		return db.Val{S: strconv.FormatFloat(x, 'g', -1, 64)}
	}
	return db.Val{S: fmt.Sprintf("%v", v)}
}

// bytesTypes are the column types that hold bytes, not text: what the grid
// shows by their size, and an export as 0x and its hex (§8.1).
var bytesTypes = map[string]bool{"binary": true, "varbinary": true, "image": true, "rowversion": true, "timestamp": true}

// hexCellOf is a value as a file has it: a type that holds bytes as 0x and
// its hex, as T-SQL writes a literal, not the grid's placeholder, which is
// no data (F12.4).
func hexCellOf(v any, col columnMeta) db.Val {
	typ := strings.ToLower(col.DatabaseTypeName())
	if b, ok := v.([]byte); ok && bytesTypes[typ] {
		return db.Val{S: "0x" + hex.EncodeToString(b)}
	}
	return cellOf(v, col)
}

// columnMeta is what a value's column tells of itself: its type, and, for
// the datetime family, the scale its text keeps (F18.6).
type columnMeta interface {
	DatabaseTypeName() string
	DecimalSize() (precision, scale int64, ok bool)
}

// guidOf is the 16 bytes of a uniqueidentifier as the standard GUID text
// (§8.1): the first three groups travel little end first, and the text the
// server writes has them the other way round, upper case, as
// try_convert(nvarchar(max), the column) reads back.
func guidOf(b []byte) string {
	if len(b) != 16 {
		return ""
	}
	return strings.ToUpper(fmt.Sprintf("%02x%02x%02x%02x-%02x%02x-%02x%02x-%02x%02x-%02x%02x%02x%02x%02x%02x",
		b[3], b[2], b[1], b[0], b[5], b[4], b[7], b[6], b[8], b[9], b[10], b[11], b[12], b[13], b[14], b[15]))
}

// timeText is a time the driver decoded as its type's text, the fractional
// digits the column's scale has, as try_convert(nvarchar, the column, 126)
// writes it back (F18.6): a datetime2's and a datetimeoffset's T between
// the parts, a datetimeoffset's zone at the end, a time alone, a date
// alone. A datetime, which has no scale of its own, keeps its three.
func timeText(t time.Time, typ string, ct columnMeta) string {
	scale := 7
	if typ == "datetime" {
		scale = 3
	}
	if _, s, ok := ct.DecimalSize(); ok && s > 0 {
		scale = int(s)
	}
	switch typ {
	case "date":
		return t.Format("2006-01-02")
	case "time":
		return t.Format("15:04:05") + fracOf(t, scale)
	case "datetimeoffset":
		return t.Format("2006-01-02T15:04:05") + fracOf(t, scale) + t.Format("-07:00")
	}
	return t.Format("2006-01-02T15:04:05") + fracOf(t, scale)
}

// fracOf is t's fraction as scale digits, of the nine the time holds.
func fracOf(t time.Time, scale int) string {
	if scale <= 0 {
		return ""
	}
	return "." + fmt.Sprintf("%09d", t.Nanosecond())[:scale]
}

// Engine is SQL Server's way with the catalog, SQL and errors (§8.1).
type Engine struct{}

func (Engine) Connect(ctx context.Context, dsn, password string, _ bool) (db.Conn, db.Info, error) {
	c, info, err := Connect(ctx, dsn, password, false)
	if err != nil {
		return nil, db.Info{}, err
	}
	return c, info, nil
}

func (Engine) Dialect() sqlkit.Dialect { return sqlkit.MSSQL }

func (Engine) Quote(names ...string) string {
	quoted := make([]string, len(names))
	for i, n := range names {
		quoted[i] = "[" + strings.ReplaceAll(n, "]", "]]") + "]"
	}
	return strings.Join(quoted, ".")
}

func (Engine) Arg(n int) string { return fmt.Sprintf("@p%d", n) }

func (Engine) Eq(col, arg string) string { return col + " = " + arg }

// Same is the column's text as try_convert(nvarchar(max), the column, 126)
// has it — the form the values were read in, a datetime family's T between
// its parts, a money's four decimals — against what was read; the second
// arm takes the types that take no style, an xml's (F18.6, §10.3).
func (Engine) Same(col, arg string) string {
	return "try_convert(nvarchar(max), " + col + ", 126) = " + arg +
		" or try_convert(nvarchar(max), " + col + ") = " + arg
}

// Begin is the statement that starts a transaction (§10.3, F18.6).
func (*Conn) Begin() string { return "begin transaction" }

// Savepoint is name's savepoint, as SQL Server has it: SAVE TRANSACTION to
// set, ROLLBACK TRANSACTION to take back what came after, and no release —
// the name stands till the transaction ends, and setting it again only
// moves it (F18.6).
func (*Conn) Savepoint(name string) (set, rollback, release string) {
	return "save transaction " + name, "rollback transaction " + name, ""
}

// UseSchema is nothing: SQL Server has no search_path (§8.6, F18.6).
func (Engine) UseSchema(context.Context, db.Conn, string, string) error { return nil }
