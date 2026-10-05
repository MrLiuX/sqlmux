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

// selected is the text the modeless selection holds, "" with none.
func selected(e *Editor) string {
	from, to, ok := e.SelectionSpan()
	if !ok {
		return ""
	}
	ls := e.Lines()
	if from.Line == to.Line {
		return ls[from.Line][from.Col:to.Col]
	}
	return strings.Join(append(append([]string{ls[from.Line][from.Col:]}, ls[from.Line+1:to.Line]...), ls[to.Line][:to.Col]), "\n")
}

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
	if !ok || from != (Pos{0, 0}) || to != (Pos{0, 4}) || selected(e) != "hello" || e.Mode() != Insert {
		t.Fatalf("selection: %v %v %v %q", from, to, ok, selected(e))
	}
	feed(e, "X")
	if text(e) != "X world" || e.HasSelection() {
		t.Fatalf("typed over it: %q", text(e))
	}
	feed(e, "<C-e>", "<S-Left>", "<S-Left>", "<S-Left>")
	if selected(e) != "rld" {
		t.Fatalf("backwards: %q", selected(e))
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
	if got := selected(e); got != "b\ncd\ne" {
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
	if _, to, ok := e.Selection(); !ok || to != (Pos{0, 2}) || selected(e) != "\n" {
		t.Errorf("the break: %v %q", to, selected(e))
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
	if text(e) != "" {
		t.Errorf("back to the start: %q", text(e))
	}
}

// The mouse selects: where the button went down to where it is dragged.
func TestPlainMouse(t *testing.T) {
	e := plain("hello world")
	e.Click(0, 2)
	e.Drag(0, 7)
	if got := selected(e); got != "llo w" {
		t.Errorf("drag: %q", got)
	}
	e.Click(0, 1)
	if e.HasSelection() {
		t.Error("a click ends it")
	}
}

// Reading the text again (the external editor) leaves the modeless editor
// as it was: in INSERT, with no selection, whatever it was selecting, and the
// next key types.
func TestPlainLoad(t *testing.T) {
	e := plain("one\ntwo\nthree")
	e.Goto(Pos{0, 0})
	feed(e, "<S-Down>", "<S-Down>", "<S-Right>")
	e.Load("short")
	if e.Mode() != Insert || e.HasSelection() || text(e) != "short" {
		t.Fatalf("loaded: %q in %v, selecting %v", text(e), e.Mode(), e.HasSelection())
	}
	e.Selection() // what the drawing asks: no index out of range
	feed(e, "x")
	if !strings.Contains(text(e), "x") {
		t.Errorf("the next key: %q", text(e))
	}
	feed(e, "<C-z>", "<C-z>")
	if text(e) != "one\ntwo\nthree" {
		t.Errorf("the load and the typing are steps of their own: %q", text(e))
	}
}

// Redo puts the cursor back where the step began, which the text may no
// longer reach: it is kept on the text.
func TestPlainRedoCursor(t *testing.T) {
	e := plain("abcdef")
	feed(e, "<BS>", "<BS>", "<BS>", "<C-z>", "<C-y>")
	if text(e) != "abc" || e.Cursor().Col > 3 {
		t.Fatalf("redo: %q at %v", text(e), e.Cursor())
	}
	feed(e, "x")
	if text(e) != "abcx" {
		t.Errorf("typing after it: %q", text(e))
	}
}

// What is typed is one undo step, which an arrow key, ↵, a paste, a
// formatting and a drag end.
func TestPlainUndoSteps(t *testing.T) {
	undo := func(e *Editor, n int) string {
		for range n {
			feed(e, "<C-z>")
		}
		return text(e)
	}
	e := plain("")
	feed(e, "a", "b", "<CR>", "c", "d")
	if got := undo(e, 1); got != "ab" {
		t.Errorf("↵ ends a step: %q", got)
	}
	e = plain("")
	feed(e, "a", "b")
	e.Paste("XY")
	feed(e, "c", "d")
	if got := undo(e, 1); got != "abXY" {
		t.Errorf("after a paste: %q", got)
	}
	if got := undo(e, 1); got != "ab" {
		t.Errorf("the paste: %q", got)
	}
	e = plain("")
	feed(e, "s", "e", "l")
	e.Replace(Pos{0, 0}, Pos{0, 3}, "SEL\nECT")
	if text(e) != "SEL\nECT" || e.HasSelection() {
		t.Fatalf("replace: %q", text(e))
	}
	if got := undo(e, 1); got != "sel" {
		t.Errorf("a formatting is a step of its own: %q", got)
	}
	if got := undo(e, 1); got != "" {
		t.Errorf("what was typed: %q", got)
	}
	e = plain("hello world")
	feed(e, "<C-a>", "<S-Right>", "<S-Right>", "X")
	if got := undo(e, 1); got != "hello world" {
		t.Errorf("shift-arrows: %q", got)
	}
	e = plain("hello world")
	e.Click(0, 0)
	e.Drag(0, 5)
	feed(e, "X")
	if got := undo(e, 1); got != "hello world" {
		t.Errorf("a drag: %q", got)
	}
}

// Tab and Shift-Tab over a selection move its lines a tab stop; the lines
// stay selected, and it is a step of its own.
func TestPlainIndentSelection(t *testing.T) {
	e := plain("a\nb\nc")
	e.TabWidth = 4
	e.Goto(Pos{0, 0})
	feed(e, "<S-Down>", "<S-Down>")
	feed(e, "<Tab>")
	if text(e) != "    a\n    b\nc" || !e.HasSelection() {
		t.Fatalf("Tab: %q", text(e))
	}
	feed(e, "<Tab>", "<S-Tab>")
	if text(e) != "    a\n    b\nc" {
		t.Errorf("Tab, S-Tab: %q", text(e))
	}
	feed(e, "<S-Tab>", "<S-Tab>")
	if text(e) != "a\nb\nc" {
		t.Errorf("S-Tab: %q", text(e))
	}
	feed(e, "<C-z>")
	if text(e) != "    a\n    b\nc" {
		t.Errorf("undo is one indent: %q", text(e))
	}
	e.Goto(Pos{2, 0})
	feed(e, "<Tab>")
	if text(e) != "    a\n    b\n    c" || e.HasSelection() {
		t.Errorf("no selection: Tab is a tab stop: %q", text(e))
	}
}

// An opening bracket or quote over a selection puts the pair round it, the
// text staying selected; with autopairs off, or any other key, it replaces.
func TestPlainSurround(t *testing.T) {
	e := plain("a + b")
	e.AutoPairs = true
	feed(e, "<C-a>")
	for range 5 {
		feed(e, "<S-Right>")
	}
	feed(e, "(")
	if text(e) != "(a + b)" || selected(e) != "a + b" {
		t.Fatalf("(: %q, selected %q", text(e), selected(e))
	}
	feed(e, "\"")
	if text(e) != "(\"a + b\")" || selected(e) != "a + b" {
		t.Errorf("a quote: %q", text(e))
	}
	feed(e, "<C-z>")
	if text(e) != "(a + b)" {
		t.Errorf("undo: %q", text(e))
	}
	e = plain("a\nb") // over lines, the selection made backwards
	e.AutoPairs = true
	e.Goto(Pos{1, 1})
	feed(e, "<S-Left>", "<S-Left>", "<S-Left>", "[")
	if text(e) != "[a\nb]" || selected(e) != "a\nb" || e.Cursor() != (Pos{0, 1}) {
		t.Errorf("backwards: %q %q at %v", text(e), selected(e), e.Cursor())
	}
	e = plain("ab")
	e.AutoPairs = true
	feed(e, "<S-Left>", ")")
	if text(e) != "a)" {
		t.Errorf("a closing half replaces: %q", text(e))
	}
	e = plain("ab")
	feed(e, "<S-Left>", "(")
	if text(e) != "a(" {
		t.Errorf("autopairs off: %q", text(e))
	}
}

// The rest of the keys over a selection and without: Up and Down end it,
// Delete and C-w take it out and nothing more, C-w goes back past where
// typing began, a drag goes on from where it began.
func TestPlainSelectionKeys(t *testing.T) {
	for _, k := range []string{"<Up>", "<Down>"} {
		e := plain("ab\ncd")
		e.Goto(Pos{0, 0})
		feed(e, "<S-Right>", k)
		if e.HasSelection() {
			t.Errorf("%s keeps it", k)
		}
	}
	e := plain("ab\ncd")
	e.Goto(Pos{1, 1})
	feed(e, "<S-Up>")
	if got := selected(e); got != "b\nc" {
		t.Errorf("S-Up: %q", got)
	}
	for _, k := range []string{"<Del>", "<C-w>"} {
		e = plain("one two")
		feed(e, "<S-Left>", "<S-Left>", k)
		if text(e) != "one t" {
			t.Errorf("%s over a selection: %q", k, text(e))
		}
	}
	e = plain("")
	feed(e, "f", "o", "o", "b", "a", "r", "<C-w>")
	if text(e) != "" {
		t.Errorf("C-w is not stopped where typing began: %q", text(e))
	}
	e = plain("hello world")
	e.Click(0, 2)
	e.Drag(0, 4)
	e.Drag(0, 6)
	if got := selected(e); got != "llo " {
		t.Errorf("a drag goes on from where it began: %q", got)
	}
	e.Replace(Pos{0, 0}, Pos{0, 5}, "HI")
	if e.HasSelection() {
		t.Error("a formatting ends the selection")
	}
}
