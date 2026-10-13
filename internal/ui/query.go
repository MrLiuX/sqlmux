package ui

import (
	"fmt"
	"image/color"
	"sort"
	"strconv"
	"strings"

	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"

	"github.com/kackerx/sqlmux/internal/sqlkit"
)

// Query is a query tab's text as its editor has it (§7.8, §9.2, §11):
// a gutter of ▶ on each statement's first line and the line numbers, then
// the SQL in its colors, not wrapped. In NORMAL the statement under the
// cursor, what ↵ runs, is on the row background, in VISUAL the selection
// on select; the / ? : line takes the last row.
// ponytail: the text is scanned twice a frame (colors, statements), linear in
// it: 1000 lines (80 KB) take 11 ms a frame, 10000 lines 86 ms; keep the
// tokens per change if a file that size lags.
type Query struct {
	Lines     []string
	TabWidth  int
	Top, Left int     // the first line and display column on screen
	Cursor    TextPos // the cursor: a line and a byte offset into it
	CursorCol int     // the display column the cursor is drawn at
	Normal    bool    // in NORMAL: the statement under the cursor shows
	Sel       Sel
	// Prompt and Text are the / ? : line being typed, its cursor at byte
	// Pos of Text; Prompt is "" when there is none.
	Prompt, Text string
	Pos          int
	// Hits are the matches of the last search on the lines in view, lit, and
	// Stat and StatRight the line a search left under the text (F15.9).
	Hits            []Span
	Stat, StatRight string
	Stops           []Span // the tabstops of a snippet still to come, on the visual color (F15.8)
	Failed          int    // the first line of the statement whose last run failed: its ▶ in error; -1 for none
	Pane            int
	Names           SQLNames       // the tables and columns among the text's names, for their colors; nil for none
	Dialect         sqlkit.Dialect // its connection's SQL
	Yank            Sel            // what a yank took, flashing (F3.32)
	// Colors are the text's own colors by byte, a value editor's (F12.2):
	// with them there are no statements and no ▶; Pane is ValuePane, so a click
	// on the text is the box's, not that of a query under it.
	Colors []color.Color
}

// Span is a match to light: bytes [From, To) of line Line, Current for the one
// the cursor is in.
type Span struct {
	Line, From, To int
	Current        bool
}

// ValuePane is the Pane of the value editor's text: no pane has it.
const ValuePane = -1

// TextPos is a line and a byte offset into it, both from 0.
type TextPos struct{ Line, Col int }

// Sel is what VISUAL has selected: the characters From to To, both in, in
// SelChars; their whole lines in SelLines; in SelBlock display columns
// Left to Right of those lines.
type Sel struct {
	Mode        SelMode
	From, To    TextPos
	Left, Right int
}

type SelMode uint8

const (
	SelNone SelMode = iota
	SelChars
	SelLines
	SelBlock
)

// gutter is ▶, the line numbers at 3 columns at least, a blank.
func (c Query) gutter() int { return 1 + max(3, len(strconv.Itoa(len(c.Lines)))) + 1 }

// Width is the columns the text takes whole, the gutter in: the longest line
// as drawn, a tab to its tabstop (F15.5).
func (c Query) Width() int {
	w := 0
	for _, l := range c.Lines {
		if !strings.Contains(l, "\t") {
			w = max(w, Width(l))
			continue
		}
		v := 0
		for i := 0; i < len(l); {
			gr, gw := ansi.FirstGraphemeCluster(l[i:], ansi.GraphemeWidth)
			if gw = max(gw, 1); l[i] == '\t' {
				gw = c.TabWidth - v%c.TabWidth
			}
			v, i = v+gw, i+len(gr)
		}
		w = max(w, v)
	}
	return c.gutter() + w
}

// TextArea is where the text goes in r: right of the gutter.
func (c Query) TextArea(r uv.Rectangle) uv.Rectangle {
	g := min(c.gutter(), r.Dx())
	return uv.Rect(r.Min.X+g, r.Min.Y, r.Dx()-g, r.Dy())
}

