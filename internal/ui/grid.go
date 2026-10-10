package ui

import (
	"image/color"
	"slices"
	"strconv"
	"strings"
	"unicode"

	uv "github.com/charmbracelet/ultraviolet"

	"sqlmux/internal/db"
)

// GridCol is one column of a Grid.
type GridCol struct {
	Name string
	PK   bool // header gets the key icon
	Type ColType
}

// ColType is the class of a column's values: it picks their color, and
// numbers are right-aligned, except transposed, where a column mixes types
// (§7.6).
type ColType int

const (
	ColOther ColType = iota
	ColNumber
	ColString
	ColTime
	ColBool
	ColJSON
)

// Grid is a data table in the §7.6 style: │ between columns and after the
// row numbers, a ─┼─ rule under the header, zebra rows, and the current row
// and cell highlighted. Positions are the data's, whichever way it is drawn:
// Transpose turns records into columns and fields into rows (G-05). Every
// row has a value for each of Cols.
type Grid struct {
	Cols      []GridCol
	Rows      [][]db.Val
	First     int // the row number before Rows[0]: the page's offset
	Row, Col  int // the current cell; Row -1 for none: a table only to look at, nothing in it to click
	Top, Left int // the first record and field shown
	Transpose bool
	FullWidth int // a column wishing no more than this many columns is never squeezed (§7.6, F8.5)
	Focused   bool
	Key       Icon // for primary key headers
	Pane      int
	Edited    map[[2]int]bool // changed cells not saved, by record and field (§7.6)
	Failed    map[int]bool    // records a save failed on: numbers in error (§10.3)
	Changed   map[int]bool    // records with changes: numbers in warn, Failed first (§10.1)
	Edit      *Input          // the current cell's edit, drawn over it (§10.1)
	EditMenu  bool            // the edit has options: a ▾ at its right shows or hides them (§10.2)
	EditBad   bool            // the edit is no value of its column: an error wavy line under it (§10.7)

	// Rows added and marked for deletion (§10.6): numbered + in warn and
	// − in error; an added one's cells not set dim, a deleted one dim and
	// struck through. Nums numbers the rest when added ones sit among them.
	Added, Deleted map[int]bool
	Nums           []int

	Yank *[2]int // the record and field a yank flashes, field -1 for all of it (F3.32)
}

// maxColWidth caps a column's wish (§7.6).
const maxColWidth = 40

// Cell is how a value reads in a grid (§7.6): NULL as <null>; a newline as
// ↵, a tab as a space and any other control character dropped, so nothing
// the value holds can drive the terminal.
func Cell(v db.Val) string {
	if v.Null {
		return "<null>"
	}
	return Printable(v.S)
}

// Printable is s as a cell or an input shows it (§7.6): a newline as ↵, a
// tab as a space, any other control character dropped.
func Printable(s string) string {
	if printableASCII(s) {
		return s
	}
	return strings.Map(func(r rune) rune {
		switch {
		case r == '\n':
			return '↵'
		case r == '\t':
			return ' '
		case unicode.IsControl(r):
			return -1
		}
		return r
	}, s)
}

// view is the grid as drawn: screen rows and columns, whichever way the
// data is turned.
type view struct {
	g          Grid
	rows, cols int
}

func (g Grid) view() view {
	if g.Transpose {
		return view{g, len(g.Cols), len(g.Rows)}
	}
	return view{g, len(g.Rows), len(g.Cols)}
}

// data is the record and field at screen row r, column c.
func (v view) data(r, c int) (rec, field int) {
	if v.g.Transpose {
		return c, r
	}
	return r, c
}

// screen is where record rec, field field is drawn.
func (v view) screen(rec, field int) (r, c int) {
	if v.g.Transpose {
		return field, rec
	}
	return rec, field
}

// val is the value at screen row r, column c, and its field's type.
func (v view) val(r, c int) (db.Val, ColType) {
	rec, field := v.data(r, c)
	return v.g.Rows[rec][field], v.g.Cols[field].Type
}

// head is the header of screen column c: a field's name, or a record's
// number when turned.
func (v view) head(c int) string {
	if v.g.Transpose {
		return v.g.number(c)
	}
	return v.g.header(v.g.Cols[c])
}

