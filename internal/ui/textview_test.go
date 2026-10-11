package ui

import (
	"strings"
	"testing"

	uv "github.com/charmbracelet/ultraviolet"
)

// The hints of a text box have a row of their own only where a row of text is
// left: a box of three rows shows its one row of text, none of them (F12.8).
func TestTextBoxHintsNeedARowOfText(t *testing.T) {
	b := TextBox{Hints: []Hint{{Key: "esc", Label: "close"}}}
	for _, c := range []struct {
		rows, text int
		hints      bool
	}{{3, 1, false}, {4, 1, true}, {5, 2, true}} {
		box := uv.Rect(0, 0, 30, c.rows)
		f := NewFrame(30, 6, TokyonightStorm)
		b.Frame(f, box)
		if got := b.Inner(box).Dy(); got != c.text || strings.Contains(f.String(), "esc close") != c.hints {
			t.Errorf("%d rows: %d for the text, hints shown %v:\n%s", c.rows, got, strings.Contains(f.String(), "esc close"), f.String())
		}
	}
}
