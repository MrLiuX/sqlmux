//go:build integration

package app

import (
	"slices"
	"strings"
	"testing"

	"github.com/kackerx/sqlmux/internal/db"
	"github.com/kackerx/sqlmux/internal/db/redis"
)

// openKey is key name of c opened in a tab of the focused pane, its page in.
func openKey(t *testing.T, a *App, c *Conn, name string) *dataTab {
	t.Helper()
	settle(a, a.openTable(c, db.Table{Name: name, Kind: "k"}))
	d := dataOf(a.focused())
	if d == nil || d.table.Name != name || d.page.Cols == nil {
		t.Fatalf("%s: no page in %+v", name, d)
	}
	return d
}

// rowsOf are a key tab's rows as the cells say them, a row to a string.
func cellsOf(d *dataTab) (rows []string) {
	for _, r := range d.page.Rows {
		var cells []string
		for _, v := range r {
			cells = append(cells, v.S)
		}
		rows = append(rows, strings.Join(cells, "="))
	}
	return rows
}

// A key opens in a tab named for it, its rows read by its type, and says what the
// key is on the line under the title: a hash's fields, a list's places, a stream's
// entries, with the count of them after it (F16.3).
func TestKeyTabOpens(t *testing.T) {
	a, c := redisApp(t, redis.IntegrationDSN(t), 100000)
	for _, k := range []struct {
		name, tab string
		cols      []string
		rows      []string
		typ, n    string
	}{
		{"str:plain", "str:plain", []string{"value"}, []string{"hello"}, "string", "5"},
		{"hash:user", "hash:user", []string{"field", "value"}, []string{"age=30", "email=ann@example.com", "meta={\"vip\":true}", "name=Ann"}, "hash", "4"},
		{"list:queue", "list:queue", []string{"index", "value"}, []string{"0=first", "1=second", "2=third", "3=fourth", "4=fifth"}, "list", "5"},
		{"set:tags", "set:tags", []string{"member"}, []string{"blue", "green", "red"}, "set", "3"},
		{"zset:rank", "zset:rank", []string{"member", "score"}, []string{"alice=1", "bob=2.5", "carol=3", "dave=3", "erin=10"}, "zset", "5"},
		{"stream:events", "stream:events", []string{"id", "target", "type", "user"}, []string{"1700000000000-1==login=ann", "1700000000000-2=button=click=ann", "1700000000001-1==logout=ann"}, "stream", "3"},
	} {
		d := openKey(t, a, c, k.name)
		var cols []string
		for _, col := range d.page.Cols {
			cols = append(cols, col.Name)
		}
		rows := cellsOf(d)
		for i, r := range rows {
			rows[i] = strings.ReplaceAll(r, "\x00", "")
		}
		if !slices.Equal(cols, k.cols) || !slices.Equal(rows, k.rows) {
			t.Errorf("%s: %v %v, want %v %v", k.name, cols, rows, k.cols, k.rows)
		}
		got := map[string]string{}
		for _, at := range d.info {
			got[at.Name] = at.Value
		}
		if got["type"] != k.typ || got["length"] != k.n || got["ttl"] == "" {
			t.Errorf("%s info: %v", k.name, d.info)
		}
		if p := a.focused(); p.tab().Name != k.tab {
			t.Errorf("%s: tab named %q", k.name, p.tab().Name)
		}
	}
	// it is one tab of its pane, as a table is
	n := len(a.focused().Tabs)
	openKey(t, a, c, "hash:user")
	if len(a.focused().Tabs) != n {
		t.Errorf("a second tab of one key: %d tabs", len(a.focused().Tabs))
	}
}

