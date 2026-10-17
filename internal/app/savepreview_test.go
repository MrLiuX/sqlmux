package app

import (
	"fmt"
	"image/color"
	"strings"
	"testing"

	"github.com/charmbracelet/x/exp/golden"

	"github.com/kackerx/sqlmux/internal/db"
	"github.com/kackerx/sqlmux/internal/db/redis"
)

// previewSetup is the orders table in a pane of its own, its Main a tagDB that says
// what is sent, the preview on.
func previewSetup(t *testing.T) (*App, *dataTab, *tagDB) {
	t.Helper()
	a := sizedWith(160, 45, userConfig(t, "", ""))
	a.removePane(2)
	a.win().focus(1)
	tab := loadOrders(t, a, 3)
	d := &tagDB{}
	a.sess.Conns[0].Main = db.NewWorker(d)
	a.previewSave = true
	return a, tab, d
}

// previewAsked is a save of every kind of change, asked: row 1 deleted, a cell of
// rows 2 and 3 changed, a row added.
func previewAsked(t *testing.T) (*App, *dataTab, *tagDB) {
	t.Helper()
	a, tab, d := previewSetup(t)
	feed(t, a, "dd")        // row 1 deleted
	feed(t, a, "jlix<Esc>") // a cell of row 2
	feed(t, a, "jix<Esc>")  // and the same column of row 3
	feed(t, a, "olidone<Esc>")
	a.Update(teaKey("<C-s>"))
	return a, tab, d
}

// A save asks first, with the statements it would send as the source writes them
// and how many rows it changes, adds and deletes: n or esc leaves the table as it
// is, y sends them (F17.10).
func TestSavePreview(t *testing.T) {
	a, tab, d := previewAsked(t)
	if a.confirm == nil || len(d.sqls) != 0 {
		t.Fatalf("C-s: confirm %v, sent %q", a.confirm != nil, d.sqls)
	}
	lines := strings.Split(a.confirm.text, "\n")
	if lines[0] != "保存到 t_order：改 2 行、增 1 行、删 1 行" || len(lines) != 5 || a.confirm.yes != "保存" {
		t.Fatalf("the box:\n%s", a.confirm.text)
	}
	if !strings.HasPrefix(lines[1], `  delete from "public"."t_order" where "id" = '1'`) || !strings.HasPrefix(lines[2], `  update "public"."t_order" set "status" = '`) || !strings.HasPrefix(lines[3], `  update "public"."t_order" set`) || lines[4] != `  insert into "public"."t_order" ("status") values ('done')` {
		t.Errorf("the statements:\n%s", a.confirm.text)
	}
	feed(t, a, "n")
	if a.confirm != nil || len(d.sqls) != 0 || tab.saving || len(tab.deleted) != 1 || tab.changes() != 4 {
		t.Fatalf("n: confirm %v, sent %q, marks %d, changes %d", a.confirm != nil, d.sqls, len(tab.deleted), tab.changes())
	}
	a.Update(teaKey("<C-s>"))
	feed(t, a, "<Esc>")
	if a.confirm != nil || len(d.sqls) != 0 || tab.changes() != 4 {
		t.Fatalf("esc: confirm %v, sent %q", a.confirm != nil, d.sqls)
	}
	a.Update(teaKey("<C-s>"))
	_, cmd := a.Update(teaKey("<CR>")) // ↵ is a yes too
	if cmd == nil || !tab.saving {
		t.Fatal("↵ sends nothing")
	}
	a.Update(cmd())
	if sent := strings.Join(d.sqls, "\n"); !strings.Contains(sent, "delete from") || strings.Count(sent, "update ") != 2 || strings.Count(sent, "insert into") != 1 {
		t.Errorf("yes: sent %q", d.sqls)
	}
}

// The preview box of an UPDATE, an INSERT and a DELETE, a screen of it.
func TestGoldenSavePreview(t *testing.T) {
	a, _, _ := previewAsked(t)
	golden.RequireEqual(t, a.render().String())
}

