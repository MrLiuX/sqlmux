//go:build integration

package app

import (
	"context"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/kackerx/sqlmux/internal/config"
	"github.com/kackerx/sqlmux/internal/db"
	"github.com/kackerx/sqlmux/internal/db/redis"
	"github.com/kackerx/sqlmux/internal/i18n"
	"github.com/kackerx/sqlmux/internal/keymap"
	"github.com/kackerx/sqlmux/internal/ui"
)

// redisApp is an App on a Redis connection at dsn, its db scanned, budget keys
// at most.
func redisApp(t *testing.T, dsn string, budget int) (*App, *Conn) {
	t.Helper()
	sess := openUp(t, []config.Connection{{Name: "cache", Engine: "redis", DSN: dsn}})
	cfg := config.Default()
	cfg.RedisScanKeys = budget
	keys, _ := keymap.New(cfg.Leader, cfg.Vim, cfg.Timeoutlen, cfg.Bindings)
	a := newApp(cfg, keys, sess, &config.State{}, "")
	t.Cleanup(a.Close)
	a.Update(tea.WindowSizeMsg{Width: 160, Height: 45})
	settle(a, a.loadCatalog(sess.Conns[0]))
	return a, sess.Conns[0]
}

// pressRedis is key k, and what the app does of it as it settles.
func pressRedis(t *testing.T, a *App, k keymap.Key) {
	t.Helper()
	_, cmd := a.Update(teaKey(k))
	settle(a, cmd)
}

// command runs a Redis command on c's Main for what a test sets up or looks at.
func command(t *testing.T, c *Conn, args ...string) db.Result {
	t.Helper()
	var vals []db.Val
	for _, a := range args[1:] {
		vals = append(vals, db.Val{S: a})
	}
	r, err := c.Main.Query(context.Background(), args[0], vals...)
	if err != nil {
		t.Fatal(args, err)
	}
	return r
}

// The seeded db0 is scanned whole when the budget allows it: its dbs, its size, the
// namespaces with how many keys each holds; one that is cut says how much it read
// of how much there is (F16.2).
func TestRedisTreeScan(t *testing.T) {
	a, c := redisApp(t, redis.IntegrationDSN(t), 100000)
	if !slices.Contains(c.Databases, "0") || !slices.Contains(c.Databases, "1") || c.scan == nil || c.scan.more || int64(len(c.scan.keys)) != c.scan.total || c.scan.total < 12000 {
		t.Fatalf("whole: dbs %v scan %+v", c.Databases, c.scan)
	}
	a.sess.open[dbNode(c, "0")] = true
	var counts []string
	ns, _ := a.treeNodes()
	for _, n := range ns {
		if n.kind == nodeNamespace && n.Depth == 2 && slices.Contains([]string{"app", "bulk", "cache", "hash"}, n.Text) {
			counts = append(counts, n.Text+" "+n.Note)
		}
	}
	if !slices.Equal(counts, []string{"app 6", "bulk 12000", "cache 2", "hash 3"}) {
		t.Errorf("namespaces: %v", counts)
	}
	a2, c2 := redisApp(t, redis.IntegrationDSN(t), 500)
	a2.sess.open[dbNode(c2, "0")] = true
	if !c2.scan.more || len(c2.scan.keys) != 500 || c2.scan.total != c.scan.total {
		t.Fatalf("cut: %d keys of %d, more %v", len(c2.scan.keys), c2.scan.total, c2.scan.more)
	}
	out := outline(a2)
	note := i18n.T("已扫描 %d / %s，用 / 过滤", 500, ui.Magnitude(float64(c2.scan.total)))
	if !slices.Contains(out, "    "+note) {
		t.Errorf("no word of the budget in\n%s", strings.Join(out, "\n"))
	}
	// another db is a connection of its own, dialed when it is opened
	db1 := c2.inDB("1")
	if db1 == c2 || db1.State != connIdle {
		t.Fatalf("db1: %+v", db1)
	}
	settle(a2, a2.treeFold(node{id: dbNode(c2, "1"), kind: nodeDatabase, conn: db1, TreeNode: ui.TreeNode{Branch: true}}, true))
	if db1.State != connReady || db1.DB != "1" || db1.scan == nil || db1.scan.total != 2 || db1.scan.keys[0].Type != "string" {
		t.Errorf("db1 opened: %v %q %+v", db1.State, db1.DB, db1.scan)
	}
}

