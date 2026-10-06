package ui

import (
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
	fm := Form{Title: "t", Save: "s", Cancel: "c", Sel: 0, Fields: []FormField{{Label: "password", Mask: true, Input: in}}}
	draw := func(in Input) (stars int, cursor uv.Position, row string) {
		fm.Fields[0].Input = in
		f := NewFrame(80, 12, TokyonightStorm)
		cursor = fm.Draw(f, f.Bounds())
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
