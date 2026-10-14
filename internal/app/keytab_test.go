package app

import (
	"testing"

	"github.com/kackerx/sqlmux/internal/db"
)

// A key's tab is kept in a layout as its key and db, its pages not: they go by
// cursors a new start has none of. It comes back named for the key, waiting for its
// connection, whatever the db it was in (F16.3).
func TestKeyTabLayout(t *testing.T) {
	a, _, alpha := withAlpha(t)
	alpha.Spec.Engine = "redis"
	d := newDataTab(alpha, db.Table{Name: "app:users", Kind: "k"})
	d.pageNo, d.applied = 3, "a*"
	data := a.win().Root.A.Pane
	data.Tabs = []Tab{{Name: "app:users", Data: d}}
	lt := paneLayout(data).Tabs[0]
	if lt.Kind != "key" || lt.Key != "app:users" || lt.Table != "" || lt.DB != "alpha" || lt.Page != 0 || lt.Where != "a*" {
		t.Fatalf("layout tab %+v", lt)
	}
	tb, warn := a.sess.restoreTab(lt, map[string]bool{})
	if got := tb.Data; warn != "" || got == nil || got.table.Kind != "k" || got.table.Name != "app:users" || got.conn != alpha || !got.pending || got.applied != "a*" || tb.Name != "app:users" {
		t.Errorf("restored %+v %q", tb.Data, warn)
	}
}
