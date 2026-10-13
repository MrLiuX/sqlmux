package editor

import (
	"strings"
	"testing"
)

// A text replaced, as a formatter's gq does, is seen from its left: the view the
// longer line it replaced had, scrolled sideways, is not kept, which showed the
// new lines without their first columns (F14.8).
func TestReplaceViewsFromTheLeft(t *testing.T) {
	e := New("select id, name from t_user where id = 1 and name = 'a' order by id")
	e.SetWidth(52)
	e.SetHeight(10)
	e.Feed("A")
	e.Feed("<Esc>")
	if e.Left() == 0 {
		t.Fatal("the long line did not scroll the view: the test is no test")
	}
	e.Replace(Pos{0, 0}, Pos{0, len(e.Lines()[0])}, "select\n  id,\n  name\nfrom\n  t_user\nwhere\n  id = 1\n  and name = 'a'\norder by\n  id")
	if e.Left() != 0 || e.Cursor() != (Pos{9, 2}) {
		t.Errorf("after the replace: left %d, cursor %v", e.Left(), e.Cursor())
	}
}

// The cursor of the last line put in is on its first non-blank, which may be
// further right than the view is wide: the view is worked out from 0 for it, as
// nvim does after a formatprg (ex_cmds.c do_filter), 9 for a width of 52 and an
// indent of 60 (nvim 0.12.4 measured) (F14.8).
func TestReplaceViewFollowsTheCursor(t *testing.T) {
	e := New("select 1")
	e.SetWidth(52)
	e.SetHeight(10)
	e.Replace(Pos{0, 0}, Pos{0, len(e.Lines()[0])}, "a\n"+strings.Repeat(" ", 60)+"b")
	if e.Cursor() != (Pos{1, 60}) || e.Left() != 9 {
		t.Errorf("cursor %v, left %d", e.Cursor(), e.Left())
	}
}

// FitView shows the text from its start when its longest line fits the view with
// the column past its end, and leaves the view alone when it does not (F15.5).
func TestFitView(t *testing.T) {
	for _, c := range []struct{ line, width, left, want int }{
		{50, 52, 3, 0},  // fits with room
		{51, 52, 3, 0},  // fits with the cursor's column
		{52, 52, 3, 3},  // the cursor's column would not
		{200, 52, 3, 3}, // far too long
		{50, 52, 0, 0},
	} {
		e := New(strings.Repeat("x", c.line))
		e.SetWidth(c.width)
		e.left = c.left
		e.FitView(c.line)
		if e.Left() != c.want {
			t.Errorf("%d columns of text in %d, left %d: %d, want %d", c.line, c.width, c.left, e.Left(), c.want)
		}
	}
}

// Version moves with a change of the text and with nothing else (F15.5).
func TestVersion(t *testing.T) {
	e := New("one\ntwo")
	v := e.Version()
	e.Feed("j")
	e.Feed("l")
	if e.Version() != v {
		t.Error("moving changed it")
	}
	for _, k := range []string{"x", "u", "o", "<Esc>", "d", "d"} {
		was := e.Version()
		e.Feed(k)
		if k != "<Esc>" && k != "d" && e.Version() == was {
			t.Errorf("%s did not change it", k)
		}
	}
	was := e.Version()
	e.Load("other")
	if e.Version() == was {
		t.Error("Load did not change it")
	}
}
