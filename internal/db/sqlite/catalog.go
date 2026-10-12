package sqlite

import (
	"context"
	"slices"
	"strconv"
	"strings"

	"github.com/kackerx/sqlmux/internal/db"
)

// Catalog queries (§8.4), over sqlite_schema and the pragma table functions.
// The file is the one schema, main; an attached database is none of ours.

const schema = "main"

// Schemas is main alone, the one unqualified names go to.
func (Engine) Schemas(context.Context, db.Conn) (schemas []string, current string, err error) {
	return []string{schema}, schema, nil
}

// Tables lists the file's tables and views by name, but SQLite's own
// (sqlite_sequence, sqlite_stat1…); SQLite keeps no row counts.
func (Engine) Tables(ctx context.Context, c db.Conn) ([]db.Table, error) {
	r, err := c.Query(ctx, `
select name, type from sqlite_schema
where type in ('table', 'view') and name not like 'sqlite\_%' escape '\'
order by name`)
	if err != nil {
		return nil, err
	}
	ts := make([]db.Table, len(r.Rows))
	for i, row := range r.Rows {
		kind := "r" // pg_class's relkind, which db.Table goes by
		if row[1].S == "view" {
			kind = "v"
		}
		ts[i] = db.Table{Schema: schema, Name: row[0].S, Rows: -1, Kind: kind}
	}
	return ts, nil
}

// TableColumns reads one table's columns, its primary key and the unique
// indexes that can stand in for it (§10.1). A table with neither is keyed by
// its rowid, which select * does not give: Lead names it, and the grid shows
// it as the first column, one that cannot be set; a WITHOUT ROWID table has a
// primary key, a view none. A column is Generated when it is a generated one;
// an INTEGER PRIMARY KEY is the rowid's name, its own to fill in (default
// rowid); a DEFAULT is for a row added only, an UPDATE has none in SQLite.
func (Engine) TableColumns(ctx context.Context, c db.Conn, _, table string) (db.Columns, error) {
	var out db.Columns
	name := db.Val{S: table}
	r, err := c.Query(ctx, `select name, type, "notnull", dflt_value, pk, hidden from pragma_table_xinfo(?) order by cid`, name)
	if err != nil {
		return out, err
	}
	type key struct {
		n    int
		name string
	}
	var pk []key
	for _, row := range r.Rows {
		if row[5].S == "1" { // a virtual table's hidden column
			continue
		}
		col := db.Column{Name: row[0].S, Type: row[1].S, NotNull: row[2].S == "1", Default: row[3].S, AddOnly: true, Generated: row[5].S == "2" || row[5].S == "3"}
		if n, _ := strconv.Atoi(row[4].S); n > 0 {
			pk = append(pk, key{n, col.Name})
		}
		out.Cols = append(out.Cols, col)
	}
	slices.SortFunc(pk, func(a, b key) int { return a.n - b.n })
	for _, k := range pk {
		out.PK = append(out.PK, k.name)
	}
	tbl, err := c.Query(ctx, `select type, wr from pragma_table_list(?) where schema = 'main'`, name)
	if err != nil {
		return out, err
	}
	rowid := len(tbl.Rows) > 0 && tbl.Rows[0][0].S == "table" && tbl.Rows[0][1].S == "0"
	if i := slices.IndexFunc(out.Cols, func(col db.Column) bool { return len(out.PK) == 1 && col.Name == out.PK[0] }); i >= 0 && rowid && strings.EqualFold(out.Cols[i].Type, "integer") {
		out.Cols[i].Default = "rowid"
	}
	// the unique indexes over not-null columns, none an expression
	idx, err := c.Query(ctx, `select name from pragma_index_list(?) where "unique" = 1 and origin <> 'pk' order by seq`, name)
	if err != nil {
		return out, err
	}
	for _, row := range idx.Rows {
		cols, err := c.Query(ctx, `select name from pragma_index_info(?) order by seqno`, row[0])
		if err != nil {
			return out, err
		}
		var names []string
		ok := true
		for _, cr := range cols.Rows {
			i := slices.IndexFunc(out.Cols, func(col db.Column) bool { return !cr[0].Null && col.Name == cr[0].S })
			ok = ok && i >= 0 && out.Cols[i].NotNull
			names = append(names, cr[0].S)
		}
		if ok {
			out.Unique = append(out.Unique, names)
		}
	}
	if out.Key() == nil && rowid && !slices.ContainsFunc(out.Cols, func(col db.Column) bool { return strings.EqualFold(col.Name, "rowid") }) {
		out.Lead, out.PK = "rowid, ", []string{"rowid"}
		out.Cols = slices.Insert(out.Cols, 0, db.Column{Name: "rowid", Type: "integer", NotNull: true, Generated: true})
	}
	return out, nil
}

// DDL is a table's or a view's definition as sqlite_schema keeps it (§8.4).
func (Engine) DDL(ctx context.Context, c db.Conn, _, name string) (string, error) {
	r, err := c.Query(ctx, "select sql from sqlite_schema where name = ?", db.Val{S: name})
	if err != nil || len(r.Rows) == 0 {
		return "", err
	}
	return r.Rows[0][0].S + ";", nil
}