// The filter asks the server for what matches, with the budget of the scan, and the
// tree shows that and nothing else; it goes when the filter does (F16.2).
func TestRedisTreeFilterOnServer(t *testing.T) {
	a, c := redisApp(t, redis.IntegrationDSN(t), 100000)
	a.sess.open[dbNode(c, "0")] = true
	a.win().tree.filter = ui.Input{Text: "users", Pos: 5}
	settle(a, a.askFilter())
	if f := a.win().tree.found[c]; f == nil || len(f.keys) != 3 {
		t.Fatalf("found: %+v", f)
	}
	want := []string{"cache", "  db0 " + ui.Magnitude(float64(c.scan.total)), "    app 3", "      users 3", "        1", "        2", "        3"}
	if got := outline(a); !slices.Equal(got, want) {
		t.Errorf("filtered:\n%s", strings.Join(got, "\n"))
	}
	if _, matches := a.treeNodes(); matches != 3 {
		t.Errorf("%d matches", matches)
	}
	// with a * of its own the pattern is as typed
	a.win().tree.filter = ui.Input{Text: "app:orders:*", Pos: 12}
	settle(a, a.askFilter())
	if got := outline(a); len(got) != 6 || got[2] != "    app 2" {
		t.Errorf("app:orders:*:\n%s", strings.Join(got, "\n"))
	}
	a.win().tree.filter = ui.Input{Text: "no-such-key", Pos: 11}
	settle(a, a.askFilter())
	if got := outline(a); len(got) != 0 {
		t.Errorf("nothing found:\n%s", strings.Join(got, "\n"))
	}
	a.win().tree.filter = ui.Input{}
	if got := outline(a); len(got) < 3 {
		t.Errorf("no filter, the whole tree:\n%s", strings.Join(got, "\n"))
	}
}

