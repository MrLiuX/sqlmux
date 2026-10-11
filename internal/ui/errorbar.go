package ui

import (
	"fmt"
	"strings"

	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
)

// ErrorBar is a database's error at the bottom of a pane's content, over
// its tab bar (§7.8「错误栏」): First on the first row in the error color,
// its middle cut to fit (§10.3), then More a row each, cut at the right,
// all on error_bg; the × at the first row's end closes it.
type ErrorBar struct {
	First Note
	More  []string
	Pane  int
}

// errorBarRows is the most rows a bar takes; the last reads … when More
// has more.
const errorBarRows = 6

// Rows is how tall b draws.
func (b ErrorBar) Rows() int { return min(1+len(b.More), errorBarRows) }

// Draw paints b over the last Rows rows of r.
func (b ErrorBar) Draw(f *Frame, r uv.Rectangle) {
	th := f.Theme
	n := min(b.Rows(), r.Dy())
	if n <= 0 || r.Dx() < 4 {
		return
	}
	r.Min.Y = r.Max.Y - n
	st := uv.Style{Fg: th.Fg, Bg: th.ErrorBg}
	f.Fill(r, st)
	x := r.Max.X - 3 // " × "
	shut := uv.Style{Fg: th.Dim, Bg: th.ErrorBg}
	if f.Region(uv.Rect(x, r.Min.Y, 3, 1), Target{Kind: KindButton, Action: fmt.Sprintf("pane.error.close %d", b.Pane)}) {
		shut.Bg = th.Select
	}
	f.Text(x, r.Min.Y, r.Max.X, " × ", shut)
	f.Text(r.Min.X+1, r.Min.Y, x, b.First.Fit(x-r.Min.X-2), uv.Style{Fg: th.Error, Bg: th.ErrorBg})
	for i, l := range b.More[:n-1] {
		if i == errorBarRows-2 && len(b.More) > n-1 {
			l = "…"
		}
		f.Text(r.Min.X+1, r.Min.Y+1+i, r.Max.X-1, Truncate(l, r.Dx()-2), st)
	}
}

// ErrorPage is what a tab shows in the middle of its content when what it
// shows is not to be had (F12.5): Title on the first row, Text, a
// driver's error as it came, wrapped to the width below it; all in the error
// color, each row centered.
type ErrorPage struct{ Title, Text string }

func (e ErrorPage) Draw(f *Frame, r uv.Rectangle) {
	if r.Dx() < 4 {
		return
	}
	lines := []string{e.Title}
	for _, l := range strings.Split(e.Text, "\n") {
		lines = append(lines, strings.Split(ansi.Wrap(l, r.Dx()-2, ""), "\n")...)
	}
	lines = lines[:min(len(lines), r.Dy())]
	st := uv.Style{Fg: f.Theme.Error, Bg: f.Theme.PaneBg}
	for i, l := range lines {
		l = Truncate(l, r.Dx())
		f.Text(r.Min.X+(r.Dx()-Width(l))/2, r.Min.Y+(r.Dy()-len(lines))/2+i, r.Max.X, l, st)
	}
}
