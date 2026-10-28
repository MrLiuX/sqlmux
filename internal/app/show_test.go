package app

import (
	"testing"

	"github.com/kackerx/sqlmux/internal/db"
)

// applyShow hides every column except the ones --cols asked for, in that
// order, once the columns are known (§14「sqlmux --table」).
func TestApplyShow(t *testing.T) {
	d := newDataTab(nil, db.Table{})
	d.cols = db.Columns{Cols: []db.Column{{Name: "a"}, {Name: "b"}, {Name: "c"}, {Name: "d"}}}
	d.wantShow = []string{"b", "a"}
	d.applyShow()
	if d.wantShow != nil {
		t.Errorf("wantShow = %v; want nil after applyShow", d.wantShow)
	}
	for _, name := range []string{"a", "b", "c", "d"} {
		want := name != "a" && name != "b"
		if d.hidden[name] != want {
			t.Errorf("hidden[%q] = %v; want %v", name, d.hidden[name], want)
		}
	}
	if len(d.colOrder) != 2 || d.colOrder[0] != "b" || d.colOrder[1] != "a" {
		t.Errorf("colOrder = %v; want [b a]", d.colOrder)
	}
}

func TestApplyShowNoShow(t *testing.T) {
	d := newDataTab(nil, db.Table{})
	d.cols = db.Columns{Cols: []db.Column{{Name: "a"}}}
	d.applyShow() // wantShow is nil: nothing changes
	if len(d.hidden) != 0 || d.colOrder != nil {
		t.Errorf("applyShow with no wantShow changed the tab: hidden=%v colOrder=%v", d.hidden, d.colOrder)
	}
}
