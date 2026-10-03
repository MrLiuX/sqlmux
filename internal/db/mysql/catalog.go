package mysql

import (
	"context"
	"strconv"
	"strings"

	"sqlmux/internal/db"
)

// Catalog queries (§8.4), over information_schema as lazysql's
// drivers/mysql.go reads it. A schema is a database.

// userSchema keeps the databases a user browses.
const userSchema = "not in ('information_schema', 'mysql', 'performance_schema', 'sys')"

// Schemas lists the user's databases by name, and the current one, ""
// when the DSN names none.
func (Engine) Schemas(ctx context.Context, c db.Conn) (schemas []string, current string, err error) {
	r, err := c.Query(ctx, "select schema_name, coalesce(database(), '') from information_schema.schemata where schema_name "+userSchema+" order by 1")
	if err != nil {
		return nil, "", err
	}
	for _, row := range r.Rows {
		schemas, current = append(schemas, row[0].S), row[1].S
	}
	return schemas, current, nil
}

// Tables lists every database's tables and views, by database and name; a
// view has no row count. Partitions are no tables of their own.
// ponytail: TABLE_ROWS is cached for information_schema_stats_expiry, a
// day by default, so counts lag behind; ANALYZE TABLE refreshes them
func (Engine) Tables(ctx context.Context, c db.Conn) ([]db.Table, error) {
	r, err := c.Query(ctx, `
select table_schema, table_name, coalesce(table_rows, -1), table_type
from information_schema.tables
where table_schema `+userSchema+`
order by 1, 2`)
	if err != nil {
		return nil, err
	}
	ts := make([]db.Table, len(r.Rows))
	for i, row := range r.Rows {
		rows, err := strconv.ParseFloat(row[2].S, 64)
		if err != nil {
			return nil, err
		}
		kind := "r" // pg_class's relkind, which db.Table goes by
		if row[3].S == "VIEW" {
			kind = "v"
		}
		ts[i] = db.Table{Schema: row[0].S, Name: row[1].S, Rows: rows, Kind: kind}
	}
	return ts, nil
}

// TableColumns reads one table's columns, primary key and the unique
// indexes that can stand in for it (§10.1): all their columns not null,
// none an expression. An auto_increment column's default is
// auto_increment; an empty literal one is a pair of quotes, so it shows.
func (Engine) TableColumns(ctx context.Context, c db.Conn, schema, table string) (db.Columns, error) {
	var out db.Columns
	r, err := c.Query(ctx, `
select column_name, column_type, is_nullable = 'NO', column_default, extra
from information_schema.columns
where table_schema = ? and table_name = ?
order by ordinal_position`, db.Val{S: schema}, db.Val{S: table})
	if err != nil {
		return out, err
	}
	for _, row := range r.Rows {
		col := db.Column{Name: row[0].S, Type: row[1].S, NotNull: row[2].S == "1", Default: row[3].S, Enum: enumOf(row[1].S)}
		switch {
		case strings.Contains(row[4].S, "auto_increment"):
			col.Default = "auto_increment"
		case !row[3].Null && row[3].S == "":
			col.Default = "''"
		}
		out.Cols = append(out.Cols, col)
	}
	// PRIMARY first, then the unique indexes by name: STATISTICS knows no
	// order they were made in. An expression's column_name is NULL.
	r, err = c.Query(ctx, `
select s.index_name, s.column_name, coalesce(c.is_nullable, 'YES')
from information_schema.statistics s
left join information_schema.columns c
  on c.table_schema = s.table_schema and c.table_name = s.table_name and c.column_name = s.column_name
where s.table_schema = ? and s.table_name = ? and s.non_unique = 0
order by s.index_name <> 'PRIMARY', s.index_name, s.seq_in_index`, db.Val{S: schema}, db.Val{S: table})
	if err != nil {
		return out, err
	}
	for i := 0; i < len(r.Rows); {
		name, cols, ok := r.Rows[i][0].S, []string(nil), true
		for ; i < len(r.Rows) && r.Rows[i][0].S == name; i++ {
			cols, ok = append(cols, r.Rows[i][1].S), ok && !r.Rows[i][1].Null && r.Rows[i][2].S == "NO"
		}
		switch {
		case name == "PRIMARY":
			out.PK = cols
		case ok:
			out.Unique = append(out.Unique, cols)
		}
	}
	return out, nil
}

// enumOf is an enum's labels from its COLUMN_TYPE, enum('a','b'), a quote
// in one doubled; nil for any other type.
func enumOf(typ string) []string {
	rest, ok := strings.CutPrefix(typ, "enum(")
	if !ok {
		return nil
	}
	var labels []string
	for len(rest) > 1 && rest[0] == '\'' {
		var b strings.Builder
		i := 1
		for ; i < len(rest); i++ {
			if rest[i] == '\'' {
				if i+1 < len(rest) && rest[i+1] == '\'' {
					b.WriteByte('\'')
					i++
					continue
				}
				break
			}
			b.WriteByte(rest[i])
		}
		labels = append(labels, b.String())
		rest = strings.TrimPrefix(rest[min(i+1, len(rest)):], ",")
	}
	return labels
}

// DDL is a table's or a view's definition as SHOW CREATE TABLE gives it
// (§8.4).
func (e Engine) DDL(ctx context.Context, c db.Conn, schema, name string) (string, error) {
	r, err := c.Query(ctx, "show create table "+e.Quote(schema, name))
	if err != nil {
		return "", err
	}
	return r.Rows[0][1].S + ";", nil
}