// label is what leads screen row r: a record's number, or a field's name.
func (v view) label(r int) string {
	if v.g.Transpose {
		return v.g.header(v.g.Cols[r])
	}
	return v.g.number(r)
}

// Number is record rec's number as the status bar shows it: + for one
// added, as its row's number column (§10.6).
func (g Grid) Number(rec int) string {
	switch {
	case g.Added[rec]:
		return "+"
	case g.Nums != nil:
		return strconv.Itoa(g.Nums[rec])
	}
	return strconv.Itoa(g.First + rec + 1)
}

// number is what leads record rec's row: its Number, − for one marked
// for deletion.
func (g Grid) number(rec int) string {
	if g.Deleted[rec] {
		return "−"
	}
	return g.Number(rec)
}

func (g Grid) header(col GridCol) string {
	if col.PK && g.Key.Text != "" {
		return g.Key.Text + " " + col.Name
	}
	return col.Name
}

// labelWidth is the width of the column that leads each row.
func (v view) labelWidth() int {
	w := 0
	for r := range v.rows {
		w = max(w, Width(v.label(r)))
	}
	return min(w, maxColWidth)
}

// widths sizes each screen column (§7.6): the larger of its header and its
// widest value on the page, at most maxColWidth; when they don't fit in room
// the ones wishing more than FullWidth are squeezed in proportion, never below
// the header nor FullWidth, and the others stay whole. What still doesn't fit
// scrolls. Turned, records are not squeezed: a page of them never fits, and
// squeezed to their numbers they would show nothing.
func (v view) widths(room int) []int {
	ws := make([]int, v.cols)
	heads := make([]int, v.cols)
	total := 0
	for c := range v.cols {
		heads[c] = Width(v.head(c))
		widest := 0
		for r := range v.rows {
			val, _ := v.val(r, c)
			widest = max(widest, Width(Cell(val)))
		}
		ws[c] = min(max(heads[c], widest), maxColWidth)
		total += ws[c]
	}
	if total > room && !v.g.Transpose {
		var wide, want, floor []int // the columns that give way, and what they wish and keep at least
		for c, w := range ws {
			if w > v.g.FullWidth {
				wide, want, floor = append(wide, c), append(want, w), append(floor, max(heads[c], v.g.FullWidth))
				total -= w
			}
		}
		for i, w := range squeeze(want, floor, room-total) {
			ws[wide[i]] = w
		}
	}
	return ws
}

// squeeze shares room among columns that wish want, in proportion, none
// below its floor: one the proportion puts below it is held there and the
// rest share what is left, again; what the division leaves over goes to the
// first of them, none past its wish. Short of the floors the columns are at
// them, and the rest scrolls (F8.5).
func squeeze(want, floor []int, room int) []int {
	held := make([]bool, len(want))
	for {
		left, share, again := room, 0, false
		for i := range want {
			if held[i] {
				left -= floor[i]
			} else {
				share += want[i]
			}
		}
		for i := range want {
			if !held[i] && want[i]*left < floor[i]*share {
				held[i], again = true, true
			}
		}
		if again {
			continue
		}
		out := slices.Clone(floor)
		given := 0
		for i := range want {
			if !held[i] {
				out[i] = min(want[i]*left/share, want[i])
				given += out[i]
			}
		}
		for i := range want {
			if left > given && !held[i] && out[i] < want[i] {
				out[i]++
				given++
			}
		}
		return out
	}
}

// layout is where the grid's columns go in area: the label column's width,
// each screen column's width, and how many rows of data show.
func (v view) layout(area uv.Rectangle) (labelW int, ws []int, rows int) {
	labelW = v.labelWidth()
	// each cell: 1 space, content, 1 space; │ after the labels and between columns
	room := area.Dx() - (labelW + 2 + 1) - 3*v.cols + 1
	return labelW, v.widths(room), max(area.Dy()-2, 0) // the header and its rule
}

// View is Top and Left moved just enough for the current cell to show
// whole in area, as vim scrolls: callers store it after the cursor moves.
// With no current cell they are only kept in range.
func (g Grid) View(area uv.Rectangle) (top, left int) {
	v := g.view()
	labelW, ws, rows := v.layout(area)
	return g.viewWith(v, area, labelW, ws, rows)
}

