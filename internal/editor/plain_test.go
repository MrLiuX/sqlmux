package editor

import (
	"strings"
	"testing"
)

func plain(text string) *Editor {
	e := New(text)
	e.SetPlain(true)
	e.Goto(Pos{Line: len(e.Lines()) - 1, Col: len(e.Lines()[len(e.Lines())-1])})
	return e
}

func feed(e *Editor, keys ...string) {
	for _, k := range keys {
		e.Feed(k)
	}
}

func text(e *Editor) string { return strings.Join(e.Lines(), "\n") }

// The modeless editor types, moves, deletes and takes <Esc> for nothing; it
// is in INSERT, whatever is pressed (§6.9, F6.6a).
func TestPlainTyping(t *testing.T) {
	e := plain("")
	feed(e, "a", "b", "c", "<Left>", "x")
	if text(e) != "abxc" || e.Mode() != Insert {
		t.Fatalf("typing at the cursor: %q in %v", text(e), e.Mode())
	}
	feed(e, "<Esc>", "<Esc>", "y", "<BS>", "<BS>", "<Del>")
	if text(e) != "ab" || e.Mode() != Insert || e.Cursor() != (Pos{0, 2}) {
		t.Fatalf("esc is nothing, BS and Delete: %q at %v in %v", text(e), e.Cursor(), e.Mode())
	}
	e = plain("one")
	e.TabWidth = 4
	feed(e, "<CR>", "<Tab>", "x")
	if got := text(e); got != "one\n    x" {
		t.Errorf("↵ and Tab: %q", got)
	}
}

// A selection grows with Shift and an arrow, goes with a key that types or
// deletes, and ends at the end the arrow points to (§6.9).
func TestPlainSelection(t *testing.T) {
	e := plain("hello world")
	feed(e, "<C-a>")
	for range 5 {
		feed(e, "<S-Right>")
	}
	from, to, ok := e.Selection()
	if !ok || from != (Pos{0, 0}) || to != (Pos{0, 4}) || e.SelectedText() != "hello" || e.Mode() != Insert {
		t.Fatalf("selection: %v %v %v %q", from, to, ok, e.SelectedText())
	}
	feed(e, "X")
	if text(e) != "X world" || e.HasSelection() {
		t.Fatalf("typed over it: %q", text(e))
	}
	feed(e, "<C-e>", "<S-Left>", "<S-Left>", "<S-Left>")
	if e.SelectedText() != "rld" {
		t.Fatalf("backwards: %q", e.SelectedText())
	}
	feed(e, "<BS>")
	if text(e) != "X wo" || e.Cursor() != (Pos{0, 4}) {
		t.Fatalf("BS takes it: %q", text(e))
	}
	feed(e, "<S-Left>", "<S-Left>", "<Left>")
	if e.HasSelection() || e.Cursor() != (Pos{0, 2}) {
		t.Errorf("← ends it at its start: %v", e.Cursor())
	}
	feed(e, "<S-Right>", "<S-Right>", "<Right>")
	if e.HasSelection() || e.Cursor() != (Pos{0, 4}) {
		t.Errorf("→ ends it at its end: %v", e.Cursor())
	}
	// over lines, and a paste takes its place
	e = plain("ab\ncd\nef")
	e.Goto(Pos{0, 1})
	feed(e, "<S-Down>", "<S-Down>")
	if got := e.SelectedText(); got != "b\ncd\ne" {
		t.Fatalf("lines: %q", got)
	}
	if _, to, _ := e.Selection(); to != (Pos{2, 0}) {
		t.Errorf("the last character in: %v", to)
	}
	e.Paste("ZZ")
	if text(e) != "aZZf" {
		t.Errorf("paste: %q", text(e))
	}
	e = plain("ab\ncd")
	e.Goto(Pos{0, 2})
	feed(e, "<S-Right>") // over the line break
	if _, to, ok := e.Selection(); !ok || to != (Pos{0, 2}) || e.SelectedText() != "\n" {
		t.Errorf("the break: %v %q", to, e.SelectedText())
	}
}

// C-w takes the word back, C-a and C-e go to the ends of the line, C-z and
// C-y undo and redo, a step ending at an arrow key (§6.9).
func TestPlainEditing(t *testing.T) {
	e := plain("foo bar")
	feed(e, "<C-w>")
	if text(e) != "foo " {
		t.Errorf("C-w: %q", text(e))
	}
	feed(e, "<C-a>", "x", "<C-e>", "y")
	if text(e) != "xfoo y" {
		t.Errorf("C-a, C-e: %q", text(e))
	}
	e = plain("")
	feed(e, "a", "b", "<Left>", "c", "<Right>", "d")
	if text(e) != "acbd" {
		t.Fatalf("typed: %q", text(e))
	}
	feed(e, "<C-z>")
	if text(e) != "acb" {
		t.Errorf("C-z: %q", text(e))
	}
	feed(e, "<C-z>")
	if text(e) != "ab" {
		t.Errorf("C-z again: %q", text(e))
	}
	feed(e, "<C-y>", "<C-y>")
	if text(e) != "acbd" || e.Mode() != Insert {
		t.Errorf("C-y: %q in %v", text(e), e.Mode())
	}
	feed(e, "<C-z>", "<C-z>", "<C-z>")
	if text(e) != "" && text(e) != "ab" {
		t.Errorf("back to the start: %q", text(e))
	}
}

// The mouse selects: where the button went down to where it is dragged.
func TestPlainMouse(t *testing.T) {
	e := plain("hello world")
	e.Click(0, 2)
	e.Drag(0, 7)
	if got := e.SelectedText(); got != "llo w" {
		t.Errorf("drag: %q", got)
	}
	e.Click(0, 1)
	if e.HasSelection() {
		t.Error("a click ends it")
	}
}
