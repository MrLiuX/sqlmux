package editor

import (
	"slices"
	"testing"
)

// After a line that ends in a ; (trailing blanks aside) a new line starts
// in the first column, by ↵, o or O; O looks at the line above the one it
// opens on. Otherwise the indent is inherited (M8 F8.2).
func TestNoIndentAfterStatement(t *testing.T) {
	for _, c := range []struct {
		name, text string
		cur        Pos
		keys       string
		want       []string
	}{
		{"enter", "  id = 1;", Pos{0, 9}, "A<CR>x", []string{"  id = 1;", "x"}},
		{"enter, blanks after the ;", "  id = 1; \t", Pos{0, 11}, "A<CR>x", []string{"  id = 1; \t", "x"}},
		{"enter, no ;", "  id = 1", Pos{0, 8}, "A<CR>x", []string{"  id = 1", "  x"}},
		{"enter, ; before the cursor, more after", "  a;b", Pos{0, 4}, "i<CR>x", []string{"  a;", "xb"}},
		{"o", "  id = 1;", Pos{0, 0}, "ox", []string{"  id = 1;", "x"}},
		{"o, no ;", "  id = 1", Pos{0, 0}, "ox", []string{"  id = 1", "  x"}},
		{"O under a statement", "a;\n  id = 1", Pos{1, 2}, "Ox", []string{"a;", "x", "  id = 1"}},
		{"O, the line itself ends the statement, the one above does not", "  a\n  id = 1;", Pos{1, 2}, "Ox", []string{"  a", "  x", "  id = 1;"}},
		{"O on the first line", "  id = 1", Pos{0, 2}, "Ox", []string{"  x", "  id = 1"}},
	} {
		e := New(c.text)
		e.Goto(c.cur)
		for _, k := range keys(t, c.keys) {
			e.Feed(k)
		}
		if !slices.Equal(e.Lines(), c.want) {
			t.Errorf("%s: %q, want %q", c.name, e.Lines(), c.want)
		}
	}
}
