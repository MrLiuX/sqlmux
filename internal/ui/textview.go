package ui

import (
	"image/color"
	"strings"

	uv "github.com/charmbracelet/ultraviolet"
)

// TextView is a text seen through a window of rows (§12「预览」, F9.7): Top
// lines scrolled past, a "…" in place of the window's first row when there
// are lines above it and of its last row when there are more below. Colors
// are the text's, a color to each byte (SQLColors); none is an error, drawn
// in its color.
type TextView struct {
	Text   string
	Colors []color.Color
	Top    int
}

// Lines is how many rows the text takes whole.
func (v TextView) Lines() int { return strings.Count(v.Text, "\n") + 1 }

// Fit is Top held to what there is to scroll in a window of rows.
func (v TextView) Fit(rows int) int { return max(min(v.Top, v.Lines()-rows), 0) }

// Draw paints the window in area, over bg.
func (v TextView) Draw(f *Frame, area uv.Rectangle, bg uv.Style) {
	th := f.Theme
	lines, top := strings.Split(v.Text, "\n"), v.Fit(area.Dy())
	at := 0 // the byte the line starts at
	for _, l := range lines[:top] {
		at += len(l) + 1
	}
	for i := range min(area.Dy(), len(lines)-top) {
		l, r := lines[top+i], uv.Rect(area.Min.X, area.Min.Y+i, area.Dx(), 1)
		switch {
		case i == 0 && top > 0, i == area.Dy()-1 && top+i < len(lines)-1:
			f.Text(r.Min.X, r.Min.Y, r.Max.X, "…", uv.Style{Fg: th.Dim, Bg: bg.Bg})
		case v.Colors == nil:
			f.Text(r.Min.X, r.Min.Y, r.Max.X, l, uv.Style{Fg: th.Error, Bg: bg.Bg})
		default:
			Input{Text: l}.draw(f, r, bg, v.Colors[at:at+len(l)])
		}
		at += len(l) + 1
	}
}
