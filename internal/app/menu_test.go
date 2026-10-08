package app

import (
	"errors"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/exp/golden"

	"sqlmux/internal/db"
	"sqlmux/internal/ui"
)

// menuApp is a tree with a view and a materialized view in public, console_1
// open: the nodes a right-click menu is for.
func menuApp() *App {
	a := sized(160, 45, "nerd")
	a.win().focus(0)
	c := a.sess.Conns[0]
	c.Tables = append(c.Tables, db.Table{Schema: "public", Name: "v_paid", Kind: "v", Rows: -1}, db.Table{Schema: "public", Name: "mv_sum", Kind: "m", Rows: -1})
	a.sess.open[groupNode(c, "public", true)] = true // the Views
	return a
}

// nodeRect is where the tree draws the node with this text.
func nodeRect(t *testing.T, a *App, text string) uv.Rectangle {
	t.Helper()
	ns, _ := a.treeNodes()
	i := slices.IndexFunc(ns, func(n node) bool { return n.Text == text || n.id == text })
	if i < 0 {
		t.Fatalf("no node %q in\n%s", text, treeTexts(a))
	}
	return find(t, a, ui.Target{Kind: ui.KindNode, Pane: 0, I: i})
}

// rightClickNode right-clicks the node with this text, drawn first.
func rightClickNode(t *testing.T, a *App, text string) {
	t.Helper()
	r := nodeRect(t, a, text)
	a.Update(tea.MouseClickMsg{X: r.Min.X + 3, Y: r.Min.Y, Button: tea.MouseRight})
}

func menuLabels(a *App) (labels []string) {
	if a.menu != nil {
		for _, it := range a.menu.items {
			labels = append(labels, it.label)
		}
	}
	return labels
}

// A right click on a table, a view, a console file opens a menu with what
// can be done to it, the tree's cursor on the node; on any other node
// nothing opens (F8.23).
func TestMenuItemsByNode(t *testing.T) {
	a := menuApp()
	for _, c := range []struct {
		node string
		want []string
	}{
		{"t_user", []string{"打开", "在新 tab 打开", "删除表…"}},
		{"v_paid", []string{"打开", "在新 tab 打开", "删除视图…"}},
		{"mv_sum", []string{"打开", "在新 tab 打开", "删除视图…"}},
		{"consoles/console_1", []string{"打开", "重命名…", "删除…"}},
		{"public", nil},
		{"Tables (14)", nil},
		{"Consoles", nil},
		{"pane-1", nil},
	} {
		a.menu = nil
		a.View()
		rightClickNode(t, a, c.node)
		if got := menuLabels(a); !slices.Equal(got, c.want) {
			t.Errorf("%s: %v, want %v", c.node, got, c.want)
		}
		if n, _ := a.treeNode(); n.Text != c.node && n.id != c.node {
			t.Errorf("%s: the cursor is on %s", c.node, n.Text)
		}
	}
}

// The menu takes j / k / the arrows, ↵ and esc, and no other key: they are no
// text for the console behind it; a click on an item runs it, a click off
// the menu closes it and does no more (F8.23).
func TestMenuKeysAndClicks(t *testing.T) {
	a := menuApp()
	data := a.win().Root.Leaves()[0]
	a.View()
	rightClickNode(t, a, "t_user")
	feed(t, a, "j<Down><Down>k<CR>")
	if a.menu != nil || tabNames(data) != "t_user" {
		t.Fatalf("↵ on the item before the last (open in a new tab): menu %v, tabs %q", a.menu != nil, tabNames(data))
	}
	a.View()
	rightClickNode(t, a, "t_order")
	if feed(t, a, "xyz"); a.menu == nil || a.menu.sel != 0 {
		t.Fatal("a key that is no menu's closed it or moved it")
	}
	feed(t, a, "<Esc>")
	if a.menu != nil || a.confirm != nil {
		t.Fatal("esc leaves the menu open")
	}
	a.View()
	rightClickNode(t, a, "t_order")
	a.View()
	click(a, find(t, a, ui.Target{Kind: ui.KindRow, I: 1}).Min)
	if a.menu != nil || tabNames(data) != "t_user t_order" {
		t.Errorf("a click on an item: menu %v, tabs %q", a.menu != nil, tabNames(data))
	}
	a.View()
	rightClickNode(t, a, "t_sku")
	a.View()
	click(a, uv.Pos(100, 20))
	if a.menu != nil || tabNames(data) != "t_user t_order" {
		t.Errorf("a click off the menu: menu %v, tabs %q", a.menu != nil, tabNames(data))
	}
	// another overlay is no place for one: the palette hides the tree, the
	// name asked in the status bar does not
	feed(t, a, "<C-p>")
	a.View()
	rightClickNode(t, a, "t_sku")
	if a.menu != nil {
		t.Error("a menu over the palette")
	}
	feed(t, a, "<Esc>")
	a.win().focus(a.win().Root.Leaves()[1].ID)
	a.run("console.rename", 0)
	if a.askName == nil {
		t.Fatal("no name asked")
	}
	a.View()
	rightClickNode(t, a, "t_sku")
	if a.menu != nil {
		t.Error("a menu while a name is typed")
	}
}

