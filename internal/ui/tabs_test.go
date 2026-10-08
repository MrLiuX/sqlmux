package ui

import (
	"fmt"
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