// A key is deleted at once, renamed to a name that is free or, asked, to one that is
// taken, given a TTL or none; a read-only connection has no part in any of it (F16.2).
func TestRedisKeyOperations(t *testing.T) {
	a, c := redisApp(t, redis.OwnDB(t), 10000)
	for _, k := range []string{"k1", "k2", "k3", "k4"} {
		command(t, c, "SET", k, k)
	}
	key := func(name string) node {
		return node{kind: nodeRedisKey, conn: c, redisKey: redis.KeyInfo{Name: name, Type: "string"}}
	}
	exists := func(name string) bool { return command(t, c, "EXISTS", name).Rows[0][0].S == "1" }
	// delete: asked, then UNLINK; not asked with confirm_dangerous off
	settle(a, a.deleteRedisKey(key("k1")))
	if a.confirm == nil || !strings.Contains(a.confirm.text, "k1") || !exists("k1") {
		t.Fatalf("a delete is asked about: %+v", a.confirm)
	}
	settle(a, a.confirm.then())
	a.confirm = nil
	if exists("k1") || a.toast != i18n.T("已删除键 %s", "k1") || c.busy != 0 {
		t.Errorf("deleted: toast %q busy %d", a.toast, c.busy)
	}
	if !slices.ContainsFunc(c.scan.keys, func(k redis.KeyInfo) bool { return k.Name == "k2" }) || slices.ContainsFunc(c.scan.keys, func(k redis.KeyInfo) bool { return k.Name == "k1" }) {
		t.Errorf("the tree is not read again: %+v", c.scan.keys)
	}
	// rename to a free name, and onto one that is taken
	a.renameRedisKey(key("k2"))
	if a.askName == nil || a.askName.in.Text != "k2" {
		t.Fatalf("no name asked: %+v", a.askName)
	}
	pressRedis(t, a, "<C-u>")
	for _, r := range "fresh" {
		pressRedis(t, a, keymap.Key(string(r)))
	}
	pressRedis(t, a, "<CR>")
	if exists("k2") || !exists("fresh") || a.toast != i18n.T("%s 改名为 %s", "k2", "fresh") {
		t.Errorf("renamed: toast %q", a.toast)
	}
	a.renameRedisKey(key("k3"))
	pressRedis(t, a, "<C-u>")
	pressRedis(t, a, "k")
	pressRedis(t, a, "4")
	pressRedis(t, a, "<CR>")
	if a.confirm == nil || !strings.Contains(a.confirm.text, "k4") || !exists("k3") {
		t.Fatalf("a name that is taken is asked about: confirm %+v", a.confirm)
	}
	settle(a, a.confirm.then())
	a.confirm = nil
	if exists("k3") || command(t, c, "GET", "k4").Rows[0][0].S != "k3" {
		t.Error("not overwritten")
	}
	// TTL
	ttl := func() int { n, _ := strconv.Atoi(command(t, c, "TTL", "fresh").Rows[0][0].S); return n }
	a.ttlKey(key("fresh"))
	for _, r := range "90" {
		pressRedis(t, a, keymap.Key(string(r)))
	}
	pressRedis(t, a, "<CR>")
	if n := ttl(); n < 80 || n > 90 {
		t.Errorf("a TTL of 90: %d, toast %q, ask %v", n, a.toast, a.askName)
	}
	a.ttlKey(key("fresh"))
	for _, r := range "soon" {
		pressRedis(t, a, keymap.Key(string(r)))
	}
	pressRedis(t, a, "<CR>")
	if a.askName == nil || a.toast != i18n.T("TTL 要是秒数或 90m、1h30m 这样的时长") { // asked again
		t.Errorf("a TTL that is none: toast %q ask %v", a.toast, a.askName)
	}
	pressRedis(t, a, "<C-u>")
	pressRedis(t, a, "<CR>")
	if n := ttl(); n != -1 {
		t.Errorf("no TTL: %d", n)
	}
	// read-only
	c.readOnly = true
	a.confirm = nil
	for name, ask := range map[string]func() tea.Cmd{
		"delete": func() tea.Cmd { return a.deleteRedisKey(key("fresh")) }, "rename": func() tea.Cmd { return a.renameRedisKey(key("fresh")) },
		"ttl": func() tea.Cmd { return a.ttlKey(key("fresh")) }, "namespace": func() tea.Cmd { return a.deleteNamespace(node{conn: c, prefix: "x:"}) },
	} {
		a.toast = ""
		if cmd := ask(); cmd == nil && a.toast == "" || a.toast != i18n.T("%s 是只读连接", "cache") || a.askName != nil || !exists("fresh") {
			t.Errorf("read-only %s: toast %q", name, a.toast)
		}
	}
}

