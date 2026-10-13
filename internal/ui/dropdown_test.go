package ui

import (
	"fmt"
	"strings"
	"testing"

	uv "github.com/charmbracelet/ultraviolet"
)

// The longest item fills the box's inside, as the box is sized for it: a blank
// is held back only for a note after the name.
func TestDropdownLongestItem(t *testing.T) {
	name := "a_very_long_layout_name"
	f := NewFrame(60, 12, TokyonightStorm)
	Dropdown{Items: []string{name}, Pos: [][]int{nil}}.Draw(f, uv.Rect(2, 1, Width(name)+4, 7), 1)
	if !strings.Contains(f.String(), name) {
		t.Errorf("cut:\n%s", f.String())
	}
}

// A name too long for the list ends in …, in the dropdown and the completion
// list alike; a note is left out unless the name fits whole beside it, and is
// never drawn left of the row (F13.9, §7.8).
func TestListRowCut(t *testing.T) {
	name := "mv_order_by_stat_daily_summary"
	row := func(f *Frame, y int) string { return strings.Split(f.String(), "\n")[y] }
	for what, draw := range map[string]func(f *Frame, name, note string) int{
		"dropdown": func(f *Frame, name, note string) int {
			Dropdown{Items: []string{name}, Pos: [][]int{nil}, Notes: []string{note}, Mark: -1}.Draw(f, uv.Rect(14, 0, 16, 5), 1)
			return 3
		},
		"complete": func(f *Frame, name, note string) int {
			Complete{Items: []CompleteItem{{Text: name, Note: note}}, Sel: -1}.Draw(f, uv.Rect(14, 0, 16, 3), 1)
			return 1
		},
	} {
		f := NewFrame(40, 6, TokyonightStorm)
		y := draw(f, name, "bigint")
		if r := row(f, y); !strings.Contains(r, "…") || strings.Contains(r, "bigint") {
			t.Errorf("%s: a long name, its note left out: %q", what, r)
		}
		f = NewFrame(40, 6, TokyonightStorm)
		draw(f, "id", "bigint")
		if r := row(f, y); !strings.Contains(r, "id") || !strings.Contains(r, "bigint") || strings.Contains(r, "…") {
			t.Errorf("%s: a name and a note that fit: %q", what, r)
		}
		f = NewFrame(40, 6, TokyonightStorm)
		f.Text(0, y, 40, strings.Repeat("#", 40), uv.Style{})
		draw(f, "id", "timestamp with time zone")
		r := []rune(row(f, y))
		if string(r[:14]) != strings.Repeat("#", 14) || !strings.Contains(string(r), "id") || strings.Contains(string(r), "timestamp") {
			t.Errorf("%s: a note wider than the box: the name stays, the note is left out, nothing outside the box: %q", what, string(r))
		}
	}
}

// A name and its note fit exactly when they and one blank take what the row has
// inside: then all three are there, a column fewer and the note is left out
// (F13.9).
func TestListTextEdge(t *testing.T) {
	row := func(f *Frame, y int) string { return strings.Split(f.String(), "\n")[y] }
	for what, draw := range map[string]func(f *Frame, w int) int{
		"dropdown": func(f *Frame, w int) int {
			Dropdown{Items: []string{"id"}, Pos: [][]int{nil}, Notes: []string{"bigint"}, Mark: -1}.Draw(f, uv.Rect(0, 0, w, 5), 1)
			return 3
		},
		"complete": func(f *Frame, w int) int {
			Complete{Items: []CompleteItem{{Text: "id", Note: "bigint"}}, Sel: -1}.Draw(f, uv.Rect(0, 0, w, 3), 1)
			return 1
		},
	} {
		f := NewFrame(30, 6, TokyonightStorm)
		y := draw(f, 13) // id, a blank, bigint: 9 of the 9 the row has
		if r := row(f, y); !strings.Contains(r, "id bigint") {
			t.Errorf("%s: it fits exactly: %q", what, r)
		}
		f = NewFrame(30, 6, TokyonightStorm)
		draw(f, 12)
		if r := row(f, y); !strings.Contains(r, "id") || strings.Contains(r, "bigint") {
			t.Errorf("%s: a column short: %q", what, r)
		}
	}
}

