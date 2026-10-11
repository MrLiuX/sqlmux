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