// Draw paints the query over r and returns where the terminal cursor
// goes; X is -1 when it is off the view.
func (c Query) Draw(f *Frame, r uv.Rectangle) uv.Position {
	th := f.Theme
	f.Fill(r, uv.Style{Fg: th.Fg, Bg: th.PaneBg})
	value := c.Colors != nil
	f.Region(r, Target{Kind: KindText, Pane: c.Pane})
	text := strings.Join(c.Lines, "\n")
	starts := make([]int, len(c.Lines)) // where each line is in text
	for i := 1; i < len(c.Lines); i++ {
		starts[i] = starts[i-1] + len(c.Lines[i-1]) + 1
	}
	lineOf := func(off int) int { return sort.SearchInts(starts, off+1) - 1 }
	first := map[int]bool{}
	cur := c.Cursor
	from, to := -1, -1 // the statement under the cursor, by lines
	colors := c.Colors
	if !value {
		stmts := sqlkit.Statements(text, c.Dialect)
		for _, s := range stmts {
			first[lineOf(s.Start)] = true
		}
		if i := sqlkit.StmtAt(text, stmts, starts[cur.Line]+cur.Col); i >= 0 && c.Normal {
			from, to = lineOf(stmts[i].Lead), lineOf(stmts[i].End) // with the comment lines above it (§9.2)
		}
		colors = SQLColors(th, text, c.Dialect, c.Names)
	}
	ta, width := c.TextArea(r), c.gutter()-2
	for y := range r.Dy() {
		n, row := c.Top+y, r.Min.Y+y
		if n >= len(c.Lines) {
			break
		}
		if first[n] { // click: run it (C-02)
			st := uv.Style{Fg: th.Focus, Bg: th.PaneBg}
			if n == c.Failed {
				st.Fg = th.Error
			}
			f.Text(r.Min.X, row, ta.Min.X, "▶", st)
			f.Region(uv.Rect(r.Min.X, row, 1, 1), Target{Kind: KindHint, Pane: c.Pane, Action: fmt.Sprintf("query.run %d", n+1)})
		}
		num := uv.Style{Fg: th.Dim, Bg: th.PaneBg}
		if n == cur.Line {
			num.Fg = th.Fg
		}
		f.Text(r.Min.X+1, row, ta.Min.X, fmt.Sprintf("%*d", width, n+1), num)
		bg := th.PaneBg
		if n >= from && n <= to {
			bg = th.Row
			f.Fill(uv.Rect(ta.Min.X, row, ta.Dx(), 1), uv.Style{Fg: th.Fg, Bg: bg})
		}
		c.drawLine(f, ta, row, n, colors[starts[n]:], bg)
	}
	if c.Prompt != "" && r.Dy() > 0 {
		return drawCmdline(f, uv.Rect(r.Min.X, r.Max.Y-1, r.Dx(), 1), c.Prompt+c.Text, len(c.Prompt)+c.Pos)
	}
	if c.Stat != "" && r.Dy() > 0 {
		c.drawStat(f, uv.Rect(r.Min.X, r.Max.Y-1, r.Dx(), 1))
	}
	x, y := ta.Min.X+c.CursorCol-c.Left, r.Min.Y+cur.Line-c.Top
	if x < ta.Min.X || x >= ta.Max.X || y >= r.Max.Y {
		return uv.Pos(-1, -1)
	}
	return uv.Pos(x, y)
}

// drawStat draws the line a search left on the last row: the pattern, the
// count at the right end when there is room for both.
func (c Query) drawStat(f *Frame, r uv.Rectangle) {
	st := uv.Style{Fg: f.Theme.Fg, Bg: f.Theme.PaneBg}
	f.Fill(r, st)
	f.Text(r.Min.X, r.Min.Y, r.Max.X, c.Stat, st)
	if x := r.Max.X - Width(c.StatRight) - 1; x > r.Min.X+Width(c.Stat) {
		f.Text(x, r.Min.Y, r.Max.X, c.StatRight, st)
	}
}

// hitAt is the match at byte col of line n, nil for none.
func (c Query) hitAt(n, col int) *Span {
	for i, h := range c.Hits {
		if h.Line == n && col >= h.From && col < h.To {
			return &c.Hits[i]
		}
	}
	return nil
}

// stopAt is whether byte col of line n is in a tabstop to come.
func (c Query) stopAt(n, col int) bool {
	for _, h := range c.Stops {
		if h.Line == n && col >= h.From && col < h.To {
			return true
		}
	}
	return false
}

// drawLine draws line n on row, from display column Left; fg are its
// characters' colors.
func (c Query) drawLine(f *Frame, ta uv.Rectangle, row, n int, fg []color.Color, bg color.Color) {
	th, l := f.Theme, c.Lines[n]
	put := func(v int, s string, st uv.Style) { // a cell at display column v, when on screen
		if x := ta.Min.X + v - c.Left; x >= ta.Min.X {
			f.Text(x, row, ta.Max.X, s, st)
		}
	}
	v := 0
	for i := 0; i < len(l); {
		gr, w := ansi.FirstGraphemeCluster(l[i:], ansi.GraphemeWidth)
		w = max(w, 1)
		if l[i] == '\t' { // the editor's tabstops (§11)
			w = c.TabWidth - v%c.TabWidth
		}
		st := uv.Style{Fg: fg[i], Bg: bg}
		switch {
		case c.Yank.has(n, i, v, w):
			st = uv.Style{Fg: th.Bg, Bg: th.Yank}
		case c.Sel.has(n, i, v, w):
			st.Bg = th.Visual
		default:
			if h := c.hitAt(n, i); h != nil {
				st = th.hit(h.Current, st)
			} else if c.stopAt(n, i) {
				st.Bg = th.Visual
			}
		}
		switch {
		case l[i] == '\t' || v < c.Left && v+w > c.Left: // a tab, or a character cut by the left edge: blanks
			for k := max(v, c.Left); k < v+w; k++ {
				if c.Sel.Mode == SelBlock { // a block colors only the columns it covers
					st.Bg = bg
					if c.Sel.has(n, i, k, 1) {
						st.Bg = th.Visual
					}
				}
				put(k, " ", st)
			}
		case v >= c.Left:
			put(v, gr, st)
		}
		v, i = v+w, i+len(gr)
	}
	if c.Sel.Mode != SelBlock && c.Sel.has(n, len(l), v, 1) { // the line break is in the selection
		put(v, " ", uv.Style{Fg: th.Fg, Bg: th.Visual})
	}
}