// A change of a cell is written to the key when saved, the row it is as it was read:
// one that someone else changed meanwhile is refused (F16.3).
func TestKeyTabSaves(t *testing.T) {
	a, c := redisApp(t, redis.OwnDB(t), 1000)
	command(t, c, "HSET", "h", "a", "1", "b", "2")
	d := openKey(t, a, c, "h")
	d.edits = map[editKey]edit{{d.rowKey(0), "value"}: {val: db.Val{S: "10"}, orig: d.page.Rows[0][1]}}
	settle(a, a.save())
	if got := command(t, c, "HGET", "h", "a").Rows[0][0].S; got != "10" {
		t.Fatalf("written: %q", got)
	}
	if rows := cellsOf(d); !slices.Equal(rows, []string{"a=10", "b=2"}) || d.bar != nil {
		t.Errorf("read back: %v %v", rows, d.bar)
	}
	command(t, c, "HSET", "h", "b", "other")
	d.edits = map[editKey]edit{{d.rowKey(1), "value"}: {val: db.Val{S: "20"}, orig: d.page.Rows[1][1]}}
	settle(a, a.save())
	if got := command(t, c, "HGET", "h", "b").Rows[0][0].S; got != "other" {
		t.Errorf("a stale row was written: %q", got)
	}
	// a read-only connection refuses
	c.readOnly = true
	d.edits = map[editKey]edit{{d.rowKey(0), "value"}: {val: db.Val{S: "x"}, orig: d.page.Rows[0][1]}}
	settle(a, a.save())
	if got := command(t, c, "HGET", "h", "a").Rows[0][0].S; got != "10" {
		t.Errorf("a read-only connection wrote: %q", got)
	}
}

// ↵ on a key in the tree opens it; the TTL, the rename and the delete the Info row
// has are done to the key of the tab, which reads again after each and goes on under
// the name it has been given (F16.3).
func TestKeyTabOperations(t *testing.T) {
	a, c := redisApp(t, redis.OwnDB(t), 1000)
	command(t, c, "SET", "k1", "v")
	command(t, c, "SET", "ns:k3", "v")
	settle(a, a.loadKeys(c))
	a.sess.open[dbNode(c, c.DB)] = true
	a.sess.open[dbNode(c, c.DB)+"/ns:ns"] = true
	ns, _ := a.treeNodes()
	i := slices.IndexFunc(ns, func(n node) bool { return n.kind == nodeRedisKey && n.redisKey.Name == "ns:k3" })
	if i < 0 {
		t.Fatalf("no node of ns:k3 in %v", outline(a))
	}
	a.treeGo(i)
	settle(a, a.treeOpen(false))
	if d := dataOf(a.focused()); d == nil || d.table.Name != "ns:k3" || d.page.Cols == nil {
		t.Fatalf("↵ on a key: %+v", d)
	}
	d := openKey(t, a, c, "k1")
	info := func(name string) (v string) {
		for _, at := range d.info {
			if at.Name == name {
				v = at.Value
			}
		}
		return v
	}
	ask := func(text string) {
		t.Helper()
		if a.askName == nil {
			t.Fatal("nothing asked")
		}
		cmd, _ := a.askName.done(a, text)
		a.askName = nil
		settle(a, cmd)
	}
	settle(a, actions["key.ttl"].Run(a, Args{}))
	ask("90")
	if got := info("ttl"); got != "1m30s" && got != "1m29s" {
		t.Errorf("ttl %q, info %v", got, d.info)
	}
	settle(a, actions["key.rename"].Run(a, Args{}))
	ask("k2")
	if d.table.Name != "k2" || a.focused().tab().Name != "k2" || cellsOf(d)[0] != "v" || d.bar != nil {
		t.Errorf("renamed: %q %q %v %v", d.table.Name, a.focused().tab().Name, cellsOf(d), d.bar)
	}
	if got := command(t, c, "EXISTS", "k1", "k2").Rows[0][0].S; got != "1" {
		t.Errorf("EXISTS k1 k2 = %s", got)
	}
	settle(a, actions["key.delete"].Run(a, Args{}))
	if a.confirm == nil {
		t.Fatal("a delete is asked about")
	}
	settle(a, a.confirm.then())
	if got := command(t, c, "EXISTS", "k2").Rows[0][0].S; got != "0" || d.bar == nil {
		t.Errorf("deleted: EXISTS %s, bar %v", got, d.bar)
	}
}
