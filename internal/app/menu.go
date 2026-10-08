package app

import (
	"context"
	"fmt"

	tea "charm.land/bubbletea/v2"
	uv "github.com/charmbracelet/ultraviolet"

	"sqlmux/internal/db"
	"sqlmux/internal/i18n"
	"sqlmux/internal/ui"
)

// menu is the right-click menu over a tree node (§7.8「右键菜单」, F8.23): the
// node is under the tree's cursor, which is what the items act on.
type menu struct {
	at    uv.Position // the pointer
	items []menuItem
	sel   int
}

type menuItem struct {
	label string
	run   func() tea.Cmd
}

func inMenu(a *App) bool { return a.menu != nil }

// menuItems is what n offers; none for a node that has no menu.
func (a *App) menuItems(n node) []menuItem {
	switch n.kind {
	case nodeConsoleFile:
		return []menuItem{
			{i18n.T("打开"), func() tea.Cmd { return a.treeOpen(false) }},
			{i18n.T("重命名…"), a.renameConsoleFile},
			{i18n.T("删除…"), a.deleteConsole},
		}
	case nodeTable:
		drop := i18n.T("删除表…")
		if n.table.View() {
			drop = i18n.T("删除视图…")
		}
		return []menuItem{
			{i18n.T("打开"), func() tea.Cmd { return a.treeOpen(false) }},
			{i18n.T("在新 tab 打开"), func() tea.Cmd { return a.treeOpen(true) }},
			{drop, a.dropTable},
		}
	case nodeConn, nodeSchema, nodeGroup, nodeColumn, nodeWorkspace, nodeWindow, nodePane, nodeTab, nodeConsoles: // no menu
	}
	return nil
}

// rightClick opens the menu of the tree node under the pointer, the cursor
// put on it; a click while one is open only closes it, through the backdrop.
func (a *App) rightClick(p uv.Position) {
	t, _ := ui.HitAt(a.hits, p)
	if t.Kind != ui.KindNode && t.Kind != ui.KindFold || a.overlaid() || !a.endEdit() { // on the ▸ too
		return
	}
	a.focusPane(t.Pane)
	a.treeGo(t.I)
	if n, ok := a.treeNode(); ok {
		if items := a.menuItems(n); len(items) > 0 {
			a.menu = &menu{at: p, items: items}
		}
	}
}

// menuMove moves the selection by d, to the ends and no further.
func (a *App) menuMove(d int) {
	a.menu.sel = max(min(a.menu.sel+d, len(a.menu.items)-1), 0)
}

// menuRun closes the menu and runs its item i.
func (a *App) menuRun(i int) tea.Cmd {
	m := a.menu
	a.menu = nil
	if i < 0 || i >= len(m.items) {
		return nil
	}
	return m.items[i].run()
}

// drawMenu paints the menu beside the pointer, in a Complete's box: its
// corner at the pointer, moved over when it does not fit; a click off it
// closes it.
func (a *App) drawMenu(f *ui.Frame) {
	m := a.menu
	v, w := ui.Complete{Sel: m.sel}, 12
	for _, it := range m.items {
		v.Items = append(v.Items, ui.CompleteItem{Text: it.label})
		w = max(w, ui.Width(it.label)+5) // a border, a blank and the two cells Complete leaves on the right
	}
	box, rows := ui.CompleteBox(a.window(), uv.Pos(m.at.X, m.at.Y-1), w, len(v.Items), 0)
	f.Region(f.Bounds(), ui.Target{Kind: ui.KindBackdrop})
	v.Draw(f, box, rows)
}

// renameConsoleFile is the menu's rename on a console file: the console is
// gone to, opened first when it is not, and asked its name as console.rename
// does.
// ponytail: a file not open is opened to be renamed; rename the file alone if
// that bothers.
func (a *App) renameConsoleFile() tea.Cmd {
	n, ok := a.treeNode()
	if !ok || n.kind != nodeConsoleFile {
		return nil
	}
	cmd := a.openConsoleFile(n.console)
	a.askConsoleName()
	return cmd
}

// treeDelete is tree.delete (dd): a console file's deletion, a table's or a
// view's drop; on any other node it does nothing.
func (a *App) treeDelete() tea.Cmd {
	if n, ok := a.treeNode(); ok && n.kind == nodeTable {
		return a.dropTable()
	}
	return a.deleteConsole()
}

// dropSQL is the statement that drops t, as its kind is dropped (§7.8「右键菜单」).
func dropSQL(eng db.Engine, t db.Table) string {
	kind := "TABLE"
	switch t.Kind {
	case "v":
		kind = "VIEW"
	case "m":
		kind = "MATERIALIZED VIEW"
	case "f":
		kind = "FOREIGN TABLE"
	}
	return fmt.Sprintf("DROP %s %s", kind, eng.Quote(t.Schema, t.Name))
}

// dropped answers dropTable.
type dropped struct {
	conn  *Conn
	table db.Table
	err   error
}

// dropTable is tree.delete on a table or a view, and the menu's: after a
// question that shows the DROP, whatever confirm_dangerous says, it runs on
// the connection's Main. In the manual transaction mode it does not: a
// DROP would either join a transaction the catalog does not see, or, MySQL's,
// commit what is pending (F8.23).
func (a *App) dropTable() tea.Cmd {
	n, ok := a.treeNode()
	if !ok || n.kind != nodeTable {
		return nil
	}
	c, t := n.conn, n.table
	switch {
	case c.readOnly: // the tree lists tables of a connection ready only
		return a.showToast(i18n.T("%s 是只读连接", c.Spec.Name), toastTTL)
	case c.Manual:
		return a.showToast(i18n.T("手动事务模式下不能删表：先提交或回滚，或切回自动"), toastTTL)
	}
	sql := dropSQL(c.eng, t)
	title := i18n.T("删除表 %s，不能撤销：", t.Schema+"."+t.Name)
	if t.View() {
		title = i18n.T("删除视图 %s，不能撤销：", t.Schema+"."+t.Name)
	}
	a.confirm = &confirmBox{text: title + "\n  " + sql, yes: i18n.T("删除"), danger: true, then: func() tea.Cmd {
		main := c.Main
		c.busy++
		return func() tea.Msg {
			err := main.Run(context.Background(), func(ctx context.Context, conn db.Conn) error {
				_, err := conn.Exec(ctx, sql, 0)
				return err
			})
			return dropped{c, t, err}
		}
	}}
	return nil
}

// gotDropped closes the tabs of the table dropped, in every session, loads
// the catalog again and says so; a failure is the server's message.
func (a *App) gotDropped(m dropped) tea.Cmd {
	c := m.conn
	c.busy--
	if m.err != nil {
		return a.showToast(c.eng.ErrorOf(m.err).Message, toastTTL)
	}
	for _, s := range a.sessions {
		for _, p := range s.panes() {
			for i := len(p.Tabs) - 1; i >= 0; i-- {
				if d := p.Tabs[i].Data; d != nil && d.conn == c && idOf(d.table) == idOf(m.table) {
					dropTab(p, i)
				}
			}
		}
	}
	c.dropCols()
	return tea.Batch(a.loadCatalog(c), a.showToast(i18n.T("已删除 %s", m.table.Schema+"."+m.table.Name), toastTTL))
}
