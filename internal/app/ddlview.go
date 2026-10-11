package app

import (
	"context"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"
	uv "github.com/charmbracelet/ultraviolet"

	"github.com/kackerx/sqlmux/internal/db"
	"github.com/kackerx/sqlmux/internal/sqlkit"
	"github.com/kackerx/sqlmux/internal/ui"
)

// ddlPopup is the DDL of a table, or what it says of one of its columns,
// over the grid (§7.5「查看 DDL」, F9.8): the table's in a box in the middle
// that scrolls, the column's in a small one under its cell that goes with the
// next key.
type ddlPopup struct {
	conn  *Conn
	table db.Table
	col   string      // the column's name; "" for the table's
	at    uv.Position // the column's cell
	top   int
}

// ddlColumnRows is the most rows the column's box has; what is more is cut.
const ddlColumnRows = 8

func inDDL(a *App) bool { return a.ddl != nil }

// openDDL is grid.ddl.table (column false) and grid.ddl.column on the focused
// table: the DDL the palette's preview keeps, fetched when it is not there yet.
// The table's needs a table tab; the column's a column to stand on, so a page
// read (F9.8).
func (a *App) openDDL(column bool) tea.Cmd {
	p := a.focused()
	t := dataOf(p)
	if t == nil {
		return nil
	}
	d := &ddlPopup{conn: t.conn, table: t.table}
	if column {
		gs, g, area, ok := a.gridOf(p)
		if !ok {
			return nil
		}
		d.col, d.at = t.page.Cols[t.shownCols()[gs.col]].Name, g.CellRect(area).Min
	}
	a.ddl = d
	return a.askDDL(t.conn, t.table)
}

// askDDL fetches t's DDL on its connection's Meta unless it is cached or on
// its way.
func (a *App) askDDL(c *Conn, t db.Table) tea.Cmd {
	if _, cached := c.ddl[idOf(t)]; cached {
		return nil
	}
	c.ddl[idOf(t)] = ddlText{}
	return a.ddlCmd(context.Background(), c, t)
}

// wantPopupDDL asks again for the DDL of an open popup whose cache is gone (R
// drops it, a fetch out of time is not kept), as often as that happens (F9.8).
func (a *App) wantPopupDDL() tea.Cmd {
	if d := a.ddl; d != nil {
		return a.askDDL(d.conn, d.table)
	}
	return nil
}

// ddlContent is what the popup shows, without its scroll, and its title: "…"
// in dim until the DDL is in, the error as it came, the DDL in SQL's colors
// (the palette's preview has the same).
func (a *App) ddlContent() (ui.TextView, string) {
	d := a.ddl
	c, t := d.conn, d.table
	title := t.Schema + "." + t.Name
	got := c.ddl[idOf(t)]
	switch {
	case !got.got:
		return ui.TextView{Text: "…", Fg: a.theme.Dim}, title
	case got.err != "":
		return ui.TextView{Text: strings.ReplaceAll(got.err, "\n", " ")}, title
	}
	text, names := got.text, a.ddlNames(c, t, got.text)
	if d.col != "" {
		cat := db.Column{}
		for _, x := range c.cols[idOf(t)].Cols {
			if x.Name == d.col {
				cat = x
			}
		}
		title, text = d.col, strings.Join(ddlColumnLines(got.text, d.col, cat, c.eng.Dialect()), "\n")
	}
	return ui.TextView{Text: text, Colors: ui.SQLColors(a.theme, text, c.eng.Dialect(), names)}, title
}

// popupBox is the box in the middle of the window the table's DDL and a
// value's editor are in: two thirds of it each way, 40 by 8 at least.
func (a *App) popupBox() uv.Rectangle {
	win := a.window()
	w, h := max(win.Dx()*2/3, min(win.Dx(), 40)), max(win.Dy()*2/3, min(win.Dy(), 8))
	return uv.Rect(win.Min.X+(win.Dx()-w)/2, win.Min.Y+(win.Dy()-h)/2, w, h)
}

