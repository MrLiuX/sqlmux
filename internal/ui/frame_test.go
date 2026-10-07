package ui

import (
	"strings"
	"testing"

	uv "github.com/charmbracelet/ultraviolet"
)

func TestText(t *testing.T) {
	for _, c := range []struct {
		name         string
		x, right     int
		s            string
		wantRow      string
		wantReturned int
	}{
		{"fits", 1, 10, "ab", " ab", 3},
		{"clipped at right", 0, 3, "abcdef", "abc", 3},
		{"wide char straddling right is dropped", 0, 3, "a中文", "a中", 3},
		{"wide char exactly fits", 0, 5, "a中文", "a中文", 5},
		{"negative x clips the left", -2, 10, "abcd", "cd", 2},
		{"right beyond the frame", 6, 99, "abcdef", "      ab", 8},
		{"a combining mark stays in its cluster", 0, 10, "e\u0301x", "e\u0301x", 2},
	} {
		f := NewFrame(8, 1, TokyonightStorm)
		got := f.Text(c.x, 0, c.right, c.s, uv.Style{})
		row := strings.TrimRight(f.String(), " ")
		if row != c.wantRow || got != c.wantReturned {
			t.Errorf("%s: row %q returned %d; want %q %d", c.name, row, got, c.wantRow, c.wantReturned)
		}
	}
}

// Every box draws with the frame's border: the sidebar and panes' Block, and
// every overlay, none keeping the single line's corners (§7.1, F8.10).
func TestEveryBoxUsesFrameBorder(t *testing.T) {
	screen := uv.Rect(0, 0, 100, 30)
	box := uv.Rect(10, 5, 60, 14)
	for name, draw := range map[string]func(f *Frame){
		"block":    func(f *Frame) { Block{Num: "①", Focused: true}.Draw(f, box) },
		"palette":  func(f *Frame) { Palette{Scopes: []string{"all"}, Rows: []PaletteRow{{Name: "x"}}}.Draw(f, screen) },
		"timepick": func(f *Frame) { TimePick{Kind: Timestamp, Text: "2026-09-01 00:01:00", Sel: -1}.Draw(f, box) },
		"confirm":  func(f *Frame) { Confirm{Text: "q", YesKey: "y", Yes: "Y", NoKey: "n", No: "N"}.Draw(f, screen) },
		"dropdown": func(f *Frame) { Dropdown{Items: []string{"a"}, Pos: [][]int{nil}}.Draw(f, box, 1) },
		"sessions": func(f *Frame) { SessionList{Rows: []SessionRow{{Name: "s"}}}.Draw(f, screen) },
		"whichkey": func(f *Frame) {
			WhichKey{Prefix: "g", Items: []WhichKeyItem{{Key: "t", Title: "tab", Group: "normal"}}}.Draw(f, screen)
		},
		"complete": func(f *Frame) { Complete{Items: []CompleteItem{{Text: "a"}}}.Draw(f, box, 1) },
		"hint":     func(f *Frame) { CellHint{Text: "bad"}.Draw(f, uv.Rect(10, 5, 20, CellHintRows)) },
		"form":     func(f *Frame) { ConnManager{Title: "t", Form: Form{Save: "s", Cancel: "c"}}.Draw(f, screen) },
	} {
		f := NewFrame(100, 30, TokyonightStorm)
		f.Border = uv.RoundedBorder()
		draw(f)
		got := f.String()
		if strings.ContainsAny(got, "┌┐└┘") || !strings.Contains(got, "╭") {
			t.Errorf("%s: not drawn with the frame's border:\n%s", name, got)
		}
	}
}
