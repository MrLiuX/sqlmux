package ui

import (
	"fmt"
	"strings"
	"testing"

	uv "github.com/charmbracelet/ultraviolet"
)

// Hovering anywhere never moves a tab, the +, or the hints; the × shown
// comes with a blank on its background.
func TestTabsNoShift(t *testing.T) {
	type cfg struct {
		names           []string
		cur, prev, keep int
		noNew           bool
		hints           []Hint
	}
	cfgs := []cfg{
		{[]string{"t_order", "t_user", "t_sku"}, 2, 0, 0, false, []Hint{{Key: "gt/gT", Action: "tab.next"}}},
		{[]string{"t_order", "t_user", "t_sku"}, 0, 1, 0, false, []Hint{{Key: "gt/gT", Action: "tab.next"}}},
		{[]string{"log", "#1", "#2"}, 1, -1, 1, true, []Hint{{Key: "C-e", Label: "导出", Action: "result.export"}}},
		{[]string{"log", "#1", "#2"}, 2, 1, 1, true, []Hint{{Key: "C-e", Label: "导出", Action: "result.export"}}},
		{[]string{"log", "#1", "#2"}, 0, -1, 1, true, []Hint{{Key: "C-e", Label: "导出", Action: "result.export"}}},
	}
	for ci, c := range cfgs {
		for w := 10; w <= 50; w++ {
			tb := Tabs{Names: c.names, Icons: make([]Icon, len(c.names)), Cur: c.cur, Prev: c.prev, Keep: c.keep, NoNew: c.noNew, Hints: c.hints, Close: Icon{Text: "x"}}
			base := NewFrame(w, 1, TokyonightStorm)
			tb.Draw(base, uv.Rect(0, 0, w, 1))
			regions := func(f *Frame) string {
				s := ""
				for _, h := range f.Hits {
					if h.Target.Kind == KindButton {
						continue
					}
					s += fmt.Sprintf("%v%v;", h.Target, h.Rect)
				}
				return s
			}
			want := regions(base)
			for x := range w {
				f := NewFrame(w, 1, TokyonightStorm)
				f.SetMouse(uv.Pos(x, 0))
				tb.Draw(f, uv.Rect(0, 0, w, 1))
				if got := regions(f); got != want {
					t.Errorf("cfg %d w %d pointer %d: regions moved\n base %s\n got  %s\n %q vs %q", ci, w, x, want, got, base.String(), f.String())
				}
				for _, h := range f.Hits {
					if h.Target.Kind != KindButton {
						continue
					}
					if h.Rect.Dx() != 2 && h.Rect.Max.X != w {
						t.Errorf("cfg %d w %d pointer %d: close %v", ci, w, x, h.Rect)
					}
					a, b := f.Buf.CellAt(h.Rect.Min.X, 0), f.Buf.CellAt(h.Rect.Max.X-1, 0)
					if a.Style.Bg != b.Style.Bg {
						t.Errorf("cfg %d w %d pointer %d: × %v and blank bg differ", ci, w, x, h.Rect)
					}
				}
			}
		}
	}
}

// Tabs that do not fit scroll: the current one is in sight, ‹N and N› count what
// is not, a click on them is an action to scroll, and a scroll the user made stays
// until the current tab or the width changes (F17.1).
func TestTabsScroll(t *testing.T) {
	var names []string
	for i := 1; i <= 10; i++ {
		names = append(names, fmt.Sprintf("t%d", i))
	}
	sc := &TabScroll{}
	draw := func(cur int) (*Frame, string) {
		tb := Tabs{Names: names, Icons: make([]Icon, 10), Cur: cur, Prev: -1, Scroll: sc}
		f := NewFrame(40, 1, TokyonightStorm)
		tb.Draw(f, uv.Rect(0, 0, 40, 1))
		return f, f.String()
	}
	for _, c := range []struct {
		cur  int
		want string
	}{
		{0, " 1:t1* │ 2:t2 │ 3:t3 │ 4:t4 │ 6› │ +    "},
		{3, " 1:t1 │ 2:t2 │ 3:t3 │ 4:t4* │ 6› │ +    "},
		{7, "‹4 5:t5 │ 6:t6 │ 7:t7 │ 8:t8* │ 2› │ +  "},
		{9, "‹6 7:t7 │ 8:t8 │ 9:t9 │ 10:t10* │ +     "},
		{2, "‹2 3:t3* │ 4:t4 │ 5:t5 │ 6:t6 │ 4› │ +  "},
	} {
		if _, got := draw(c.cur); got != c.want {
			t.Errorf("cur %d: %q, want %q", c.cur, got, c.want)
		}
	}
	// the arrows are clickable, and the first goes by the screen
	f, _ := draw(2)
	var acts []string
	for _, h := range f.Hits {
		if h.Target.Action != "" && h.Target.Action != "tab.new" {
			acts = append(acts, h.Target.Action)
		}
	}
	if fmt.Sprint(acts) != "[tab.scroll 0 -1 tab.scroll 0 1]" || sc.Shown != 4 {
		t.Errorf("actions %v, shown %d", acts, sc.Shown)
	}
	sc.First += sc.Shown // a click on 4›: the current tab is out of sight, and stays so
	if _, got := draw(2); got != "‹6 7:t7 │ 8:t8 │ 9:t9 │ 10:t10 │ +      " {
		t.Errorf("scrolled: %q", got)
	}
	sc.First += sc.Shown // and on to the end: the room after the last tab is for the ones before it
	if _, got := draw(2); got != "‹6 7:t7 │ 8:t8 │ 9:t9 │ 10:t10 │ +      " {
		t.Errorf("past the end: %q", got)
	}
	for w := 24; w <= 60; w++ { // the + and the current tab are shown whole at any width
		for cur := range names {
			tb := Tabs{Names: names, Icons: make([]Icon, 10), Cur: cur, Prev: -1, Scroll: &TabScroll{}}
			f := NewFrame(w, 1, TokyonightStorm)
			tb.Draw(f, uv.Rect(0, 0, w, 1))
			if got := f.String(); !strings.Contains(got, " + ") || !strings.Contains(got, fmt.Sprintf(" %d:t%d* ", cur+1, cur+1)) && !strings.Contains(got, fmt.Sprintf(" %d:t%d*│", cur+1, cur+1)) {
				t.Errorf("w %d cur %d: %q", w, cur, got)
			}
		}
	}
	tb := Tabs{Names: names[:3], Icons: make([]Icon, 3), Cur: 0, Prev: -1, Scroll: sc} // they fit again: no arrows
	f = NewFrame(40, 1, TokyonightStorm)
	tb.Draw(f, uv.Rect(0, 0, 40, 1))
	if got := f.String(); got != " 1:t1* │ 2:t2 │ 3:t3 │ +                " {
		t.Errorf("fitting: %q", got)
	}
}
