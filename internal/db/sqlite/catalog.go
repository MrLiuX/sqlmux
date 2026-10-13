package sqlite

import (
	"context"
	"slices"
	"strconv"
	"strings"

	"github.com/kackerx/sqlmux/internal/db"
	"github.com/kackerx/sqlmux/internal/i18n"
)

// Catalog queries (§8.4), over sqlite_schema and the pragma table functions.
// The file is the one schema, main; an attached database is none of ours.

const schema = "main"

// Schemas is main alone, the one unqualified names go to.
func (Engine) Schemas(context.Context, db.Conn) (schemas []string, current string, err error) {
	return []string{schema}, schema, nil
}

// Tables lists the file's tables and views by name, but SQLite's own
// (sqlite_sequence, sqlite_stat1…) and the shadow tables of a virtual one, an
// FTS5's; SQLite keeps no row counts.
func (Engine) Tables(ctx context.Context, c db.Conn) ([]db.Table, error) {
	r, err := c.Query(ctx, `
select name, type from sqlite_schema
where type in ('table', 'view') and name not like 'sqlite\_%' escape '\'
  and name not in (select name from pragma_table_list where type = 'shadow')
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
		if col.Generated {
			col.ReadOnly = i18n.T("%s 是生成列，只读", col.Name)
		}
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
	// the unique indexes over not-null columns, none an expression, none partial; an
	// index of origin pk is a primary key that is no alias of the rowid
	idx, err := c.Query(ctx, `select name, origin from pragma_index_list(?) where "unique" = 1 and partial = 0 order by seq`, name)
	if err != nil {
		return out, err
	}
	aliased := true // the primary key is the rowid's own name: an INTEGER one, with no index of its own (not DESC, not WITHOUT ROWID)
	for _, row := range idx.Rows {
		if row[1].S == "pk" {
			aliased = false
			continue
		}
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
	alias := false // INTEGER PRIMARY KEY, the rowid's name: never NULL, though not declared NOT NULL
	if i := slices.IndexFunc(out.Cols, func(col db.Column) bool { return len(out.PK) == 1 && col.Name == out.PK[0] }); i >= 0 && rowid && aliased && strings.EqualFold(out.Cols[i].Type, "integer") {
		out.Cols[i].Default = "rowid"
		alias = true
	}
	// a primary key of a rowid table may hold NULLs unless declared NOT NULL, and
	// = finds no row by one: it is no identity, a unique index or the rowid is
	if rowid && !alias && slices.ContainsFunc(out.PK, func(n string) bool {
		return !out.Cols[slices.IndexFunc(out.Cols, func(col db.Column) bool { return col.Name == n })].NotNull
	}) {
		out.PK = nil
	}
	for _, k := range out.Key() { // a blob's bytes are no text to find a row by (MySQL's too)
		for _, col := range out.Cols {
			if col.Name == k && strings.Contains(strings.ToLower(col.Type), "blob") {
				out.ReadOnly = i18n.T("行标识列 %s 是 %s", k, col.Type)
			}
		}
	}
	if out.Key() == nil && rowid && !slices.ContainsFunc(out.Cols, func(col db.Column) bool { return strings.EqualFold(col.Name, "rowid") }) {
		out.Lead, out.PK = "rowid, ", []string{"rowid"}
		out.Cols = slices.Insert(out.Cols, 0, db.Column{Name: "rowid", Type: "integer", NotNull: true, Generated: true, ReadOnly: i18n.T("rowid 是行号，只读")})
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

// Indexes lists a table's indexes by name, but those SQLite makes of a
// PRIMARY KEY or UNIQUE of the table (sqlite_autoindex_…), which no statement
// made; Def is the statement sqlite_schema keeps (F14.3). An expression in a
// key is "expr".
func (Engine) Indexes(ctx context.Context, c db.Conn, _, table string) ([]db.Index, error) {
	r, err := c.Query(ctx, `
select l.name, l."unique", coalesce(s.sql, '')
from pragma_index_list(?) l left join sqlite_schema s on s.type = 'index' and s.name = l.name
where l.name not like 'sqlite\_autoindex\_%' escape '\'
order by l.name`, db.Val{S: table})
	if err != nil {
		return nil, err
	}
	var out []db.Index
	for _, row := range r.Rows {
		ix := db.Index{Name: row[0].S, Unique: row[1].S == "1", Def: row[2].S + ";"}
		cols, err := c.Query(ctx, `select coalesce(name, 'expr') from pragma_index_info(?) order by seqno`, row[0])
		if err != nil {
			return nil, err
		}
		for _, cr := range cols.Rows {
			ix.Cols = append(ix.Cols, cr[0].S)
		}
		out = append(out, ix)
	}
	return out, nil
}

// Keys lists a table's primary key, then its foreign keys, which SQLite does
// not name, by the order it lists them in (F14.3). A foreign key with no
// column written points at the parent's primary key.
func (Engine) Keys(ctx context.Context, c db.Conn, _, table string) ([]db.Key, error) {
	var out []db.Key
	pk, err := c.Query(ctx, `select name from pragma_table_info(?) where pk > 0 order by pk`, db.Val{S: table})
	if err != nil {
		return nil, err
	}
	if len(pk.Rows) > 0 {
		k := db.Key{Name: "PRIMARY"}
		for _, row := range pk.Rows {
			k.Cols = append(k.Cols, row[0].S)
		}
		out = append(out, k)
	}
	fk, err := c.Query(ctx, `select id, "table", "from", "to" from pragma_foreign_key_list(?) order by id, seq`, db.Val{S: table})
	if err != nil {
		return nil, err
	}
	id := "" // the foreign key the rows are of
	for _, row := range fk.Rows {
		if row[0].S != id {
			id = row[0].S
			out = append(out, db.Key{Ref: &db.Ref{Schema: schema, Table: row[1].S}})
		}
		k := &out[len(out)-1]
		k.Cols = append(k.Cols, row[2].S)
		if !row[3].Null {
			k.Ref.Cols = append(k.Ref.Cols, row[3].S)
		}
	}
	for i := range out {
		if k := out[i]; k.Ref != nil && len(k.Ref.Cols) == 0 { // the parent's primary key
			parent, err := c.Query(ctx, `select name from pragma_table_info(?) where pk > 0 order by pk`, db.Val{S: k.Ref.Table})
			if err != nil {
				return nil, err
			}
			for _, row := range parent.Rows {
				out[i].Ref.Cols = append(out[i].Ref.Cols, row[0].S)
			}
		}
	}
	return out, nil
}

// Databases is none: a SQLite file is the one database (§5).
func (Engine) Databases(context.Context, db.Conn) ([]string, error) { return nil, nil }

// Functions, Sequences and FunctionDef are none: SQLite has no stored functions or sequences.
func (Engine) Functions(context.Context, db.Conn) ([]db.Function, error)         { return nil, nil }
func (Engine) Sequences(context.Context, db.Conn) ([]db.Sequence, error)         { return nil, nil }
func (Engine) FunctionDef(context.Context, db.Conn, db.Function) (string, error) { return "", nil }
