package app

import (
	"cmp"
	"strings"

	"github.com/kackerx/sqlmux/internal/db"
	"github.com/kackerx/sqlmux/internal/sqlkit"
	"github.com/kackerx/sqlmux/internal/ui"
)

// signature is the line of fields for what a value typed at query t's cursor
// is for (§11「签名提示与值补全」, F15.3): an INSERT's columns with the one
// being filled in lit, the column of an UPDATE's SET or of a condition, a
// user function's arguments; none where the cursor is in no such place, or
// `[query] hints` is off. A call the catalog does not know leaves the place
// it is an argument of.
// ponytail: the text is scanned once a frame for the focused query, as the
// colors are; keep the context per change if a big file lags
func (a *App) signature(t *queryTab) ui.Signature {
	if !a.valueHints {
		return ui.Signature{}
	}
	lines, cur := t.ed.Lines(), t.ed.Cursor()
	v := sqlkit.ValueContext(strings.Join(lines, "\n"), offsetOf(lines, cur), t.conn.eng.Dialect())
	for schema := cmp.Or(t.schema, t.conn.Schema); v.OK; v = *v.Outer {
		if s := t.conn.fields(schema, v); len(s.Fields) > 0 {
			return s
		}
		if v.Outer == nil {
			break
		}
	}
	return ui.Signature{}
}

// fields is the signature of v on c, schema the one the statement runs in; a
// column's type is the cache's, so it is missing until the columns are in.
// ponytail: a function is found by its name in any schema when the call has
// none, the first of them
func (c *Conn) fields(schema string, v sqlkit.Value) ui.Signature {
	switch v.Kind {
	case sqlkit.ValueInsert:
		var have []db.Column
		if len(v.Tables) > 0 {
			if t, ok := c.table(cmp.Or(v.Tables[0].Schema, schema), v.Tables[0].Name); ok {
				have = c.cols[idOf(t)].Cols
			}
		}
		names := v.Cols
		if len(names) == 0 { // no list: the columns in the table's order
			for _, col := range have {
				names = append(names, col.Name)
			}
		}
		if v.Index >= len(names) { // more values than columns: no field to name
			break
		}
		s := ui.Signature{Cur: v.Index}
		for _, n := range names {
			s.Fields = append(s.Fields, field(n, have))
		}
		return s
	case sqlkit.ValueUpdate, sqlkit.ValueWhere:
		for _, r := range v.Tables {
			if v.Qualifier != "" && !strings.EqualFold(v.Qualifier, cmp.Or(r.Alias, r.Name)) && !strings.EqualFold(v.Qualifier, r.Name) {
				continue
			}
			if t, ok := c.table(cmp.Or(r.Schema, schema), r.Name); ok {
				for _, col := range c.cols[idOf(t)].Cols {
					if strings.EqualFold(col.Name, v.Cols[0]) {
						return ui.Signature{Fields: []string{field(col.Name, []db.Column{col})}}
					}
				}
			}
		}
	case sqlkit.ValueCall:
		for _, f := range c.funcs { // the first overload that has the argument
			args := splitArgs(f.Args)
			if strings.EqualFold(f.Name, v.Func.Name) && (v.Func.Schema == "" || strings.EqualFold(f.Schema, v.Func.Schema)) && v.Index < len(args) {
				return ui.Signature{Fields: args, Cur: v.Index}
			}
		}
	}
	return ui.Signature{}
}

// field is "name type" for the column named name among have, the name alone
// for one the cache lacks.
func field(name string, have []db.Column) string {
	for _, col := range have {
		if strings.EqualFold(col.Name, name) && col.Type != "" {
			return name + " " + col.Type
		}
	}
	return name
}

// splitArgs is the arguments of a function's list, "a integer, b numeric(10,2)",
// split at its commas outside parentheses.
func splitArgs(args string) (out []string) {
	depth, from := 0, 0
	for i, r := range args {
		switch r {
		case '(':
			depth++
		case ')':
			depth--
		case ',':
			if depth == 0 {
				out, from = append(out, strings.TrimSpace(args[from:i])), i+1
			}
		}
	}
	if rest := strings.TrimSpace(args[from:]); rest != "" {
		out = append(out, rest)
	}
	return out
}