// has reports whether the character at col of line n, drawn at display
// columns [v, v+w), is in s.
func (s Sel) has(n, col, v, w int) bool {
	switch s.Mode {
	case SelNone:
		return false
	case SelLines:
		return n >= s.From.Line && n <= s.To.Line
	case SelBlock:
		return n >= s.From.Line && n <= s.To.Line && v <= s.Right && v+w-1 >= s.Left
	case SelChars: // from From to To, below
	}
	return (n > s.From.Line || n == s.From.Line && col >= s.From.Col) && (n < s.To.Line || n == s.To.Line && col <= s.To.Col)
}

// SQLName is what a name in SQL is to SQLColors.
type SQLName uint8

const (
	OtherName SQLName = iota
	TableName
	ColumnName
)

// SQLNames tells what the name at byte at of some SQL is: word is it as
// written, "quoted" or not.
type SQLNames func(at int, word string) SQLName

// SQLColors are SQL text's colors by byte (§7.3), a query's and a WHERE
// input's, read as d: keywords, numbers, strings, comments, operators, a
// name right before ( as a function, the tables and columns names says it
// is, the rest fg.
func SQLColors(th *Theme, text string, d sqlkit.Dialect, names SQLNames) []color.Color {
	out := make([]color.Color, len(text))
	ts := sqlkit.Scan(text, d)
	for i, t := range ts {
		col := th.Fg
		switch t.Kind {
		case sqlkit.Keyword:
			col = th.Keyword
		case sqlkit.Number:
			col = th.Number
		case sqlkit.String:
			col = th.SQLString
		case sqlkit.Comment:
			col = th.Comment
		case sqlkit.Op:
			col = th.SQLOperator
		case sqlkit.Punct:
			switch text[t.Start] { // not the . of schema.table, and not [ ]
			case ';', ',', '(', ')':
				col = th.SQLOperator
			}
		case sqlkit.Ident, sqlkit.Quoted:
			n := OtherName
			if names != nil {
				n = names(t.Start, text[t.Start:t.End])
			}
			switch {
			case n == TableName: // a table's name before ( too: references t(id), insert into t(a)
				col = th.SQLTable
			case t.Kind == sqlkit.Ident && i+1 < len(ts) && ts[i+1].Start == t.End && text[t.End] == '(':
				col = th.Func
			case n == ColumnName:
				col = th.SQLColumn
			}
		case sqlkit.Space: // the plain color
		}
		for j := t.Start; j < t.End; j++ {
			out[j] = col
		}
	}
	return out
}

// ValueColors are a value's colors by byte for the value editor (F12.2): JSON's
// when json, keys, strings, numbers, true / false / null and punctuation, else
// all fg.
func ValueColors(th *Theme, text string, json bool) []color.Color {
	out := make([]color.Color, len(text))
	for i := range out {
		out[i] = th.Fg
	}
	if !json {
		return out
	}
	for _, t := range sqlkit.ScanJSON(text) {
		col := th.Fg
		switch t.Kind {
		case sqlkit.JSONKey:
			col = th.SQLColumn
		case sqlkit.JSONString:
			col = th.SQLString
		case sqlkit.JSONNumber:
			col = th.Number
		case sqlkit.JSONKeyword:
			col = th.Keyword
		case sqlkit.JSONPunct:
			col = th.SQLOperator
		case sqlkit.JSONOther: // the plain color
		}
		for j := t.Start; j < t.End; j++ {
			out[j] = col
		}
	}
	return out
}

// drawCmdline draws the / ? : line s on r, the cursor at byte pos, and
// returns where the cursor is: the start goes off the left when the text
// is too long for the row.
func drawCmdline(f *Frame, r uv.Rectangle, s string, pos int) uv.Position {
	f.Fill(r, uv.Style{Fg: f.Theme.Fg, Bg: f.Theme.PaneBg})
	for s != "" && r.Dx() > 0 && Width(s[:pos]) >= r.Dx() {
		gr, _ := ansi.FirstGraphemeCluster(s, ansi.GraphemeWidth)
		s, pos = s[len(gr):], pos-len(gr)
	}
	f.Text(r.Min.X, r.Min.Y, r.Max.X, s, uv.Style{Fg: f.Theme.Fg, Bg: f.Theme.PaneBg})
	return uv.Pos(r.Min.X+Width(s[:pos]), r.Min.Y)
}

// hit is st, a character of a search's match, as it is drawn: the match color
// for its background, bg for its text, which a theme that has no search color
// (its visual in its place) keeps its own (F15.9).
func (th *Theme) hit(current bool, st uv.Style) uv.Style {
	bg := th.Search
	if current {
		bg = th.SearchCurrent
	}
	if bg != th.Visual {
		st.Fg = th.Bg
	}
	st.Bg = bg
	return st
}
