// Package db is how the app talks to a database (tech-design §8): every
// value is text, and each connection runs one request at a time.
package db

import (
	"context"
	"time"

	"sqlmux/internal/sqlkit"
)

// Val is one value as the database's text, or NULL (§8.1).
type Val struct {
	S    string
	Null bool
}

type Col struct {
	Name string
	Type string // the database's type name; "" when the engine can't tell (e.g. a PG enum)
}

type Result struct {
	Cols      []Col
	Rows      [][]Val
	Tag       string // e.g. "UPDATE 3"
	Affected  int64  // the rows an INSERT, UPDATE or DELETE hit
	Truncated bool   // there were more rows than the caller's maxRows, which it keeps
	Took      time.Duration
}

// Conn is one engine's connection (§8.1). It sits behind a Worker, which
// runs one request on it at a time; cancelling a request's ctx cancels it on
// the server.
type Conn interface {
	// Exec runs sql, which may hold several statements, over the simple
	// protocol; each result keeps at most maxRows rows (0: all).
	Exec(ctx context.Context, sql string, maxRows int) ([]Result, error)
	// Query runs one statement over the extended protocol, args as text.
	Query(ctx context.Context, sql string, args ...Val) (Result, error)
	Close() error
}

// TxStatus is where c's transaction is: PG's connection tells ('I' out of
// one, 'T' in one, 'E' in one that failed); taken as out of one else.
func TxStatus(c Conn) byte {
	if c, ok := c.(interface{ TxStatus() byte }); ok {
		return c.TxStatus()
	}
	return 'I'
}

// Engine is what PG and MySQL each do their own way above a Conn (§8.1):
// connecting, writing names and arguments into SQL, the catalog (§8.4),
// quick SQL (§12), a console's schema (§8.6) and what an error says.
type Engine interface {
	// Connect opens one connection; readOnly makes its transactions read
	// only, as Meta's are (§8.2).
	Connect(ctx context.Context, dsn, password string, readOnly bool) (Conn, Info, error)
	Dialect() sqlkit.Dialect
	Quote(names ...string) string // a name, or schema and name, quoted
	Arg(n int) string             // the nth argument, from 1
	// Same is the condition that column col still holds what was read,
	// arg its text (§10.3).
	Same(col, arg string) string
	// Schemas lists the user's schemas (MySQL's databases), and the one
	// unqualified names go to first, "" for none.
	Schemas(ctx context.Context, c Conn) (schemas []string, current string, err error)
	// Tables lists every schema's tables and views by schema and name.
	Tables(ctx context.Context, c Conn) ([]Table, error)
	// TableColumns reads a table's columns, its primary key and the unique
	// indexes that can stand in for it (§10.1).
	TableColumns(ctx context.Context, c Conn, schema, table string) (Columns, error)
	// DDL is a table's or a view's definition (§12「预览」).
	DDL(ctx context.Context, c Conn, schema, name string) (string, error)
	// Quick runs one statement that reads, in a read-only transaction on
	// w, schema first, keeping maxRows rows (§12).
	Quick(ctx context.Context, w *Worker, sql, schema string, maxRows int) (Result, error)
	// UseSchema makes schema the first unqualified names go to, then
	// started, the one c connected with (§8.6).
	UseSchema(ctx context.Context, c Conn, schema, started string) error
	ErrorOf(err error) ServerError
}

// Info is where a connection landed: user@host:port for the status bar,
// the database for console titles, the schema path it started with
// (§7.8, §8.6).
type Info struct {
	Addr, Database, Path string
}

// ServerError is err as the result area's log and an error bar show it
// (§11, §7.8「错误栏」): a server's Severity, SQLSTATE and Message without
// the driver's dressing, its DETAIL and HINT a line each (DETAIL's own
// lines apart), and Position, the statement's character it points at from
// 1, 0 for none. An error no server sent is ERROR and its text alone.
type ServerError struct {
	Severity, Code, Message string
	More                    []string
	Position                int
}
