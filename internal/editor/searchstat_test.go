package editor

import (
	"strings"
	"testing"
)

func stat(e *Editor) string {
	l, r, ok := e.Stat()
	if !ok {
		return ""
	}
	return l + " " + r
}

// A search leaves the line nvim has under the text: the pattern and, at the
// right end, [current/total], W before it when the search went round the end
// of the text; the line is gone with the next key that is no search. Measured
// on nvim 0.12.4 with this text (F15.9).
func TestSearchStat(t *testing.T) {
	e := New("foo a\nfoo b foo\nc")
	for _, c := range []struct{ keys, want string }{
		{"/foo<CR>", "/foo [2/3]"},
		{"n", "/foo [3/3]"},
		{"n", "/foo W [1/3]"},
		{"N", "?foo W [3/3]"},
		{"<Esc>?foo<CR>", "?foo [2/3]"},
		{"N", "/foo [3/3]"},
		{"2N", "/foo W [2/3]"}, // two steps forward: the first goes round
		{"j", ""},              // the next key is no n or N
	} {
		for _, k := range strings.Split(strings.ReplaceAll(strings.ReplaceAll(c.keys, "<Esc>", "\x00"), "<CR>", "\x01"), "") {
			switch k {
			case "\x00":
				k = "<Esc>"
			case "\x01":
				k = "<CR>"
			}
			e.Feed(k)
		}
		if got := stat(e); got != c.want {
			t.Errorf("%q: %q, want %q", c.keys, got, c.want)
		}
	}
	e.Feed("/")
	e.Feed("z")
	e.Feed("<CR>")
	if got := stat(e); got != "" || e.eff.Error == "" {
		t.Errorf("a pattern with no match: stat %q, error %q", got, e.eff.Error)
	}
}

// The count stops at 99: ">99" for what is beyond it, as nvim has it.
func TestSearchStatOver99(t *testing.T) {
	e := New(strings.Repeat("x\n", 150))
	for _, c := range []struct{ keys, want string }{
		{"/x<CR>", "/x [2/>99]"},
		{"G", ""},
		{"N", "?x [>99/>99]"},
		{"gg", ""},
	} {
		for _, k := range strings.Split(strings.ReplaceAll(c.keys, "<CR>", "\x01"), "") {
			if k == "\x01" {
				k = "<CR>"
			}
			e.Feed(k)
		}
		if got := stat(e); got != c.want {
			t.Errorf("%q: %q, want %q", c.keys, got, c.want)
		}
	}
}

// The matches are lit from a search until esc in NORMAL, the one the cursor is
// in as the current one, and n moves that (F15.9).
func TestSearchHits(t *testing.T) {
	e := New("foo a\nfoo b foo\nc")
	e.SetHeight(10)
	if len(e.Hits()) != 0 {
		t.Fatal("lit before a search")
	}
	for _, k := range []string{"/", "f", "o", "o", "<CR>"} {
		e.Feed(k)
	}
	hits := e.Hits()
	if len(hits) != 3 || hits[0].Current || !hits[1].Current || hits[2].Current || hits[1] != (Hit{1, 0, 3, true}) {
		t.Errorf("after /foo: %+v", hits)
	}
	e.Feed("n")
	if hits = e.Hits(); !hits[2].Current || hits[1].Current {
		t.Errorf("after n: %+v", hits)
	}
	e.Feed("j") // no longer in a match
	if hits = e.Hits(); len(hits) != 3 || hits[0].Current || hits[1].Current || hits[2].Current {
		t.Errorf("a cursor off the matches: %+v", hits)
	}
	e.Feed("<Esc>")
	if len(e.Hits()) != 0 {
		t.Error("esc in NORMAL puts them out")
	}
	e.Feed("n")
	if len(e.Hits()) != 3 {
		t.Error("n lights them again")
	}
	e.Feed("i")
	e.Feed("<Esc>") // the esc of INSERT leaves them, as vim's
	if len(e.Hits()) != 3 {
		t.Error("esc out of INSERT leaves them lit")
	}
	e.Feed("<Esc>")
	e.SetHeight(1) // the lines in view only
	e.Feed("/")
	e.Feed("f")
	e.Feed("<CR>")
	for _, h := range e.Hits() {
		if h.Line != e.Top() {
			t.Errorf("a match of line %d with only line %d in view", h.Line, e.Top())
		}
	}
	if len(e.Hits()) == 0 {
		t.Error("none in view")
	}
}

// With a count line under the text the cursor is not on the last row of the
// view, which the line covers (F15.9).
func TestSearchStatKeepsTheCursorUp(t *testing.T) {
	e := New(strings.Repeat("a\n", 30) + "foo")
	e.SetHeight(10)
	e.Feed("/")
	e.Feed("f")
	e.Feed("<CR>")
	if _, _, ok := e.Stat(); !ok || e.Cursor().Line-e.Top() != 8 {
		t.Errorf("the cursor on row %d of the view (top %d)", e.Cursor().Line-e.Top(), e.Top())
	}
}

// A match with no width has nothing to light.
func TestSearchHitsHaveWidth(t *testing.T) {
	e := New("foo")
	e.SetHeight(5)
	for _, k := range []string{"/", "o", "*", "<CR>"} {
		e.Feed(k)
	}
	hits := e.Hits()
	if len(hits) != 1 || hits[0].From != 1 || hits[0].To != 3 {
		t.Errorf("o* in foo: %+v", hits)
	}
}

// :s lights the pattern it used, as vim's does.
func TestSubstituteLightsMatches(t *testing.T) {
	e := New("a b a")
	e.SetHeight(5)
	for _, k := range []string{":", "s", "/", "a", "/", "c", "/", "<CR>"} {
		e.Feed(k)
	}
	if got := e.Lines()[0]; got != "c b a" || len(e.Hits()) != 1 {
		t.Errorf("%q, %d lit", got, len(e.Hits()))
	}
}
