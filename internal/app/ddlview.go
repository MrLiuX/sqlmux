package app

import (
	"context"
	"image/color"
	"regexp"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"
	uv "github.com/charmbracelet/ultraviolet"

	"sqlmux/internal/db"
	"sqlmux/internal/ui"
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
func (a *App) openDDL(column bool) tea.Cmd {
	p := a.focused()
	t := dataOf(p)
	if t == nil || t.page.Cols == nil {
		return nil
	}
	d := &ddlPopup{conn: t.conn, table: t.table}
	if column {
		gs, g, area, ok := a.gridOf(p)
		if !ok {
			return nil
		}
		d.col, d.at = t.page.Cols[t.shownCols()[gs.col]].Name, g.EditRect(area).Min
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

// ddlContent is what the popup shows: its text, "…" until the DDL is in, in
// SQL's colors, none for an error; and the title.
func (a *App) ddlContent() (text string, colors []color.Color, title string) {
	d := a.ddl
	c, t := d.conn, d.table
	title = t.Schema + "." + t.Name
	got := c.ddl[idOf(t)]
	switch {
	case !got.got:
		return "…", nil, title
	case got.err != "":
		return strings.ReplaceAll(got.err, "\n", " "), nil, title
	}
	text = got.text
	var names ui.SQLNames
	switch cols := c.cols[idOf(t)]; {
	case d.col != "":
		cat := db.Column{}
		if i := slices.IndexFunc(cols.Cols, func(x db.Column) bool { return x.Name == d.col }); i >= 0 {
			cat = cols.Cols[i]
		}
		title, text = d.col, strings.Join(ddlColumnLines(got.text, d.col, cat), "\n")
	case t.View():
		names, _ = a.sqlNames(c, text, t.Schema, nil)
	default:
		names, _ = a.sqlNames(c, text, "", &cols)
	}
	return text, ui.SQLColors(a.theme, text, c.eng.Dialect(), names), title
}

// ddlRect is where the popup is: the table's in the middle, two thirds of the
// window each way; the column's under its cell, or over it, as wide as its text.
func (a *App) ddlRect(text string) uv.Rectangle {
	win := a.window()
	if a.ddl.col == "" {
		w, h := max(win.Dx()*2/3, min(win.Dx(), 40)), max(win.Dy()*2/3, min(win.Dy(), 8))
		return uv.Rect(win.Min.X+(win.Dx()-w)/2, win.Min.Y+(win.Dy()-h)/2, w, h)
	}
	w := 0
	for _, l := range strings.Split(text, "\n") {
		w = max(w, ui.Width(l))
	}
	box, _ := ui.CompleteBox(win, a.ddl.at, min(w+4, max(win.Dx()*2/3, 20)), min(strings.Count(text, "\n")+1, ddlColumnRows), 0)
	return box
}

// drawDDL paints the popup over the frame.
func (a *App) drawDDL(f *ui.Frame) {
	text, colors, title := a.ddlContent()
	d := a.ddl
	if d.col != "" { // a long one is cut, not scrolled: the table's DDL has the rest
		if lines := strings.Split(text, "\n"); len(lines) > ddlColumnRows {
			text = strings.Join(lines[:ddlColumnRows-1], "\n") + "\n…" // its colors are the whole text's, whose start it is
		}
	}
	box := a.ddlRect(text)
	ui.TextBox{Title: title, View: ui.TextView{Text: text, Colors: colors, Top: d.top}}.Draw(f, box)
}

// scrollDDL moves the table's popup by d rows, or d half pages, to the ends and no
// further; the top and the bottom are d -1 and 1 of "the whole text".
func (a *App) scrollDDL(d int, pages bool) {
	text, _, _ := a.ddlContent()
	rows := ui.TextBox{}.Inner(a.ddlRect(text)).Dy()
	if pages {
		d *= max(rows/2, 1)
	}
	a.ddl.top = max(min(a.ddl.top+d, strings.Count(text, "\n")+1-rows), 0)
}

// ddlEdge is gg (false) and G: the first rows, or the last.
func (a *App) ddlEdge(bottom bool) {
	text, _, _ := a.ddlContent()
	a.ddl.top = 0
	if bottom {
		a.ddl.top = strings.Count(text, "\n") + 1
		a.scrollDDL(0, false)
	}
}

// ddlColumnLines is what a table's DDL says of column col, for its popup
// (F9.8): the column's own line, then the lines that make an index or a key of
// it, then its comment. A column the DDL has no line for, a view's, has its
// name and what the catalog says of it.
func ddlColumnLines(ddl, col string, c db.Column) []string {
	mention := regexp.MustCompile("(?i)(^|[^\\w])[\"`]?" + regexp.QuoteMeta(col) + "[\"`]?([^\\w]|$)")
	var line string
	var rest, notes []string
	for _, l := range strings.Split(ddl, "\n") {
		t := strings.TrimSpace(l)
		low := strings.ToLower(t)
		switch {
		case strings.HasPrefix(low, "create ") && strings.Contains(low, " index ") || isKeyLine(low):
			if i := strings.Index(t, "("); i >= 0 && mention.MatchString(t[i:]) {
				rest = append(rest, strings.TrimSuffix(t, ","))
			}
		case strings.HasPrefix(low, "comment on column "):
			// the column's name is what follows the last dot before " is "
			if target, _, ok := strings.Cut(t, " is "); ok {
				if i := strings.LastIndex(target, "."); i >= 0 && mention.MatchString(target[i:]) {
					notes = append(notes, t)
				}
			}
		case strings.HasPrefix(l, "  ") && !strings.HasPrefix(l, "   ") && line == "":
			if name := firstName(t); strings.EqualFold(name, col) {
				line = strings.TrimSuffix(t, ",")
			}
		}
	}
	if line == "" {
		line = col + " " + c.Type
		if c.NotNull {
			line += " not null"
		}
		if c.Default != "" {
			line += " default " + c.Default
		}
	}
	return append(append([]string{line}, rest...), notes...)
}

// isKeyLine is whether a line of a table's definition, lower case and trimmed,
// makes a key or an index: PG's primary key and unique constraints, MySQL's
// KEY lines.
func isKeyLine(low string) bool {
	if strings.HasPrefix(low, "constraint ") {
		return strings.Contains(low, " primary key") || strings.Contains(low, " unique")
	}
	for _, p := range []string{"primary key", "unique key", "unique index", "key ", "index ", "fulltext ", "spatial "} {
		if strings.HasPrefix(low, p) {
			return true
		}
	}
	return false
}

// firstName is the name a line of a table's definition starts with, the
// quotes of PG's and MySQL's off; "" for a line that starts with a word of the
// definition's (constraint, key …) and not a name.
func firstName(line string) string {
	if line == "" {
		return ""
	}
	if q := line[0]; q == '"' || q == '`' {
		if end := strings.IndexByte(line[1:], q); end >= 0 {
			return strings.ReplaceAll(line[1:1+end], string(q)+string(q), string(q))
		}
		return ""
	}
	word, _, _ := strings.Cut(line, " ")
	if isKeyLine(strings.ToLower(word)+" ") || strings.EqualFold(word, "constraint") || strings.EqualFold(word, "foreign") || strings.EqualFold(word, "check") {
		return ""
	}
	return word
}