// A namespace is deleted after the question that has the true number in it, 500 keys
// at a time, or with no question when confirm_dangerous is off; C-c in the middle
// says how many went (F16.2).
func TestRedisDeleteNamespace(t *testing.T) {
	dsn := redis.OwnDB(t)
	a, c := redisApp(t, dsn, 10000)
	probe, _, err := redis.Connect(context.Background(), dsn, "", false) // a connection of its own to look at the db while the delete holds Main
	if err != nil {
		t.Fatal(err)
	}
	defer probe.Close()
	command(t, c, "EVAL", "for i = 1, 1300 do redis.call('SET', 'ns:' .. i, i) end", "0")
	command(t, c, "SET", "keep", "1")
	command(t, c, "SET", "nsx", "1") // a name that starts alike is not in it
	dbsize := func() int {
		r, err := probe.Query(context.Background(), "DBSIZE")
		if err != nil {
			t.Fatal(err)
		}
		n, _ := strconv.Atoi(r.Rows[0][0].S)
		return n
	}
	space := node{kind: nodeNamespace, conn: c, prefix: "ns:"}
	// a prefix with a glob's characters in it names itself alone
	command(t, c, "SET", "we*:1", "1")
	command(t, c, "SET", "weXX:2", "1")
	a.confirmDangerous = false
	settle(a, a.deleteNamespace(node{kind: nodeNamespace, conn: c, prefix: "we*:"}))
	if command(t, c, "EXISTS", "we*:1", "weXX:2").Rows[0][0].S != "1" || command(t, c, "EXISTS", "weXX:2").Rows[0][0].S != "1" {
		t.Error("the glob of a prefix took another key")
	}
	a.confirmDangerous = true
	before := dbsize()
	settle(a, a.deleteNamespace(space))
	if a.confirm == nil || !strings.Contains(a.confirm.text, "1300") || dbsize() != before {
		t.Fatalf("asked: %+v", a.confirm)
	}
	settle(a, a.confirm.then())
	if dbsize() != before-1300 || command(t, c, "EXISTS", "keep", "nsx").Rows[0][0].S != "2" || a.toast != i18n.T("已删除 %d 个键", 1300) || c.busy != 0 {
		t.Errorf("deleted: %d of %d, toast %q busy %d", dbsize(), before, a.toast, c.busy)
	}
	a.confirm = nil
	settle(a, a.deleteNamespace(space))
	if a.confirm != nil || a.toast != i18n.T("%s 下面没有键", "ns:") {
		t.Errorf("nothing there: %+v %q", a.confirm, a.toast)
	}
	// with no question
	a.confirmDangerous = false
	command(t, c, "EVAL", "for i = 1, 600 do redis.call('SET', 'ns:' .. i, i) end", "0")
	settle(a, a.deleteNamespace(space))
	if a.confirm != nil || command(t, c, "EXISTS", "ns:7").Rows[0][0].S != "0" {
		t.Errorf("not asked, deleted: %+v", a.confirm)
	}
	// C-c: a great many keys, stopped after some
	command(t, c, "EVAL", "for i = 1, 80000 do redis.call('SET', 'ns:' .. i, i) end", "0")
	total := dbsize()
	found := a.deleteNamespace(space)().(namespaceFound)
	cmd := a.gotNamespaceFound(found)
	if cmd == nil {
		t.Fatal("no delete begun")
	}
	done := make(chan tea.Msg, 1)
	go func() { done <- cmd() }()
	for dbsize() > total-2000 { // some have gone
		time.Sleep(time.Millisecond)
	}
	c.cancel()
	var m keyDone
	select {
	case msg := <-done:
		m = msg.(keyDone)
	case <-time.After(10 * time.Second):
		t.Fatal("C-c did not end the delete")
	}
	left := dbsize()
	if left == 2 { // keep and nsx: it finished before the cancel
		t.Skip("the delete was done before C-c")
	}
	n, _ := strconv.Atoi(regexp.MustCompile(`\d+`).FindString(m.say))
	if m.say != i18n.T("已取消，已删除 %d 个键", n) || n < 2000 || n > total-left {
		t.Errorf("cancelled: %q, %d left of %d", m.say, left, total)
	}
}

// R on a db node reads its keys again, a key made meanwhile among them, and the
// menu of the node says it (F16.2).
func TestRedisTreeRefresh(t *testing.T) {
	a, c := redisApp(t, redis.OwnDB(t), 10000)
	command(t, c, "SET", "late", "1")
	ns, _ := a.treeNodes()
	at := slices.IndexFunc(ns, func(n node) bool { return n.kind == nodeDatabase })
	a.treeGo(at)
	a.focusPane(a.win().Tree.ID)
	if slices.ContainsFunc(c.scan.keys, func(k redis.KeyInfo) bool { return k.Name == "late" }) {
		t.Fatal("the key is there before the refresh")
	}
	if items := a.menuItems(ns[at]); len(items) != 1 || items[0].label != i18n.T("刷新") {
		t.Fatalf("the menu of a db: %+v", items)
	}
	settle(a, a.refreshTree())
	if !slices.ContainsFunc(c.scan.keys, func(k redis.KeyInfo) bool { return k.Name == "late" }) {
		t.Errorf("not read again: %+v", c.scan.keys)
	}
}