// The menu's rename is console.rename, the console opened first when it is
// not; its delete is tree.delete (F8.23).
func TestMenuRenameAndDelete(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	a := menuApp()
	consoleFile(t, "console_2", "two\n")
	a.View()
	rightClickNode(t, a, "consoles/console_2")
	feed(t, a, "jj<CR>")
	if a.confirm == nil || a.confirm.text != "删除 console_2？" {
		t.Fatalf("delete: %v", a.confirm)
	}
	feed(t, a, "<Esc>")
	a.View()
	rightClickNode(t, a, "consoles/console_2")
	feed(t, a, "j<CR>")
	if a.askName == nil || consoleOf(a.focused()) == nil || consoleOf(a.focused()).name != "console_2" {
		t.Fatalf("rename: asked %v, on %v", a.askName != nil, consoleOf(a.focused()))
	}
}

// dd on a table or a view asks to drop it, the DROP in the question, the title in
// the error color, whatever confirm_dangerous says; on a schema it does
// nothing; ↵ drops, and the tabs of it in every window go (F8.23).
func TestDropTable(t *testing.T) {
	a := menuApp()
	a.confirmDangerous = false
	data := a.win().Root.Leaves()[0]
	feed(t, a, "<C-p>@t_user<CR><C-p>@t_sku<CR>")
	if tabNames(data) != "t_user t_sku" {
		t.Fatalf("tabs %q", tabNames(data))
	}
	a.win().focus(0)
	treeTo(t, a, "public")
	if feed(t, a, "dd"); a.confirm != nil {
		t.Fatal("dd on a schema asks")
	}
	for node, want := range map[string]string{"t_user": `DROP TABLE "public"."t_user"`, "v_paid": `DROP VIEW "public"."v_paid"`, "mv_sum": `DROP MATERIALIZED VIEW "public"."mv_sum"`} {
		treeTo(t, a, node)
		if feed(t, a, "dd"); a.confirm == nil || !a.confirm.danger || !strings.Contains(a.confirm.text, want) {
			t.Fatalf("%s: %+v", node, a.confirm)
		}
		feed(t, a, "<Esc>")
	}
	treeTo(t, a, "t_user")
	feed(t, a, "dd")
	golden.RequireEqual(t, a.render().String())
	busy := a.sess.Conns[0].busy // the pages of the two tabs
	_, cmd := a.Update(teaKey("<CR>"))
	if a.confirm != nil || cmd == nil || a.sess.Conns[0].busy != busy+1 {
		t.Fatalf("↵: confirm %v, cmd %v, busy %d, was %d", a.confirm != nil, cmd != nil, a.sess.Conns[0].busy, busy)
	}
	m, ok := cmd().(dropped)
	if !ok || m.err != nil || m.table.Name != "t_user" {
		t.Fatalf("answer %+v", m)
	}
	a.Update(m)
	if tabNames(data) != "t_sku" || a.sess.Conns[0].busy != busy || a.toast != "已删除 public.t_user" {
		t.Errorf("dropped: tabs %q, busy %d, toast %q", tabNames(data), a.sess.Conns[0].busy, a.toast)
	}
}

// A drop that the server refuses is its message in a toast, and the tabs stay (F8.23).
func TestDropTableFails(t *testing.T) {
	a := menuApp()
	data := a.win().Root.Leaves()[0]
	feed(t, a, "<C-p>@t_user<CR>")
	a.win().focus(0)
	treeTo(t, a, "t_user")
	a.Update(dropped{a.sess.Conns[0], db.Table{Schema: "public", Name: "t_user"}, errors.New("boom")})
	if tabNames(data) != "t_user" || a.toast != "boom" {
		t.Errorf("tabs %q, toast %q", tabNames(data), a.toast)
	}
}

// Not dropped on a read-only connection, nor in the manual transaction mode
// (F8.23).
func TestDropTableRefused(t *testing.T) {
	for _, c := range []struct {
		set  func(c *Conn)
		want string
	}{
		{func(c *Conn) { c.readOnly = true }, "doraemon 是只读连接"},
		{func(c *Conn) { c.Manual = true }, "手动事务模式下不能删表：先提交或回滚，或切回自动"},
	} {
		a := menuApp()
		c.set(a.sess.Conns[0])
		treeTo(t, a, "t_user")
		a.win().focus(0)
		if feed(t, a, "dd"); a.confirm != nil || a.toast != c.want {
			t.Errorf("%q: asked %v, toast %q", c.want, a.confirm != nil, a.toast)
		}
	}
}

// The menu of a table, at the pointer, its first item selected (F8.23).
func TestGoldenTableMenu(t *testing.T) {
	a := menuApp()
	a.View()
	rightClickNode(t, a, "t_user")
	golden.RequireEqual(t, a.render().String())
}

// The tree's filter has the keys while it is typed in, the menu opened over
// it takes them: what it has no key for is not typed into the filter (F8.23).
func TestMenuKeysAreNotTypedInTheFilter(t *testing.T) {
	a := menuApp()
	feed(t, a, "/")
	if !a.win().tree.filtering {
		t.Fatal("the filter is not typed in")
	}
	a.View()
	rightClickNode(t, a, "t_user")
	if a.menu == nil {
		t.Fatal("no menu")
	}
	feed(t, a, "xyz")
	if got := a.win().tree.filter.Text; got != "" {
		t.Errorf("the filter has %q", got)
	}
}
