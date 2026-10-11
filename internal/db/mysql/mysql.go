// Package mysql is db.Conn over go-sql-driver/mysql (tech-design §8.1):
// one connection held out of a pool of one, the text protocol throughout
// (interpolateParams), every value as text.
package mysql

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"io"
	"log"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/go-sql-driver/mysql"

	"github.com/kackerx/sqlmux/internal/db"
	"github.com/kackerx/sqlmux/internal/sqlkit"
)

// The driver logs what goes wrong on a connection, one the server dropped,
// to stderr, over the screen; the caller gets the error anyway.
func init() { _ = mysql.SetLogger(log.New(io.Discard, "", 0)) }

type Conn struct {
	pool *sql.DB
	c    *sql.Conn
	kill driver.Connector // a second connection's, for KILL QUERY (§8.3)
	id   string           // connection_id()
	tx   byte             // 'I' or 'T', as the statements run tell (sqlkit.MySQLTx)
	dead bool             // a request found the connection lost: the Worker, told so by Closed, dials before the next
}

// Config is dsn as the driver takes it: mysql://user:pass@host:port/db?…,
// or the driver's own user:pass@tcp(host:port)/db?…; password, when not "",
// wins. The text protocol all the way, interpolateParams; an UPDATE's rows
// are those it found, clientFoundRows, not those it changed, or one
// setting what a column holds would count none (§10.3). Whatever the dsn
// says, values stay the server's text, not parseTime's; columns their
// names, not columnsWithAlias's table.name; a statement one, no
// multiStatements (§8.1).
func Config(dsn, password string) (*mysql.Config, error) {
	var cfg *mysql.Config
	var err error
	if strings.HasPrefix(dsn, "mysql://") {
		u, err := url.Parse(dsn)
		if ue := (*url.Error)(nil); errors.As(err, &ue) {
			return nil, fmt.Errorf("dsn: %w", ue.Err) // not the URL: it holds the password
		}
		host := u.Host
		if u.Port() == "" {
			host += ":3306"
		}
		if cfg, err = mysql.ParseDSN("tcp(" + host + ")/?" + u.RawQuery); err != nil {
			return nil, err
		}
		cfg.User, cfg.DBName = u.User.Username(), strings.TrimPrefix(u.Path, "/")
		cfg.Passwd, _ = u.User.Password()
	} else if cfg, err = mysql.ParseDSN(dsn); err != nil {
		return nil, err
	}
	if password != "" {
		cfg.Passwd = password
	}
	cfg.InterpolateParams, cfg.ClientFoundRows = true, true
	cfg.ParseTime, cfg.ColumnsWithAlias, cfg.MultiStatements = false, false, false
	if cfg.Timeout == 0 {
		cfg.Timeout = 10 * time.Second
	}
	return cfg, nil
}

// Connect opens one connection. readOnly makes every transaction read
// only, as the Meta connection is (§8.2).
func Connect(ctx context.Context, dsn, password string, readOnly bool) (*Conn, db.Info, error) {
	cfg, err := Config(dsn, password)
	if err != nil {
		return nil, db.Info{}, err
	}
	dial, err := mysql.NewConnector(cfg)
	if err != nil {
		return nil, db.Info{}, err
	}
	kcfg := cfg.Clone()
	kcfg.DBName = "" // KILL needs none, and one dropped since would stop it
	kill, err := mysql.NewConnector(kcfg)
	if err != nil {
		return nil, db.Info{}, err
	}
	pool := sql.OpenDB(dial)
	pool.SetMaxOpenConns(1)
	conn, err := pool.Conn(ctx)
	if err != nil {
		pool.Close()
		return nil, db.Info{}, err
	}
	c := &Conn{pool: pool, c: conn, kill: kill, tx: 'I'}
	// MySQL's schemas are its databases: no database before them on
	// query titles (§8.6)
	info := db.Info{Addr: cfg.User + "@" + cfg.Addr}
	r, err := c.Query(ctx, "select connection_id()")
	if err == nil && readOnly {
		_, err = c.Query(ctx, "set session transaction read only")
	}
	if err != nil {
		c.Close()
		return nil, db.Info{}, err
	}
	c.id = r.Rows[0][0].S
	return c, info, nil
}

// Closed is whether a request found the connection lost (Broken).
func (c *Conn) Closed() bool { return c.dead }

func (c *Conn) Close() error {
	c.c.Close()
	return c.pool.Close()
}

// TxStatus is 'T' after a begin or a start transaction and 'I' after a
// commit or a rollback: MySQL tells the driver, which tells no one.
// ponytail: a DDL's implicit commit, autocommit off go by unseen
func (c *Conn) TxStatus() byte { return c.tx }

// Exec runs sql, one statement: no multiStatements.
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

