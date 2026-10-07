package ui

import (
	"fmt"
	"strings"
	"testing"

	uv "github.com/charmbracelet/ultraviolet"
)

// A password is drawn as a * for each character the user sees, a wide one,
// an emoji and a letter with its accent each one, and the cursor goes
// where the text's does (§7.8).
func TestFormMask(t *testing.T) {
	text := "密码😀é" // four graphemes, 13 bytes
	in := Input{Text: text, Pos: len(text)}
	fm := Form{Save: "s", Cancel: "c", Sel: 0, Fields: []FormField{{Label: "password", Mask: true, Input: in}}}
	draw := func(in Input) (stars int, cursor uv.Position, row string) {
		fm.Fields[0].Input = in
		f := NewFrame(80, 12, TokyonightStorm)
		cursor = ConnManager{Title: "t", Rows: []ConnRow{{Name: "a"}}, Form: fm}.Draw(f, f.Bounds())
		for _, l := range strings.Split(f.String(), "\n") {
			if n := strings.Count(l, "*"); n > 0 {
				return n, cursor, l
			}
		}
		return 0, cursor, ""
	}
	n, end, row := draw(in)
	if n != 4 || strings.Contains(row, "码") || strings.Contains(row, "😀") {
		t.Fatalf("%d stars in %q", n, row)
	}
	in.Left()
	in.Left()
	if _, at, _ := draw(in); at.X != end.X-2 || at.Y != end.Y {
		t.Errorf("two ← from the end %v: cursor %v", end, at)
	}
}

// The list takes the form's height: with more connections than rows it shows
// a window of them that has the selection in it, whatever Top says (F8.16).
func TestConnManagerScrolls(t *testing.T) {
	var rows []ConnRow
	for i := range 40 {
		rows = append(rows, ConnRow{Name: fmt.Sprintf("conn_%02d", i)})
	}
	m := ConnManager{Title: "t", Rows: rows, Form: Form{Save: "s", Cancel: "c", Fields: make([]FormField, 3)}}
	for _, c := range []struct {
		sel, top      int
		shows, hidden string
	}{{0, 0, "conn_00", "conn_08"}, {30, 0, "conn_30", "conn_00"}, {5, 30, "conn_05", "conn_30"}, {39, 39, "conn_39", "conn_00"}} {
		m.Sel, m.Top = c.sel, c.top
		f := NewFrame(80, 20, TokyonightStorm)
		m.Draw(f, f.Bounds())
		if out := f.String(); !strings.Contains(out, c.shows) || strings.Contains(out, c.hidden) {
			t.Errorf("sel %d top %d: shows %s %v, hides %s %v", c.sel, c.top, c.shows, strings.Contains(out, c.shows), c.hidden, !strings.Contains(out, c.hidden))
		}
	}
	if m.ListRows(80) != 3+5-2 {
		t.Errorf("rows %d", m.ListRows(80))
	}
}
