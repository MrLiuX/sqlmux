package app

import (
	"slices"
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

// The state keeps a key under its connection and db, as the tree names them.
func TestKeyTableKey(t *testing.T) {
	a, c := redisTree(t, someKeys())
	_ = a
	if got := c.tableKey(db.Table{Name: "app:users:1", Kind: "k"}); got != "cache/db0/app:users:1" {
		t.Errorf("key %q", got)
	}
	if got := c.inDB("3").tableKey(db.Table{Name: "k", Kind: "k"}); got != "cache/db3/k" {
		t.Errorf("key in db 3 %q", got)
	}
}

// A key tab restored from a layout reads once its connection is up, where a table's
// waits for its catalog to have it; its TTL chip sets the TTL (F16.3).
func TestKeyTabRestoredFetches(t *testing.T) {
	a, _, alpha := withAlpha(t)
	alpha.Spec.Engine, alpha.State = "redis", connReady
	d := newDataTab(alpha, db.Table{Name: "app:users", Kind: "k"})
	d.pending = true
	data := a.win().Root.A.Pane
	data.Tabs = []Tab{{Name: "app:users", Data: d}}
	cmds := a.resolvePending(alpha)
	if len(cmds) != 1 || cmds[0] == nil || d.pending || d.seq != 1 || len(data.Tabs) != 1 {
		t.Errorf("resolved: %d cmds, pending %v, seq %d, %d tabs", len(cmds), d.pending, d.seq, len(data.Tabs))
	}
	d.info = []db.Attr{{Name: "type", Value: "hash"}, {Name: "ttl", Value: "12m03s"}}
	var actions []string
	for _, c := range a.keyInfo(d) {
		actions = append(actions, c.Label+"="+c.Action)
	}
	if want := []string{"TYPE=", "TTL=key.ttl", "RENAME=key.rename", "DELETE=key.delete"}; !slices.Equal(actions, want) {
		t.Errorf("chips %v", actions)
	}
}