// ddlRect is where the popup is: the table's in the middle, two thirds of the
// window each way and 40 by 8 at least; the column's under its cell, or over
// it, as wide as its text.
func (a *App) ddlRect(v ui.TextView) uv.Rectangle {
	win := a.window()
	if a.ddl.col == "" {
		return a.popupBox()
	}
	w := 0
	for _, l := range strings.Split(v.Text, "\n") {
		w = max(w, ui.Width(l))
	}
	box, _ := ui.CompleteBox(win, a.ddl.at, min(w+4, max(win.Dx()*2/3, 20)), min(v.Lines(), ddlColumnRows), 0, ui.CompleteRows)
	return box
}

// drawDDL paints the popup over the frame; a column's that is long is cut by
// its box, with the ellipsis the view puts there, not scrolled.
func (a *App) drawDDL(f *ui.Frame) {
	v, title := a.ddlContent()
	v.Top = a.ddl.top
	ui.TextBox{Title: title, View: v}.Draw(f, a.ddlRect(v))
}

// scrollDDL moves the table's popup by d rows, or d half pages, from where it
// is held now, to the ends and no further.
func (a *App) scrollDDL(d int, pages bool) {
	v, _ := a.ddlContent()
	rows := ui.TextBox{}.Inner(a.ddlRect(v)).Dy()
	if pages {
		d *= max(rows/2, 1)
	}
	v.Top = a.ddl.top
	a.ddl.top = v.Scroll(rows, d)
}

// ddlEdge is gg (bottom false) and G: the first rows, or the last.
func (a *App) ddlEdge(bottom bool) {
	v, _ := a.ddlContent()
	a.ddl.top = 0
	if bottom {
		a.ddl.top = v.Scroll(ui.TextBox{}.Inner(a.ddlRect(v)).Dy(), v.Lines())
	}
}

// ddlItem is a token of a DDL statement and its text.
type ddlItem struct {
	kind       sqlkit.Kind
	text       string
	start, end int
}

// ddlTokens are the tokens of statement s that are neither a blank nor a comment.
func ddlTokens(s string, d sqlkit.Dialect) (out []ddlItem) {
	for _, t := range sqlkit.Scan(s, d) {
		if t.Kind != sqlkit.Space && t.Kind != sqlkit.Comment {
			out = append(out, ddlItem{t.Kind, s[t.Start:t.End], t.Start, t.End})
		}
	}
	return out
}

// is reports whether it is the unquoted word w, in any case.
func (t ddlItem) is(w string) bool {
	return (t.kind == sqlkit.Ident || t.kind == sqlkit.Keyword) && strings.EqualFold(t.text, w)
}

// name is the name it is, a quoted one without its quotes; "" for a token
// that is no name.
func (t ddlItem) name() string {
	if t.kind == sqlkit.Quoted {
		q := t.text[:1]
		return strings.ReplaceAll(t.text[1:len(t.text)-1], q+q, q)
	}
	if t.kind == sqlkit.Ident || t.kind == sqlkit.Keyword {
		return t.text
	}
	return ""
}

// mentions is whether the tokens ts name col, as a name and not as a string or
// a part of a longer name; before a ( it is a function called, in an index's
// expression, but a key's column with its prefix length (calls false, true).
func mentions(ts []ddlItem, col string, prefix bool) bool {
	for i, t := range ts {
		if t.name() == col && (prefix || i+1 == len(ts) || ts[i+1].text != "(") {
			return true
		}
	}
	return false
}

