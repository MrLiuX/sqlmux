package app

import (
	"testing"

	"github.com/kackerx/sqlmux/internal/keymap"
	"github.com/kackerx/sqlmux/internal/ui"
)

// C-h deletes back in every single-line input, as <BS> does: the shared edit,
// and the inputs that use it (F13.11).
func TestEditInputBackspaceKeys(t *testing.T) {
	for _, k := range []keymap.Key{"<BS>", "<C-h>"} {
		in := ui.Input{Text: "abc def", Pos: 5}
		if !editInput(&in, k) || in.Text != "abc ef" || in.Pos != 4 {
			t.Errorf("%s: %q at %d", k, in.Text, in.Pos)
		}
	}
	a := sized(160, 45, "nerd")
	feed(t, a, "<C-p>abc<C-h>")
	if a.palette.input.Text != "ab" {
		t.Errorf("the palette's input: %q", a.palette.input.Text)
	}
	b, _, _ := withRecorder(t)
	feed(t, b, "gc/ab<C-h>")
	if b.cols == nil || b.cols.filter.Text != "a" {
		t.Errorf("the COLS filter: %+v", b.cols)
	}
	// the quick SQL's pairs: an empty pair goes whole, as with <BS>
	for _, k := range []string{"<BS>", "<C-h>"} {
		q := sized(160, 45, "nerd")
		feed(t, q, "<C-p>;select ("+k)
		if got := q.palette.input.Text; got != ";select " {
			t.Errorf("%s in an empty pair of the quick SQL: %q", k, got)
		}
	}
	// the jump's pattern
	j := wide(160, 45)
	loadOrders(t, j, 5)
	feed(t, j, "s")
	j.jump.pattern = "ab"
	feed(t, j, "<C-h>")
	if j.jump == nil || j.jump.pattern != "a" {
		t.Errorf("C-h in the jump's pattern: %+v", j.jump)
	}
}
