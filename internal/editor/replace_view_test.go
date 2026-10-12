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
