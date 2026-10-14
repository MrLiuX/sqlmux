package ui

import (
	"cmp"
	"strings"

	uv "github.com/charmbracelet/ultraviolet"
)

// Signature is the fields of what a value typed in a query goes in, one line
// beside the cursor's (§11「签名提示与值补全」, F15.3): an INSERT's columns, an
// UPDATE's or a condition's one, a function's arguments, each as "name type";
// Cur is the one the cursor is at.
type Signature struct {
	Fields []string
	Cur    int
	Sep    string // between fields, ", " when empty: a Redis command's are parted by a blank (§9.8)
}

func (s Signature) sep() string { return cmp.Or(s.Sep, sigSep) }

const (
	sigSep = ", "
	sigDot = "…"
)

// fit is the fields from to to, to not in, that a line of room columns shows: the
// current one, then the ones around it, a side at a time, with … on the side
// of those left out.
func (s Signature) fit(room int) (from, to int) {
	width := func(from, to int) int {
		w := len(s.sep()) * (to - from - 1)
		for _, f := range s.Fields[from:to] {
			w += Width(f)
		}
		if from > 0 {
			w += Width(sigDot + s.sep())
		}
		if to < len(s.Fields) {
			w += Width(s.sep() + sigDot)
		}
		return w
	}
	from, to = s.Cur, s.Cur+1
	for grown := true; grown; {
		grown = false
		if to < len(s.Fields) && width(from, to+1) <= room {
			to, grown = to+1, true
		}
		if from > 0 && width(from-1, to) <= room {
			from, grown = from-1, true
		}
	}
	return from, to
}

// draw draws s over row of the text area ta, from its left edge: the current
// field lit, the rest and the … dim.
func (s Signature) draw(f *Frame, ta uv.Rectangle, row int) {
	th := f.Theme
	if len(s.Fields) == 0 || s.Cur < 0 || s.Cur >= len(s.Fields) || ta.Dx() < 3 {
		return
	}
	from, to := s.fit(ta.Dx() - 2)
	dim, cur := uv.Style{Fg: th.Dim, Bg: th.Bar}, uv.Style{Fg: th.Focus, Bg: th.Bar, Attrs: uv.AttrBold}
	text := strings.Join(s.Fields[from:to], s.sep())
	if from > 0 {
		text = sigDot + s.sep() + text
	}
	if to < len(s.Fields) {
		text += s.sep() + sigDot
	}
	f.Fill(uv.Rect(ta.Min.X, row, min(Width(text)+2, ta.Dx()), 1), dim)
	x := ta.Min.X + 1
	if from > 0 {
		x = f.Text(x, row, ta.Max.X, sigDot+s.sep(), dim)
	}
	for i := from; i < to; i++ {
		if i > from {
			x = f.Text(x, row, ta.Max.X, s.sep(), dim)
		}
		st := dim
		if i == s.Cur {
			st = cur
		}
		x = f.Text(x, row, ta.Max.X, s.Fields[i], st)
	}
	if to < len(s.Fields) {
		f.Text(x, row, ta.Max.X, s.sep()+sigDot, dim)
	}
}