// viewWith is View with the layout already worked out: Draw has it, and the
// widths of every column of the page are a pass over every value (F8.20).
func (g Grid) viewWith(v view, area uv.Rectangle, labelW int, ws []int, rows int) (top, left int) {
	voff, hoff := v.screen(g.Top, g.Left)
	cr, cc := voff, hoff
	if g.Row >= 0 {
		cr, cc = v.screen(g.Row, max(g.Col, 0))
	}
	voff = max(min(voff, cr, v.rows-rows), cr-rows+1, 0)
	hoff = max(min(hoff, cc, v.cols-1), 0)
	x0 := area.Min.X + labelW + 3 // the first column's cell
	for hoff < cc && x0+colSpan(ws[hoff:cc+1]) > area.Max.X {
		hoff++
	}
	return v.data(voff, hoff)
}

// Scroll is the wheel (§7.6): the view moved dr rows and dc columns on
// screen, at most until the last row sits at the bottom, and the cursor
// pulled back into it. It returns Top, Left, Row and Col.
func (g Grid) Scroll(area uv.Rectangle, dr, dc int) (top, left, row, col int) {
	v := g.view()
	labelW, ws, rows := v.layout(area)
	voff, hoff := v.screen(g.Top, g.Left)
	cr, cc := v.screen(g.Row, g.Col)
	voff = max(min(voff+dr, v.rows-rows), 0)
	hoff = max(min(hoff+dc, v.cols-1), 0)
	last := hoff // the last column that shows whole, at least the first
	for c := hoff + 1; c < v.cols && area.Min.X+labelW+3+colSpan(ws[hoff:c+1]) <= area.Max.X; c++ {
		last = c
	}
	cr = max(min(cr, voff+rows-1), voff)
	cc = max(min(cc, last), hoff)
	top, left = v.data(voff, hoff)
	row, col = v.data(cr, cc)
	return top, left, row, col
}

// colSpan is how far columns of widths ws reach from the first one's cell.
func colSpan(ws []int) int {
	x := 0
	for _, w := range ws {
		x += w + 3
	}
	return x - 2
}