// The preview lists 20 statements and says how many more there are; with it off a
// save goes at once, and on a production connection it is asked whatever the
// setting says, under the connection's name (F17.10, F17.9).
func TestSavePreviewFoldsAndIsOffUnlessProduction(t *testing.T) {
	a, tab, _ := previewSetup(t)
	var stmts []string
	for i := range 25 {
		stmts = append(stmts, fmt.Sprintf("update t set a = %d", i))
	}
	tab.src = &fakeSource{n: 3, preview: stmts}
	tab.deleted = map[string]bool{"1": true}
	a.Update(teaKey("<C-s>"))
	lines := strings.Split(a.confirm.text, "\n")
	if len(lines) != 1+20+1 || lines[20] != "  update t set a = 19" || lines[21] != "  …另 5 条" {
		t.Fatalf("%d lines, the last %q", len(lines), lines[len(lines)-1])
	}
	feed(t, a, "n")
	a.previewSave, a.confirmDangerous = false, false
	if _, cmd := a.Update(teaKey("<C-s>")); a.confirm != nil || cmd == nil {
		t.Errorf("off: confirm %v, cmd %v", a.confirm != nil, cmd != nil)
	}
	tab.saving = false
	a.sess.Conns[0].Spec.Production = true
	a.Update(teaKey("<C-s>"))
	if a.confirm == nil || !strings.HasPrefix(a.confirm.text, "doraemon 是生产连接\n保存到 ") || !a.confirm.danger {
		t.Errorf("production: %+v", a.confirm)
	}
}

// A statement is one line in the box: a line break is ⏎, a tab a space, a CR gone.
func TestSavePreviewOneLineEach(t *testing.T) {
	a, tab, _ := previewSetup(t)
	tab.src = &fakeSource{n: 3, preview: []string{"insert into t values ('a\nb\r\tc')"}}
	tab.deleted = map[string]bool{"1": true}
	a.Update(teaKey("<C-s>"))
	if lines := strings.Split(a.confirm.text, "\n"); len(lines) != 2 || lines[1] != "  insert into t values ('a⏎b c')" {
		t.Errorf("%q", lines)
	}
}

// Every write on a production connection is asked about, whatever confirm_dangerous
// says: the save, a query's write, the tree's drop, a Redis key's delete, rename,
// TTL, and a namespace's delete, a console's write command and the quick SQL's; the
// question is titled by the connection's name, and nothing is sent before a yes. A
// connection that is none asks only where it did before (F17.9, §13).
func TestProductionWritesAreAsked(t *testing.T) {
	const title = "doraemon 是生产连接\n"
	asked := func(a *App, entry string) {
		t.Helper()
		if c := a.confirm; c == nil || !strings.HasPrefix(c.text, title) || !c.danger || c.then == nil {
			t.Errorf("%s: %+v", entry, c)
		}
		a.confirm = nil
	}
	// SQL
	a := sizedWith(160, 45, userConfig(t, "", ""))
	tab := loadOrders(t, a, 3)
	d := &tagDB{}
	a.sess.Conns[0].Main = db.NewWorker(d)
	a.previewSave, a.confirmDangerous = false, false
	a.sess.Conns[0].Spec.Production = true
	feed(t, a, "lix<Esc>")
	a.Update(teaKey("<C-s>"))
	asked(a, "save")
	tab.saving = false
	feed(t, a, "<C-l>iupdate t_order set note = 'x'<Esc>") // a query on the connection, a write
	q := a.focusedQuery()
	if cmd := a.queryRun(""); cmd != nil || q.running != nil {
		t.Errorf("a query's write is sent: %v", cmd != nil)
	}
	asked(a, "query")
	a.win().focus(0)
	ns, _ := a.treeNodes()
	for i, n := range ns {
		if n.kind == nodeTable {
			a.treeGo(i)
			break
		}
	}
	a.dropTable()
	asked(a, "drop table")
	if len(d.sqls) != 0 {
		t.Errorf("sent before a yes: %q", d.sqls)
	}
	// Redis
	b, console := redisConsole(t, &execDB{}, someCommands(), "SET k v")
	b.confirmDangerous = false
	c := b.sess.Conns[0]
	c.Spec.Production = true
	key := node{kind: nodeRedisKey, conn: c, redisKey: redis.KeyInfo{Name: "k", Type: "string"}}
	b.deleteRedisKey(key)
	asked(b, "delete key")
	b.renameRedisKey(key)
	b.askName.done(b, "k2")
	asked(b, "rename key")
	b.ttlKey(key)
	b.askName.done(b, "60")
	asked(b, "ttl")
	for _, typed := range []string{"<CR>", "0<CR>"} { // blank and 0 are no expiry, which EXPIRE 0 would not be
		b.ttlKey(key)
		feed(t, b, typed)
		if b.confirm == nil || !strings.Contains(b.confirm.text, "：不过期？") {
			t.Errorf("TTL %q: %+v", typed, b.confirm)
		}
		b.confirm = nil
	}
	b.gotNamespaceFound(namespaceFound{conn: c, space: "ns:", keys: []string{"ns:1", "ns:2"}})
	asked(b, "delete namespace")
	b.palette = &palette{conn: c, quick: &quickSQL{}}
	b.redisQuick(c, "SET k v")
	asked(b, "quick SQL")
	b.palette = nil
	if cmd := b.queryRun(""); cmd != nil || console.running != nil {
		t.Errorf("a console's write is sent: %v", cmd != nil)
	}
	asked(b, "console")
	// the same entries where it is none: nothing is asked
	c.Spec.Production = false
	if b.deleteRedisKey(key); b.confirm != nil {
		t.Error("a key's delete asked on a connection that is none, with confirm_dangerous off")
	}
	b.confirm = nil
	b.palette = &palette{conn: c, quick: &quickSQL{}}
	b.redisQuick(c, "SET k v")
	if b.confirm != nil {
		t.Error("quick SQL asked on a connection that is none")
	}
}

