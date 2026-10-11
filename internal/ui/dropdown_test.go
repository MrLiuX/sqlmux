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
// list alike, and a note wider than the box is cut where the row starts, not
// drawn over what is left of the box (F13.9).
func TestListRowCut(t *testing.T) {
	name := "mv_order_by_stat_daily_summary"
	row := func(f *Frame, y int) string { return strings.Split(f.String(), "\n")[y] }
	for what, draw := range map[string]func(f *Frame, note string) int{
		"dropdown": func(f *Frame, note string) int {
			Dropdown{Items: []string{name}, Pos: [][]int{nil}, Notes: []string{note}, Mark: -1}.Draw(f, uv.Rect(14, 0, 16, 5), 1)
			return 3
		},
		"complete": func(f *Frame, note string) int {
			Complete{Items: []CompleteItem{{Text: name, Note: note}}, Sel: -1}.Draw(f, uv.Rect(14, 0, 16, 3), 1)
			return 1
		},
	} {
		f := NewFrame(40, 6, TokyonightStorm)
		y := draw(f, "bigint")
		if r := row(f, y); !strings.Contains(r, "…") || !strings.Contains(r, "bigint") {
			t.Errorf("%s: a long name: %q", what, r)
		}
		f = NewFrame(40, 6, TokyonightStorm)
		f.Text(0, y, 40, strings.Repeat("#", 40), uv.Style{})
		y = draw(f, "timestamp with time zone")
		if r := []rune(row(f, y)); string(r[:14]) != strings.Repeat("#", 14) {
			t.Errorf("%s: a note wider than the box drew outside it: %q", what, string(r))
		}
	}
}
