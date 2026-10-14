package app

import (
	"slices"
	"strings"
	"testing"

	"github.com/charmbracelet/x/exp/golden"

	"github.com/kackerx/sqlmux/internal/config"
	"github.com/kackerx/sqlmux/internal/db/redis"
	"github.com/kackerx/sqlmux/internal/i18n"
	"github.com/kackerx/sqlmux/internal/ui"
)

// redisTree is an App whose first connection is a Redis one with dbs 0 and 1, db
// 0 scanned: the keys of kinds, a namespace that is a key too, and the odd ones.
func redisTree(t *testing.T, scan *keyScan) (*App, *Conn) {
	t.Helper()
	a := wide(160, 45)
	c := a.sess.Conns[0]
	c.Spec = config.Connection{Name: "cache", Engine: "redis", DSN: "redis://localhost/0"}
	c.drv, c.eng = kvDriver{}, nil
	c.Tables, c.Schemas, c.home, c.Schema = nil, nil, "", ""
	c.DB, c.Databases, c.scan = "0", []string{"0", "1"}, scan
	return a, c
}

func someKeys() *keyScan {
	var ks []redis.KeyInfo
	for _, k := range [][2]string{
		{"str:plain", "string"}, {"hash:user", "hash"}, {"app:users:1", "string"}, {"app:users:2", "string"}, {"app:orders:100", "string"},
		{"app", "string"}, {"a::b", "list"}, {":lead", "set"}, {"trail:", "zset"}, {"nodelimiter", "stream"}, {"json:doc", "ReJSON-RL"},
	} {
		ks = append(ks, redis.KeyInfo{Name: k[0], Type: k[1]})
	}
	return &keyScan{keys: ks, total: int64(len(ks))}
}

// outline is the tree's nodes down to the workspace, a line each: its depth in
// blanks, its text, its note.
func outline(a *App) (out []string) {
	ns, _ := a.treeNodes()
	for _, n := range ns {
		if n.kind == nodeWorkspace {
			break
		}
		line := strings.Repeat("  ", n.Depth) + n.Text
		if n.Note != "" {
			line += " " + n.Note
		}
		out = append(out, line)
	}
	return out
}

// openAll opens every node that has others under it.
func openAll(a *App) {
	for range 20 {
		ns, _ := a.treeNodes()
		changed := false
		for _, n := range ns {
			if n.Branch && !n.Open && (n.kind == nodeNamespace || n.kind == nodeDatabase) {
				a.sess.open[n.id], changed = true, true
			}
		}
		if !changed {
			return
		}
	}
}