// rowWords start the statements that answer with rows, a WITH one when it
// reads; the rest are run for what they hit: database/sql tells the one or
// the other. A procedure's CALL may answer with rows: its first set is
// read.
var rowWords = map[string]bool{
	"select": true, "with": true, "table": true, "values": true, "show": true, "explain": true,
	"describe": true, "desc": true, "help": true, "analyze": true, "check": true, "checksum": true,
	"optimize": true, "repair": true, "call": true, "execute": true, "handler": true,
}

// run runs query, keeping at most maxRows rows (0: all), the rest read and
// dropped. A cancel of ctx kills the statement, not the connection (§8.3);
// cancelled before, it does not run. A deadlock, the transaction rolled
// back whole, comes as db.RolledBack (§8.2).
func (c *Conn) run(ctx context.Context, query string, args []any, maxRows int) (r db.Result, err error) {
	if err := ctx.Err(); err != nil { // a KILL QUERY finds nothing running yet
		return r, err
	}
	defer c.watch(ctx)()
	defer func() { err = c.noted(query, err) }()
	run, start := context.WithoutCancel(ctx), time.Now()
	word := sqlkit.FirstWord(query, sqlkit.MySQL)
	if !rowWords[word] || word == "with" && !sqlkit.IsRead(query, sqlkit.MySQL) {
		res, err := c.c.ExecContext(run, query, args...)
		if err != nil {
			return r, err
		}
		r.Affected, _ = res.RowsAffected()
		r.Tag = strings.ToUpper(word)
		switch word {
		case "insert", "update", "delete", "replace", "with":
			r.Tag += fmt.Sprintf(" %d", r.Affected)
		}
		r.Took = time.Since(start)
		return r, nil
	}
	rows, err := c.c.QueryContext(run, query, args...)
	if err != nil {
		return r, err
	}
	defer rows.Close()
	types, err := rows.ColumnTypes()
	if err != nil {
		return r, err
	}
	raw := make([]sql.RawBytes, len(types))
	dest := make([]any, len(types))
	for i, t := range types {
		r.Cols, dest[i] = append(r.Cols, db.Col{Name: t.Name(), Type: strings.ToLower(t.DatabaseTypeName())}), &raw[i]
	}
	for rows.Next() {
		if maxRows > 0 && len(r.Rows) == maxRows {
			r.Truncated = true
			continue
		}
		if err := rows.Scan(dest...); err != nil {
			return r, err
		}
		row := make([]db.Val, len(raw))
		for i, v := range raw {
			row[i] = db.Val{S: shown(v), Null: v == nil}
			if r.Cols[i].Type == "bit" && v != nil {
				row[i].S = bits(v)
			}
		}
		r.Rows = append(r.Rows, row)
	}
	r.Tag, r.Took = fmt.Sprintf("%s %d", strings.ToUpper(word), len(r.Rows)), time.Since(start)
	if err := ctx.Err(); err != nil { // killed, SLEEP() answers 1 as if done; a read, it did nothing else
		return r, err
	}
	return r, rows.Err()
}

// noted is what a statement's outcome does to c: the transaction it is in, or
// the connection it lost, and err as the caller has it (a deadlock comes as
// db.RolledBack).
func (c *Conn) noted(query string, err error) error {
	switch {
	case err == nil:
		c.tx = sqlkit.MySQLTx(query, c.tx)
	case errno(err) == 1213:
		c.tx, err = 'I', db.RolledBack{Err: err}
	case (Engine{}).Broken(err) != db.NotLost: // the server took the transaction with the connection
		c.tx, c.dead = 'I', true // a cancelled request, whose Worker does not dial, may find it so
	}
	return err
}

// Stream reads the rows of a statement that answers with them as they come,
// one at a time (F12.4); cancelled, it is killed as run's is.
func (c *Conn) Stream(ctx context.Context, query string, head func([]db.Col) error, row func([]db.Val) error) (err error) {
	if err := ctx.Err(); err != nil {
		return err
	}
	defer c.watch(ctx)()
	defer func() { err = c.noted(query, err) }()
	rows, err := c.c.QueryContext(context.WithoutCancel(ctx), query)
	if err != nil {
		return err
	}
	defer rows.Close()
	types, err := rows.ColumnTypes()
	if err != nil {
		return err
	}
	raw, dest, cols := make([]sql.RawBytes, len(types)), make([]any, len(types)), make([]db.Col, len(types))
	for i, t := range types {
		cols[i], dest[i] = db.Col{Name: t.Name(), Type: strings.ToLower(t.DatabaseTypeName())}, &raw[i]
	}
	if err := head(cols); err != nil {
		return err
	}
	for rows.Next() {
		if err := rows.Scan(dest...); err != nil {
			return err
		}
		vals := make([]db.Val, len(raw))
		for i, v := range raw {
			vals[i] = db.Val{S: shown(v), Null: v == nil}
			if cols[i].Type == "bit" && v != nil {
				vals[i].S = bits(v)
			}
		}
		if err := row(vals); err != nil {
			return err
		}
	}
	if err := ctx.Err(); err != nil { // killed: the rows so far were all of it
		return err
	}
	return rows.Err()
}