// Draw paints the grid in area and returns where the terminal cursor goes:
// in the cell being edited, else X -1.
func (g Grid) Draw(f *Frame, area uv.Rectangle) uv.Position {
	th := f.Theme
	v := g.view()
	cursor := uv.Pos(-1, -1)
	if area.Dx() <= 0 || area.Dy() <= 0 || len(g.Cols) == 0 {
		return cursor
	}
	labelW, ws, rows := v.layout(area)
	g.Top, g.Left = g.viewWith(v, area, labelW, ws, rows)
	voff, hoff := v.screen(g.Top, g.Left)
	cr, cc := v.screen(g.Row, g.Col)
	line := uv.Style{Fg: th.Sep, Bg: th.PaneBg}
	fg := [...]color.Color{ColOther: th.Fg, ColNumber: th.Number, ColString: th.String, ColTime: th.Time, ColBool: th.Bool, ColJSON: th.JSON}
	name := uv.Style{Fg: th.Func, Bg: th.PaneBg, Attrs: uv.AttrBold}
	num := func(current bool, bg color.Color) uv.Style {
		if current {
			return uv.Style{Fg: th.Focus, Bg: bg}
		}
		return uv.Style{Fg: th.Dim, Bg: bg}
	}

	// seps are the x of each │: after the labels, then between the columns
	// shown; none when not even the labels fit.
	labelEnd := min(area.Min.X+labelW+2, area.Max.X)
	seps := []int{area.Min.X + labelW + 2}
	cols := []int{}
	for c := hoff; c < v.cols && seps[len(seps)-1] < area.Max.X; c++ {
		cols = append(cols, c)
		seps = append(seps, seps[len(seps)-1]+ws[c]+3)
	}
	seps = seps[:len(cols)]
	cellX := func(i int) int { return seps[i] + 2 }

	// a row number is a button to its row, the column kept (G-04)
	rowNo := func(r uv.Rectangle, rec int) {
		if g.Row >= 0 {
			f.Region(r.Intersect(area), Target{Kind: KindRowNo, Pane: g.Pane, Action: "grid.goto " + strconv.Itoa(rec) + " " + strconv.Itoa(max(g.Col, 0))})
		}
	}
	y := area.Min.Y
	for i, c := range cols {
		right := min(cellX(i)+ws[c], area.Max.X)
		h := Truncate(v.head(c), ws[c])
		switch {
		case g.Transpose:
			rowNo(uv.Rect(cellX(i)-1, y, ws[c]+2, 1), c)
			st := num(c == cc, th.PaneBg)
			st.Fg = g.recColor(th, c, st.Fg)
			f.Text(cellX(i), y, right, h, st)
		default:
			f.Text(cellX(i), y, right, h, name)
			if g.Cols[c].PK && g.Key.Fg != nil { // the key icon keeps its own color (§7.7)
				f.Text(cellX(i), y, right, g.Key.Text, g.Key.On(name))
			}
		}
	}
	for _, x := range seps {
		f.Text(x, y, area.Max.X, "│", line)
	}
	if y++; y >= area.Max.Y {
		return cursor
	}
	f.Text(area.Min.X, y, area.Max.X, strings.Repeat("─", area.Dx()), line)
	for _, x := range seps {
		f.Text(x, y, area.Max.X, "┼", line)
	}
	y++

	for r := voff; r < v.rows && y < area.Max.Y; r, y = r+1, y+1 {
		bg := th.PaneBg
		switch {
		case r == cr, f.Over(uv.Rect(area.Min.X, y, area.Dx(), 1)): // the current row, and the one under the pointer (G-06)
			bg = th.Row
		case (r+1)%2 == 0: // even row numbers: zebra
			bg = th.RowAlt
		}
		f.Fill(uv.Rect(area.Min.X, y, area.Dx(), 1), uv.Style{Bg: bg})
		label := v.label(r)
		if g.Transpose {
			st := name
			st.Bg = bg
			f.Text(area.Min.X+1, y, labelEnd, Truncate(label, labelW), st)
			if field := g.Cols[r]; field.PK && g.Key.Fg != nil {
				f.Text(area.Min.X+1, y, labelEnd, g.Key.Text, g.Key.On(st))
			}
		} else {
			rowNo(uv.Rect(area.Min.X, y, labelW+2, 1), r)
			st := num(r == cr, bg)
			rec, _ := v.data(r, 0)
			st.Fg = g.recColor(th, rec, st.Fg)
			f.Text(area.Min.X+1+labelW-Width(label), y, labelEnd, label, st)
		}
		for i, c := range cols {
			val, typ := v.val(r, c)
			s := Truncate(Cell(val), ws[c])
			rec, field := v.data(r, c)
			st := uv.Style{Fg: fg[typ], Bg: bg}
			if val.Null {
				st.Fg = th.Dim
			}
			x := cellX(i)
			if typ == ColNumber && !g.Transpose {
				x += ws[c] - Width(s)
			}
			cell := uv.Rect(cellX(i)-1, y, ws[c]+2, 1).Intersect(area)
			switch edited := g.Edited[[2]int{rec, field}]; {
			case g.Deleted[rec]:
				st.Fg, st.Attrs = th.Dim, uv.AttrStrikethrough
			case edited:
				st = uv.Style{Fg: th.Warn, Bg: th.EditedBg, Underline: uv.UnderlineDotted}
				f.Fill(cell, uv.Style{Bg: st.Bg})
			case g.Added[rec]: // <default>, not set yet
				st.Fg = th.Dim
			}
			if r == cr && c == cc {
				st.Bg = th.CursorBlur
				if g.Focused {
					st.Bg = th.Cursor
				}
				f.Fill(cell, uv.Style{Bg: st.Bg})
			}
			if yk := g.Yank; yk != nil && rec == yk[0] && (yk[1] < 0 || field == yk[1]) {
				st.Fg, st.Bg = th.Bg, th.Yank
				f.Fill(cell, uv.Style{Bg: st.Bg})
			}
			if g.Row >= 0 {
				f.Region(cell, Target{Kind: KindCell, Pane: g.Pane, Action: "grid.goto " + strconv.Itoa(rec) + " " + strconv.Itoa(field)})
			}
			drawCell(f, x, y, min(cellX(i)+ws[c], area.Max.X), s, st, uv.Style{Fg: th.Dim, Bg: st.Bg})
		}
		for _, x := range seps {
			f.Text(x, y, area.Max.X, "│", uv.Style{Fg: th.Sep, Bg: bg})
		}
	}
	if edit := g.EditRect(area); !edit.Empty() {
		f.Region(edit, Target{}) // a click in it is not elsewhere
		f.Fill(edit, uv.Style{Bg: th.Cursor})
		text := uv.Rect(edit.Min.X+1, edit.Min.Y, max(edit.Dx()-2, 1), 1)
		if g.EditMenu {
			text.Max.X = max(text.Max.X-2, text.Min.X+1)
			arrow := uv.Rect(edit.Max.X-2, edit.Min.Y, 2, 1)
			st := uv.Style{Fg: th.Dim, Bg: th.Cursor}
			if f.Region(arrow, Target{Kind: KindButton, Action: "cell.options"}) {
				st.Bg = th.Select
			}
			f.Text(arrow.Min.X, arrow.Min.Y, arrow.Max.X, "▾ ", st)
		}
		st := uv.Style{Fg: th.Fg, Bg: th.Cursor}
		if g.EditBad { // a plain line where a terminal has no curly one
			st.Underline, st.UnderlineColor = uv.UnderlineCurly, th.Error
		}
		cursor = g.Edit.Draw(f, text, st)
	}
	return cursor
}