// A Redis connection's tree is its dbs, db0 with its size, and under a db the
// namespaces of its keys by the delimiter with how many keys each holds, then
// the keys: a name with two delimiters in a row has a namespace with no name,
// one with a delimiter at its end a key with none, a key may be called as a
// namespace is, and the keys of a db that is not connected are not there (F16.2).
func TestRedisTree(t *testing.T) {
	a, c := redisTree(t, someKeys())
	c.State = connReady
	a.sess.open[dbNode(c, "0")] = true
	empty := i18n.T("(空)")
	if got := outline(a); !slices.Equal(got[:2], []string{"cache", "  db0 11"}) || got[len(got)-1] != "  db1" {
		t.Fatalf("closed: %q", got)
	}
	openAll(a)
	want := []string{
		"cache", "  db0 11",
		"    " + empty + " 1", "      lead",
		"    a 1", "      " + empty + " 1", "        b",
		"    app 3", "      orders 1", "        100", "      users 2", "        1", "        2",
		"    hash 1", "      user",
		"    json 1", "      doc",
		"    str 1", "      plain",
		"    trail 1", "      " + empty,
		"    app", "    nodelimiter",
		"  db1",
	}
	if got := outline(a); !slices.Equal(got, want) {
		t.Errorf("tree:\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	// the icons are of the types, in the colors of §7.7
	ns, _ := a.treeNodes()
	for _, n := range ns {
		if n.kind == nodeRedisKey {
			if want := a.icons.KeyType(n.redisKey.Type); n.Icon != want || n.IconFg != a.keyColor(n.redisKey.Type) {
				t.Errorf("%s: icon %+v, want %+v", n.redisKey.Name, n.Icon, want)
			}
		}
	}
	// ids: the db, the namespace by its path, the key by its name
	ids := map[string]bool{}
	for _, n := range ns {
		ids[n.id] = true
	}
	for _, id := range []string{"conn:cache/db:0", "conn:cache/db:0/ns:app:users", "conn:cache/db:0/key:app:users:1", "conn:cache/db:0/key:app", "conn:cache/db:0/ns:app"} {
		if !ids[id] {
			t.Errorf("no node %s", id)
		}
	}
}

// The delimiter is the connection's, else `[redis] delimiter`; the scan that
// stopped at its budget says so under the keys it read (F16.2).
func TestRedisTreeDelimiterAndBudget(t *testing.T) {
	a, c := redisTree(t, &keyScan{keys: []redis.KeyInfo{{Name: "a/b:c", Type: "string"}, {Name: "a/d", Type: "string"}}, total: 1_200_000, more: true})
	c.State = connReady
	a.sess.open[dbNode(c, "0")] = true
	openAll(a)
	note := i18n.T("已扫描 %d / %s，用 / 过滤", 2, "1.2m")
	if got := outline(a); !slices.Equal(got[:6], []string{"cache", "  db0 1.2m", "    a/b 1", "      c", "    a/d", "    " + note}) {
		t.Errorf("by the default's :\n%q", got)
	}
	a.redisDelim = "/"
	openAll(a)
	if got := outline(a); !slices.Equal(got[:6], []string{"cache", "  db0 1.2m", "    a 2", "      b:c", "      d", "    " + note}) {
		t.Errorf("by [redis] delimiter /:\n%q", got)
	}
	c.Spec.Delimiter = ":"
	openAll(a)
	if got := outline(a); !slices.Equal(got[2:4], []string{"    a/b 1", "      c"}) {
		t.Errorf("by the connection's:\n%q", got)
	}
}

// The filter asks the server, and the tree shows what it found with every
// namespace open and none of the rest; the pattern has a * at each end unless
// it has one (F16.2).
func TestRedisTreeFilter(t *testing.T) {
	for typed, want := range map[string]string{"": "", "users": "*users*", "app:*": "app:*", "*x": "*x", "a?c": "*a?c*"} {
		if got := filterPattern(typed); got != want {
			t.Errorf("filterPattern(%q) = %q, want %q", typed, got, want)
		}
	}
	a, c := redisTree(t, someKeys())
	c.State = connReady
	a.sess.open[dbNode(c, "0")] = true
	a.win().tree.filter = ui.Input{Text: "users", Pos: 5}
	if got := outline(a); len(got) != 0 { // nothing found yet: no connection kept
		t.Errorf("before: %q", got)
	}
	c.found = &keyScan{keys: []redis.KeyInfo{{Name: "app:users:1", Type: "string"}, {Name: "app:users:2", Type: "string"}, {Name: "app:users:3", Type: "string"}}, total: 11}
	if got, matches := outline(a), 0; !slices.Equal(got, []string{"cache", "  db0 11", "    app 3", "      users 3", "        1", "        2", "        3"}) {
		t.Errorf("filtered: %q %d", got, matches)
	}
	if _, matches := a.treeNodes(); matches != 3 {
		t.Errorf("matches %d", matches)
	}
	// what was found for another filter is not shown, nor asked of a db that is not connected
	c.found = nil
	a.gotFound(foundMsg{conn: c, pattern: "*other*", scan: &keyScan{keys: []redis.KeyInfo{{Name: "x", Type: "string"}}}})
	if c.found != nil {
		t.Error("a scan for a filter that is gone is shown")
	}
	a.gotFound(foundMsg{conn: c, pattern: "*users*", scan: &keyScan{keys: []redis.KeyInfo{{Name: "x", Type: "string"}}}})
	if c.found == nil {
		t.Error("a scan for the filter is not shown")
	}
	if cmd := a.findKeys(c.inDB("1")); cmd != nil {
		t.Error("a db that is not connected is asked")
	}
	// only the last change of the filter asks the servers
	a.filterSeq = 5
	if a.gotFilterDue(filterDue{3}) != nil {
		t.Error("an old change asks")
	}
	if a.gotFilterDue(filterDue{5}) == nil {
		t.Error("the last change does not ask")
	}
}

// The tree of a connected Redis connection with its db scanned (nerd icons).
func TestGoldenRedisTree(t *testing.T) {
	a, c := redisTree(t, someKeys())
	c.State = connReady
	a.sess.open[dbNode(c, "0")] = true
	openAll(a)
	golden.RequireEqual(t, a.render().String())
}