// shown is a value as the grid shows it: a blob that is no UTF-8 by its size (§8.1).
func shown(v []byte) string {
	if !utf8.Valid(v) {
		return db.BinaryShown(len(v))
	}
	return string(v)
}

// bits is a BIT's value, the bytes the text protocol sends, big end
// first, as a number (§8.1).
func bits(v []byte) string {
	var n uint64
	for _, b := range v {
		n = n<<8 | uint64(b)
	}
	return strconv.FormatUint(n, 10)
}

// errno is err's MySQL error number, 0 for none of the server's.
func errno(err error) uint16 {
	var me *mysql.MySQLError
	if errors.As(err, &me) {
		return me.Number
	}
	return 0
}

// watch kills what runs on c once ctx is cancelled, over a connection of
// its own (§8.3): cancelling the driver's ctx would close c, the statement
// running on in the server. The stop it returns waits for a kill under
// way, so none lands on the next statement.
// ponytail: a KILL that cannot connect (too many connections, the network
// gone) kills nothing, Main held till the statement ends; PG's drops its
// connection 5s on (§8.3)
func (c *Conn) watch(ctx context.Context) (stop func()) {
	if ctx.Done() == nil {
		return func() {}
	}
	done, over := make(chan struct{}), make(chan struct{})
	go func() {
		defer close(over)
		select {
		case <-ctx.Done():
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			pool := sql.OpenDB(c.kill)
			defer pool.Close()
			_, _ = pool.ExecContext(ctx, "kill query "+c.id)
		case <-done:
		}
	}()
	return func() { close(done); <-over }
}

// Engine is MySQL's way with the catalog, SQL and errors (§8.1).
type Engine struct{}

func (Engine) Connect(ctx context.Context, dsn, password string, readOnly bool) (db.Conn, db.Info, error) {
	c, info, err := Connect(ctx, dsn, password, readOnly)
	if err != nil {
		return nil, info, err
	}
	return c, info, nil
}

func (Engine) Dialect() sqlkit.Dialect { return sqlkit.MySQL }
func (Engine) Arg(int) string          { return "?" }

func (Engine) Quote(names ...string) string {
	quoted := make([]string, len(names))
	for i, n := range names {
		quoted[i] = "`" + strings.ReplaceAll(n, "`", "``") + "`"
	}
	return strings.Join(quoted, ".")
}

// Same compares bytes: what was read is the text the server sent, which
// CAST AS BINARY gives back for every type; = or <=> on the column would
// convert the text to its type, a json never equal to a string, a float
// off by its precision, and compare by the collation, case aside. CONVERT
// first makes a latin1 or gbk column's text UTF-8, as it was sent (§10.3).
// ponytail: the connection's character set is taken to be utf8mb4, the
// driver's
func (Engine) Same(col, arg string) string {
	return "cast(convert(" + col + " using utf8mb4) as binary) = " + arg
}

// UseSchema makes schema, a database, the current one; USE is the
// session's, whatever a transaction does (§8.6).
func (e Engine) UseSchema(ctx context.Context, c db.Conn, schema, _ string) error {
	_, err := c.Query(ctx, "use "+e.Quote(schema))
	return err
}

// Broken says whether err is a lost connection (§8.2「断线重连」):
// database/sql's ErrBadConn is the driver's for one that sent nothing; the
// rest is the driver's invalid connection, the out of sync packets it
// reads from what wait_timeout closed, the network's, and the server's own
// as it goes away (1053).
func (Engine) Broken(err error) db.Lost {
	switch {
	case errors.Is(err, driver.ErrBadConn):
		return db.LostUnsent
	case errors.Is(err, mysql.ErrInvalidConn), errors.Is(err, mysql.ErrPktSync), errors.Is(err, mysql.ErrPktSyncMul),
		db.NetworkError(err), errno(err) == 1053:
		return db.LostSent
	}
	return db.NotLost
}

// ErrorOf is err as the log and an error bar show it (§11): a server's
// SQLSTATE and message.
func (Engine) ErrorOf(err error) db.ServerError {
	var me *mysql.MySQLError
	if !errors.As(err, &me) {
		return db.ServerError{Severity: "ERROR", Message: err.Error()}
	}
	e := db.ServerError{Severity: "ERROR", Message: me.Message}
	if me.SQLState != [5]byte{} {
		e.Code = string(me.SQLState[:])
	}
	return e
}
