package ui

import (
	"strings"

	uv "github.com/charmbracelet/ultraviolet"
)

// FormField is a row of a form (§7.8「新建连接表单」): a line input, or a
// switch among Choices, On the one it is on.
type FormField struct {
	Label   string
	Input   Input
	Choices []string
	On      int
}

// Form is a box of fields over the screen: Sel is the field with the keys,
// Err why it did not save, in error above the hints.
type Form struct {
	Title  string
	Fields []FormField
	Sel    int
	Err    string
	Hints  []Hint
}

// Draw dims what is behind (§7.5), paints the form centered and returns
// where the selected input has its cursor, else (-1, -1).
func (fm Form) Draw(f *Frame, screen uv.Rectangle) uv.Position {
	th := f.Theme
	w, h := min(80, screen.Dx()-4), len(fm.Fields)+5 // border, fields, error, rule, hints, border
	box := uv.Rect(screen.Min.X+(screen.Dx()-w)/2, screen.Min.Y+max(screen.Dy()-h, 0)/2, w, h)
	cursor := uv.Pos(-1, -1)
	f.Dim()
	f.Region(f.Bounds(), Target{}) // no click goes past it
	if box.Dx() < 8 || box.Dy() > screen.Dy() {
		return cursor
	}
	bg, dim := uv.Style{Fg: th.Fg, Bg: th.PaneBg}, uv.Style{Fg: th.Dim, Bg: th.PaneBg}
	f.Fill(box, bg)
	border := uv.NormalBorder().Style(uv.Style{Fg: th.Focus, Bg: th.PaneBg})
	border.Draw(f.Buf, box)
	f.Text(box.Min.X+2, box.Min.Y, box.Max.X-1, " "+fm.Title+" ", uv.Style{Fg: th.Focus, Bg: th.PaneBg, Attrs: uv.AttrBold})
	x0, x1 := box.Min.X+2, box.Max.X-2
	labelW := 0
	for _, fd := range fm.Fields {
		labelW = max(labelW, Width(fd.Label))
	}
	for i, fd := range fm.Fields {
		y := box.Min.Y + 1 + i
		f.Region(uv.Rect(box.Min.X+1, y, box.Dx()-2, 1), Target{Kind: KindRow, I: i})
		f.Text(x0, y, x1, fd.Label, dim)
		r := uv.Rect(x0+labelW+2, y, max(x1-x0-labelW-2, 0), 1)
		switch {
		case fd.Choices == nil && i == fm.Sel:
			cursor = fd.Input.Draw(f, r, bg)
		case fd.Choices == nil:
			fd.Input.Draw(f, r, bg)
		default: // a switch: its value, lit while it has the keys
			st := bg
			if i == fm.Sel {
				st.Bg = th.Select
			}
			f.Text(r.Min.X, y, r.Max.X, fd.Choices[fd.On], st)
		}
	}
	y := box.Max.Y - 4 // the error, the rule, the hints, the border
	f.Text(x0, y, x1, fm.Err, uv.Style{Fg: th.Error, Bg: th.PaneBg})
	f.Text(box.Min.X+1, y+1, box.Max.X-1, strings.Repeat("─", box.Dx()-2), uv.Style{Fg: th.Sep, Bg: th.PaneBg})
	hintRow(f, x0, y+2, x1, fm.Hints, dim, Target{Kind: KindButton})
	return cursor
}
