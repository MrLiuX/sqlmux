package ui

import (
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