// A production connection says so: the border of its panes, the connection block of
// the status bar and the titles of its tabs are in the production color; the others
// are not (F17.9).
func TestProductionMarks(t *testing.T) {
	a := sizedWith(160, 45, userConfig(t, "", ""))
	loadOrders(t, a, 3)
	th := *a.theme
	th.Production = color.RGBA{R: 1, G: 2, B: 3, A: 255} // not the error color, which the tab's own marks could be
	a.theme = &th
	p := a.focused()
	colors := func() (border, status, title color.Color) {
		f := a.render()
		r := a.layout()[p.ID]
		border = f.Buf.CellAt(r.Min.X, r.Min.Y+3).Style.Fg // the left edge, below the corner
		for y := a.h - 1; y >= 0; y-- {
			for x := range a.w {
				if c := f.Buf.CellAt(x, y); c != nil && c.Content == "d" && f.Buf.CellAt(x+1, y).Content == "o" && f.Buf.CellAt(x+2, y).Content == "r" && y == a.h-1 {
					status = c.Style.Fg // doraemon in the connection block
				}
			}
		}
		in := bodyRect(r)
		for x := in.Min.X; x < in.Max.X; x++ {
			if c := f.Buf.CellAt(x, in.Max.Y); c != nil && c.Content == "t" && f.Buf.CellAt(x+1, in.Max.Y).Content == "_" { // t_order in the tab bar
				title = c.Style.Fg
			}
		}
		return border, status, title
	}
	b, s, ti := colors()
	if b == th.Production || s == th.Production || ti == th.Production {
		t.Errorf("a connection that is none: %v %v %v", b, s, ti)
	}
	a.sess.Conns[0].Spec.Production = true
	b, s, ti = colors()
	if b != th.Production || s != th.Production || ti != th.Production {
		t.Errorf("production: border %v, status %v, tab title %v, want %v", b, s, ti, th.Production)
	}
}

func TestGoldenProduction(t *testing.T) {
	a := sizedWith(160, 45, userConfig(t, "", ""))
	loadOrders(t, a, 3)
	a.sess.Conns[0].Spec.Production = true
	golden.RequireEqual(t, a.render().Buf.Render())
}

// A key tab's source knows the key's type from what the last page said, which is
// what its commands are written by (F17.10).
func TestKeyTabPreviewsItsCommands(t *testing.T) {
	a, _ := redisConsole(t, &execDB{}, someCommands(), "")
	c := a.sess.Conns[0]
	tab := &dataTab{conn: c, table: db.Table{Schema: "0", Name: "h", Kind: "k"}, info: []db.Attr{{Name: "type", Value: "hash"}}}
	ch := db.Changes{Updates: []db.Row{{Key: []string{"a"}, Cols: []db.Change{{Name: "value", Val: db.Val{S: "x y"}}}}}, Deletes: [][]string{{"b"}}}
	if got := a.source(tab, db.Columns{}).Preview(ch); strings.Join(got, "|") != `HDEL h b|HSET h a "x y"` {
		t.Errorf("hash: %q", got)
	}
	tab.info = nil // before the first page it said nothing
	if got := a.source(tab, db.Columns{}).Preview(ch); len(got) != 0 {
		t.Errorf("no type: %q", got)
	}
}
