package app

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/kackerx/sqlmux/internal/config"
	"github.com/kackerx/sqlmux/internal/i18n"
)

// consoleDir is the console files' names as the directory last read held
// them, and when it changed (F8.21).
type consoleDir struct {
	names []string
	at    time.Time
	read  bool
}

// consoleFiles is the console files' names (config.ConsoleFiles), read again
// when the directory changed: a console file coming and going changes it,
// what is written in one does not. The tree asks on every frame.
// ponytail: a change in the same tick of the file system's clock as the
// read before it is not seen; drop the read in rename and delete if that shows.
func (a *App) consoleFiles() []string {
	dir := filepath.Dir(config.ConsolePath("x"))
	if fi, err := os.Stat(dir); a.files.read && err == nil && fi.ModTime().Equal(a.files.at) {
		return a.files.names
	}
	names, at, err := config.ConsoleFiles()
	if err != nil { // none is listed: the tree is not where this is told
		names, at = nil, time.Time{}
	}
	a.files = consoleDir{names, at, true}
	return names
}

// consoleList is what the Consoles node lists: the files, and the consoles
// open that have none yet, a console being written once a word is typed in
// it (§7.8「Consoles 节点」, F8.21).
func (a *App) consoleList() []string {
	names := slices.Clone(a.consoleFiles())
	for _, t := range a.consoles() {
		if !slices.ContainsFunc(names, func(n string) bool { return strings.EqualFold(n, t.name) }) {
			names = append(names, t.name)
		}
	}
	config.SortConsoles(names)
	return names
}

// openConsoleFile is ↵ on a console of the Consoles node: one that is open
// is gone to, as tab.reopen does, and one that is not opens in a tab of the
// data pane that takes what is opened, on the connection it last used, the
// current one when that is not in connections.toml (§11).
func (a *App) openConsoleFile(name string) tea.Cmd {
	if a.focusConsole(name) {
		return nil
	}
	c := a.conn()
	last, kept := a.state.Consoles[name]
	if i := slices.IndexFunc(a.sess.Conns, func(cn *Conn) bool { return cn.Spec.Name == last.Conn }); kept && i >= 0 {
		c = a.sess.Conns[i]
	}
	if c == nil {
		return a.noConnection()
	}
	t, err := openConsole(c, name)
	if err != nil {
		return a.showToast(err.Error(), toastTTL)
	}
	if kept && c.Spec.Name == last.Conn {
		t.schema = last.Schema
	}
	p := a.openTarget()
	a.showPane(p.ID)
	putTab(p, Tab{Name: name, Console: t}, p.tab().landing())
	return a.keepConsole(t)
}

// deleteConsole is tree.delete on a console of the Consoles node: after a
// question the file goes, and what state.json kept of it; one that is open
// is not asked about, it has to be closed first (F8.21). On any other node
// it does nothing.
func (a *App) deleteConsole() tea.Cmd {
	n, ok := a.treeNode()
	if !ok || n.kind != nodeConsoleFile {
		return nil
	}
	name := n.console
	if a.consoleOpen(name, nil) {
		return a.showToast(i18n.T("先关掉它的 tab"), toastTTL)
	}
	a.confirm = &confirmBox{text: i18n.T("删除 %s？", name), yes: i18n.T("删除"), then: func() tea.Cmd {
		if err := config.DeleteConsole(name); err != nil {
			return a.showToast(err.Error(), toastTTL)
		}
		delete(a.state.Consoles, name)
		a.clampTree()
		return a.saveState()
	}}
	return nil
}
