package app

import (
	"cmp"
	"context"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/kackerx/sqlmux/internal/db"
	"github.com/kackerx/sqlmux/internal/i18n"
)

// The objects the tree lists besides tables, views and columns (F14.3, §7.8
// 「对象节点」): a table's indexes and keys, fetched with its columns when it
// opens; a schema's functions and sequences, with the catalog.

// objectsMsg is a table's indexes and keys, as fetchObjects read them.
type objectsMsg struct {
	conn  *Conn
	table db.Table
	idx   []db.Index
	keys  []db.Key
	err   error
}

// fetchObjects reads t's indexes and keys on its connection's Meta.
func (a *App) fetchObjects(c *Conn, t db.Table) tea.Cmd {
	meta, eng := c.Meta, c.eng
	return func() tea.Msg {
		ctx := context.Background()
		idx, err := eng.Indexes(ctx, meta, t.Schema, t.Name)
		if err != nil {
			return objectsMsg{conn: c, table: t, err: err}
		}
		keys, err := eng.Keys(ctx, meta, t.Schema, t.Name)
		return objectsMsg{conn: c, table: t, idx: idx, keys: keys, err: err}
	}
}

// gotObjects keeps what fetchObjects read, in the tree from now on; a table's
// that failed shows none, and says why.
func (a *App) gotObjects(m objectsMsg) tea.Cmd {
	if a.sessionOf(m.conn) == nil {
		return nil
	}
	if m.err != nil {
		return a.showToast(i18n.T("%s：%s", m.conn.Spec.Name, m.err))
	}
	m.conn.idx[idOf(m.table)], m.conn.keys[idOf(m.table)] = m.idx, m.keys
	a.clampTree()
	return nil
}

// indexText is an index as the tree shows it: `name (a, b) UNIQUE`.
func indexText(ix db.Index) string {
	s := ix.Name + " (" + strings.Join(ix.Cols, ", ") + ")"
	if ix.Unique {
		return s + " UNIQUE"
	}
	return s
}

// keyText is a key as the tree shows it: `PRIMARY (id)`, `fk_name (a) →
// schema.table (b)`; a foreign key with no name, SQLite's, is a FOREIGN KEY, the
// table of a file with no schema before it.
func keyText(c *Conn, k db.Key) string {
	s := cmp.Or(k.Name, "FOREIGN KEY") + " (" + strings.Join(k.Cols, ", ") + ")"
	if k.Ref == nil {
		return s
	}
	target := k.Ref.Table
	if c.Spec.Engine != "sqlite" {
		target = k.Ref.Schema + "." + target
	}
	return s + " → " + target + " (" + strings.Join(k.Ref.Cols, ", ") + ")"
}

// functionText is a function as the tree shows it: its name and signature.
func functionText(f db.Function) string { return f.Name + "(" + f.Args + ")" }

// openIndexDDL is ↵ on an index: the DDL popup of the table says the
// statement that makes the index instead (F14.3).
func (a *App) openIndexDDL(c *Conn, t db.Table, ix db.Index) {
	a.ddl = &ddlPopup{conn: c, table: t, text: ix.Def, label: ix.Name}
}

// openRef is ↵ on a foreign key: the table it points at opens, its schema and
// Tables opened in the tree first when it is another one's; one that is not
// in the catalog says so (F14.3).
func (a *App) openRef(c *Conn, ref *db.Ref) tea.Cmd {
	for _, t := range c.Tables {
		if idOf(t) == (tableID{ref.Schema, ref.Table}) {
			for _, id := range []string{connNode(c), schemaNode(c, t.Schema), groupNode(c, t.Schema, t.View())} {
				a.sess.open[id] = true
			}
			return a.openTable(c, t)
		}
	}
	return a.showToast(i18n.T("%s 里没有表 %s", c.Spec.Name, ref.Schema+"."+ref.Table))
}

// funcMsg is the definition of a function, as FunctionDef read it.
type funcMsg struct {
	conn *Conn
	fn   db.Function
	text string
	err  error
}

// openFunction is ↵ on a function: its definition is read, then put in a
// new query on the connection, in the function's schema (F14.3).
func (a *App) openFunction(c *Conn, f db.Function) tea.Cmd {
	meta, eng := c.Meta, c.eng
	return func() tea.Msg {
		text, err := eng.FunctionDef(context.Background(), meta, f)
		return funcMsg{c, f, text, err}
	}
}

func (a *App) gotFunction(m funcMsg) tea.Cmd {
	if a.sessionOf(m.conn) == nil {
		return nil
	}
	if m.err != nil {
		return a.showToast(i18n.T("%s：%s", m.conn.Spec.Name, m.err))
	}
	return a.queryWith(m.conn, m.fn.Schema, m.text)
}

// queryWith opens a new query on c, in schema, holding text.
func (a *App) queryWith(c *Conn, schema, text string) tea.Cmd {
	kept, ok := a.newQuery(c)
	if !ok {
		return kept
	}
	t := queryOf(a.focused())
	t.schema = cmp.Or(schema, t.schema)
	return tea.Batch(kept, a.queryDid(t, t.ed.Load(text)), a.keepQuery(t))
}