// EditRect is where the current cell's edit goes in area (§10.1): over the
// cell and on to the right as its text needs, up to the edge; empty with
// no edit, or when the cell doesn't show.
func (g Grid) EditRect(area uv.Rectangle) uv.Rectangle {
	if g.Edit == nil {
		return uv.Rectangle{}
	}
	need := Width(Printable(g.Edit.Text)) + 3 // a space each side, and the cursor's cell
	if g.EditMenu {
		need += 2
	}
	return g.cellRect(area, need)
}

// CellRect is where the current cell is in area, edited or not; empty when it
// doesn't show (F9.8).
func (g Grid) CellRect(area uv.Rectangle) uv.Rectangle { return g.cellRect(area, 0) }

// cellRect is the current cell's, at least need wide, on to the right as far
// as the edge.
func (g Grid) cellRect(area uv.Rectangle, need int) uv.Rectangle {
	if len(g.Cols) == 0 {
		return uv.Rectangle{}
	}
	v := g.view()
	labelW, ws, rows := v.layout(area)
	g.Top, g.Left = g.View(area)
	voff, hoff := v.screen(g.Top, g.Left)
	cr, cc := v.screen(g.Row, g.Col)
	if cr < voff || cr >= voff+rows || cc < hoff {
		return uv.Rectangle{}
	}
	x := area.Min.X + labelW + 3 // the first shown column's cell, its padding in
	for c := hoff; c < cc; c++ {
		x += ws[c] + 3
	}
	r := uv.Rect(x, area.Min.Y+2+cr-voff, max(ws[cc]+2, need), 1)
	r.Max.X = min(r.Max.X, area.Max.X)
	return r.Intersect(area)
}

// drawCell draws a Cell's text with its ↵ marks in mark (§7.6).
// ponytail: a ↵ that was in the value itself is dimmed too.
func drawCell(f *Frame, x, y, right int, s string, st, mark uv.Style) {
	for i, part := range strings.Split(s, "↵") {
		if i > 0 {
			x = f.Text(x, y, right, "↵", mark)
		}
		x = f.Text(x, y, right, part, st)
	}
}

// recColor is record rec's number's color: error where a save failed on
// it or it is marked for deletion, warn where it has changes or is added,
// else fg (§10.1, §10.3, §10.6).
func (g Grid) recColor(th *Theme, rec int, fg color.Color) color.Color {
	switch {
	case g.Failed[rec], g.Deleted[rec]:
		return th.Error
	case g.Changed[rec], g.Added[rec]:
		return th.Warn
	}
	return fg
}