// ddlColumnLines is what a table's DDL says of column col, for its popup
// (F9.8): the column's own line, then the statements and lines that make an
// index or a key of it (CREATE INDEX, PG's primary key and unique
// constraints, MySQL's KEY lines), then its comment on. Names are compared as
// the DDL spells them, for both engines put the catalog's in it. A column the
// DDL has no line for, a view's, has its name and what the catalog says of it.
func ddlColumnLines(ddl, col string, cat db.Column, d sqlkit.Dialect) []string {
	var own string
	var keys, notes []string
	for _, st := range sqlkit.Statements(ddl, d) {
		text := ddl[st.Start:st.End]
		ts := ddlTokens(text, d)
		switch {
		case len(ts) >= 3 && ts[0].is("comment") && ts[1].is("on") && ts[2].is("column"):
			var target []ddlItem // schema.table.column up to is
			for _, t := range ts[3:] {
				if t.is("is") {
					break
				}
				if t.name() != "" {
					target = append(target, t)
				}
			}
			if n := len(target); n > 0 && target[n-1].name() == col {
				notes = append(notes, text+";")
			}
		case len(ts) >= 2 && ts[0].is("create") && ts[1].is("table"):
			var items []string
			own, items = tableLines(text, ts, col, d)
			keys = append(keys, items...)
		case len(ts) >= 2 && ts[0].is("create"):
			if i := indexOn(ts); i >= 0 && mentions(afterParen(ts[i:]), col, false) {
				keys = append(keys, text+";")
			}
		}
	}
	if own == "" {
		own = col + " " + cat.Type
		if cat.NotNull {
			own += " not null"
		}
		if cat.Default != "" {
			own += " default " + cat.Default
		}
	}
	return append(append([]string{own}, keys...), notes...)
}

// indexOn is the position of the ON of a CREATE [UNIQUE] INDEX, -1 when ts is
// no such statement.
func indexOn(ts []ddlItem) int {
	isIndex := false
	for i, t := range ts {
		switch {
		case t.is("index"):
			isIndex = true
		case t.is("on") && isIndex:
			return i
		}
	}
	return -1
}

// afterParen is what follows the first ( of ts, to the end.
func afterParen(ts []ddlItem) []ddlItem {
	for i, t := range ts {
		if t.text == "(" && t.kind == sqlkit.Punct {
			return ts[i+1:]
		}
	}
	return nil
}

// tableLines goes through the items of a CREATE TABLE's parentheses: the line
// of column col, and the lines of the keys that name it.
func tableLines(text string, ts []ddlItem, col string, d sqlkit.Dialect) (own string, keys []string) {
	open := slices.IndexFunc(ts, func(t ddlItem) bool { return t.kind == sqlkit.Punct && t.text == "(" })
	if open < 0 {
		return "", nil
	}
	depth, from := 0, open+1
	emit := func(to int) {
		if to <= from {
			return
		}
		item := ts[from:to]
		line := strings.TrimSpace(text[item[0].start:item[len(item)-1].end])
		switch {
		case keyItem(item, d):
			if mentions(afterParen(item), col, true) {
				keys = append(keys, line)
			}
		case own == "" && item[0].name() == col:
			own = line
		}
	}
	for i := open; i < len(ts); i++ {
		if ts[i].kind != sqlkit.Punct {
			continue
		}
		switch ts[i].text {
		case "(":
			depth++
		case ")":
			if depth--; depth == 0 {
				emit(i)
				return own, keys
			}
		case ",":
			if depth == 1 {
				emit(i)
				from = i + 1
			}
		}
	}
	return own, keys
}

// keyItem is whether an item of a table's definition makes a key or an index:
// a constraint (both engines name them: constraint <name> primary key …) that
// is a primary key or unique, and in MySQL's the KEY lines, bare. PG's columns
// called key or index are columns: its definitions all begin with constraint.
func keyItem(item []ddlItem, d sqlkit.Dialect) bool {
	w := item[0]
	if w.is("constraint") {
		if len(item) < 3 {
			return false
		}
		w = item[2]
	}
	return w.is("primary") || w.is("unique") ||
		d == sqlkit.MySQL && (w.is("key") || w.is("index") || w.is("fulltext") || w.is("spatial"))
}