// The hints do not widen the box: they go in two rows when the box is too
// narrow for one, and the last whole ones are left out when two are not
// enough; the box has a row for each (F15.6).
func TestDropdownHintRows(t *testing.T) {
	hs := []Hint{{Key: "j/k", Label: "move"}, {Key: "space", Label: "direction"}, {Key: "a", Label: "out"}, {Key: "A", Label: "clear"}, {Key: "↵", Label: "this one only"}}
	d := Dropdown{Items: []string{"id"}, Pos: [][]int{nil}, Hints: hs, Mark: -1}
	anchor := uv.Rect(2, 0, 10, 1)
	screen := uv.Rect(0, 0, 100, 30)
	for _, c := range []struct {
		w    int
		rows []int // how many hints in each row
	}{
		{100, []int{5}},
		{50, []int{4, 1}}, // inner 46: four take 44
		{34, []int{2, 2}},
		{30, []int{2, 2}}, // inner 26: the two take just that
		{29, []int{1, 2}}, // one column less: the second goes to the next row
		{20, []int{1, 1}}, // two rows at most: the rest is left out
		{8, []int{1, 1}},  // the first is wider than the box: shown all the same, cut by the border
	} {
		var got []int
		for _, r := range d.hintRows(c.w) {
			got = append(got, len(r))
		}
		box, _ := d.Box(screen, anchor, c.w)
		if fmt.Sprint(got) != fmt.Sprint(c.rows) || box.Dx() != c.w || box.Dy() != 4+1+len(got)+1 {
			t.Errorf("w %d: hints in rows %v, want %v; box %v", c.w, got, c.rows, box)
		}
	}
	f := NewFrame(100, 30, TokyonightStorm)
	box, rows := d.Box(screen, anchor, 34)
	d.Draw(f, box, rows)
	out := f.String()
	rows3 := strings.Split(out, "\n")[6:8] // under the rule
	if !strings.Contains(rows3[0], "space direction") || !strings.Contains(rows3[1], "A clear") || strings.Contains(out, "this one") {
		t.Errorf("two rows, the last hint left out:\n%s", out)
	}
}

// The width of a text is the one it is drawn in: a glyph of no width is a
// column, a tab goes to its tabstop, with a tab in the line or not, and the
// gutter is in (F15.5).
func TestQueryWidthIsTheDrawnWidth(t *testing.T) {
	zero := strings.Repeat("a\u200b", 20)
	gutter := Query{Lines: []string{"x"}}.gutter()
	for _, c := range []struct {
		line string
		want int
	}{
		{"abc", 3},
		{zero, 40},
		{"\t" + zero, 42},
		{strings.Repeat("\x01", 20), 20},
		{"\t" + strings.Repeat("\x01", 20), 22},
		{"ab\tc", 5},
		{strings.Repeat("宽", 10), 20},
	} {
		if got := (Query{Lines: []string{c.line}, TabWidth: 2}).Width(); got != gutter+c.want {
			t.Errorf("%q: %d, want %d", c.line, got, gutter+c.want)
		}
	}
}

// A box too narrow for even the first hint shows it, cut by the border, in
// the text box (the value editor's) as in the dropdown (F12.8, F15.6).
func TestTextBoxFirstHintIsShownCut(t *testing.T) {
	f := NewFrame(30, 12, TokyonightStorm)
	tb := TextBox{Title: "t", Hints: []Hint{{Key: "esc", Label: "close it now"}, {Key: "C-s", Label: "write"}}}
	tb.Frame(f, uv.Rect(2, 1, 12, 8))
	if out := f.String(); !strings.Contains(out, "esc clo") || strings.Contains(out, "C-s") {
		t.Errorf("a box narrower than the first hint:\n%s", out)
	}
}
