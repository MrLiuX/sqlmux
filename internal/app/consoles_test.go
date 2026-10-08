package app

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/charmbracelet/x/exp/golden"

	"sqlmux/internal/config"
)

// consoleFile writes a console file with text in it.
func consoleFile(t *testing.T, name, text string) {
	t.Helper()
	if err := os.MkdirAll(config.ConsolePath(name)[:strings.LastIndex(config.ConsolePath(name), "/")], 0o700); err != nil {
		t.Fatal(err)
	}
	if err := config.WriteConsole(config.ConsolePath(name), text); err != nil {
		t.Fatal(err)
	}
}

// consoleNode puts the tree cursor on the console file called name.
func consoleNode(t *testing.T, a *App, name string) {
	t.Helper()
	ns, _ := a.treeNodes()
	i := slices.IndexFunc(ns, func(n node) bool { return n.id == "consoles/"+name })
	if i < 0 {
		t.Fatalf("no console %s in\n%s", name, treeTexts(a))
	}
	a.win().focus(0)
	a.treeGo(i)
}

// consoleNames is the names under the Consoles root.
func consoleNames(a *App) (names []string) {
	ns, _ := a.treeNodes()
	for _, n := range ns {
		if n.kind == nodeConsoleFile {
			names = append(names, n.console)
		}
	}
	return names
}

// A new console is the first number with no file and no open console: a
// closed one keeps its file, and its number, till the file is deleted; one
// open that has no file yet keeps its number too (F8.21).
func TestNewConsoleNumbers(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	a := sized(160, 45, "nerd")
	for _, n := range []string{"console_1", "console_2", "console_3"} {
		consoleFile(t, n, n+"\n")
	}
	if got := a.freeConsole(); got != "console_4" {
		t.Errorf("with console_1 to _3: %s", got)
	}
	if err := os.Remove(config.ConsolePath("console_2")); err != nil {
		t.Fatal(err)
	}
	if got := a.freeConsole(); got != "console_2" {
		t.Errorf("console_2 deleted: %s", got)
	}
	// console_1 is open, as it is at start, console_2 too and with no file
	a.run("console.new", 0)
	if got := consoleOf(a.focused()).name; got != "console_2" {
		t.Fatalf("console.new: %s", got)
	}
	if got := a.freeConsole(); got != "console_4" {
		t.Errorf("console_2 open with no file: %s", got)
	}
	if text := strings.Join(consoleOf(a.focused()).ed.Lines(), ""); text != "" {
		t.Errorf("the new console_2 is not new: %q", text)
	}
}

// The Consoles root lists the files by number and the open ones with none
// yet, the connection last used at the right, those open in focus color; ↵
// goes to one open, and opens one not, on the connection it was on; a file
// is listed once it is written, and a name once it is changed (F8.21).
func TestConsolesNode(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	a := sized(160, 45, "nerd")
	consoleFile(t, "console_10", "ten\n")
	consoleFile(t, "console_2", "two\n")
	consoleFile(t, "scratch", "select 1\n")
	a.state.Consoles = map[string]config.ConsoleState{"console_2": {Conn: "doraemon", Schema: "agentable"}}
	if got := consoleNames(a); !slices.Equal(got, []string{"console_1", "console_2", "console_10", "scratch"}) {
		t.Fatalf("listed: %v", got)
	}
	ns, _ := a.treeNodes()
	for _, n := range ns {
		if n.kind != nodeConsoleFile {
			continue
		}
		if want := n.console == "console_1"; n.Current != want {
			t.Errorf("%s: open is %v", n.console, n.Current)
		}
		if want := map[string]string{"console_2": "doraemon"}[n.console]; n.Note != want {
			t.Errorf("%s: connection %q", n.console, n.Note)
		}
	}
	// ↵ on one that is not open: a tab of the data pane, on its connection and schema
	consoleNode(t, a, "console_2")
	feed(t, a, "<CR>")
	c := consoleOf(a.focused())
	if c == nil || c.name != "console_2" || text(c) != "two" || c.conn != a.sess.Conns[0] || c.schema != "agentable" {
		t.Fatalf("↵ on console_2: %+v", c)
	}
	if got := len(a.consoles()); got != 2 {
		t.Errorf("%d consoles open", got)
	}
	// ↵ on one open goes to it
	a.showPane(1)
	consoleNode(t, a, "console_2")
	feed(t, a, "<CR>")
	if c2 := consoleOf(a.focused()); c2 != c || len(a.consoles()) != 2 {
		t.Errorf("↵ on an open console: %v, %d open", c2, len(a.consoles()))
	}
	// a console with no file is listed while it is open, and once it is
	// written, closed: the directory is read again after its first write
	a.run("console.new", 0)
	n := consoleOf(a.focused())
	if !slices.Contains(consoleNames(a), n.name) || config.ConsoleExists(n.name) {
		t.Fatalf("%s open, with no file: %v", n.name, consoleNames(a))
	}
	a.consoleDid(n, n.ed.Load("select 2"))
	if err := n.flush(); err != nil {
		t.Fatal(err)
	}
	a.run("tab.close", 0)
	if len(a.consoles()) != 2 || slices.ContainsFunc(a.consoles(), func(c *consoleTab) bool { return c.name == n.name }) {
		t.Fatalf("%s not closed: %d open", n.name, len(a.consoles()))
	}
	if got := consoleNames(a); !slices.Contains(got, n.name) {
		t.Errorf("after a write and a close: %v", got)
	}
}

// ↵ on a console not open opens it on the connection state.json says it was
// on; when that is no connection of the session, on the current one (F8.21).
func TestOpenConsoleFileConnection(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	a := sized(160, 45, "nerd")
	other := readyConn("other")
	a.sess.Conns = append(a.sess.Conns, other)
	consoleFile(t, "console_2", "two\n")
	consoleFile(t, "console_3", "three\n")
	a.state.Consoles = map[string]config.ConsoleState{"console_2": {Conn: "other"}, "console_3": {Conn: "gone"}}
	for _, c := range []struct { // in this order: the current connection is the one the focused tab is on
		name string
		want *Conn
	}{{"console_3", a.sess.Conns[0]}, {"console_2", other}} {
		consoleNode(t, a, c.name)
		feed(t, a, "<CR>")
		if got := consoleOf(a.focused()); got == nil || got.name != c.name || got.conn != c.want {
			t.Errorf("%s: opened on %v, want %s", c.name, got, c.want.Spec.Name)
		}
	}
}

// A file that cannot be deleted says so, and keeps its file and what
// state.json has of it (F8.21).
func TestDeleteConsoleFileFails(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	a := sized(160, 45, "nerd")
	consoleFile(t, "console_2", "two\n")
	a.state.Consoles = map[string]config.ConsoleState{"console_2": {Conn: "doraemon"}}
	if os.Geteuid() == 0 {
		t.Skip("root deletes in a read-only directory")
	}
	dir := filepath.Dir(config.ConsolePath("x"))
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(dir, 0o700) //nolint:errcheck // the temp dir goes next
	consoleNode(t, a, "console_2")
	feed(t, a, "dd<CR>")
	if a.toast == "" || !config.ConsoleExists("console_2") || a.state.Consoles["console_2"].Conn != "doraemon" {
		t.Errorf("toast %q, file %v, state %v", a.toast, config.ConsoleExists("console_2"), a.state.Consoles)
	}
}

// tree.delete on a console file asks first, then deletes the file and what
// state.json kept of it, and the number is free again; one open is not
// asked about; on any other node it does nothing (F8.21).
func TestDeleteConsoleFile(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	a := sized(160, 45, "nerd")
	consoleFile(t, "console_2", "two\n")
	consoleFile(t, "console_3", "three\n")
	a.state.Consoles = map[string]config.ConsoleState{"console_2": {Conn: "doraemon"}, "console_3": {Conn: "doraemon"}}
	consoleNode(t, a, "console_2")
	feed(t, a, "dd")
	if a.confirm == nil || a.confirm.text != "删除 console_2？" || !config.ConsoleExists("console_2") {
		t.Fatalf("asked: %v", a.confirm)
	}
	feed(t, a, "<Esc>")
	if a.confirm != nil || !config.ConsoleExists("console_2") {
		t.Fatalf("no: asked %v, file %v", a.confirm, config.ConsoleExists("console_2"))
	}
	feed(t, a, "dd<CR>")
	if config.ConsoleExists("console_2") || config.ConsoleExists("console_3") != true || slices.Contains(consoleNames(a), "console_2") {
		t.Errorf("deleted: %v", consoleNames(a))
	}
	if _, kept := a.state.Consoles["console_2"]; kept || a.state.Consoles["console_3"].Conn != "doraemon" {
		t.Errorf("state: %v", a.state.Consoles)
	}
	if got := a.freeConsole(); got != "console_2" {
		t.Errorf("the number is free: %s", got)
	}
	// an open one is refused
	consoleNode(t, a, "console_1")
	if feed(t, a, "dd"); a.confirm != nil || a.toast != "先关掉它的 tab" {
		t.Errorf("open: asked %v, toast %q", a.confirm, a.toast)
	}
	// another node: nothing
	a.toast = ""
	treeTo(t, a, "public")
	if feed(t, a, "dd"); a.confirm != nil || a.toast != "" {
		t.Errorf("a schema: asked %v, toast %q", a.confirm, a.toast)
	}
}

// The Consoles node: one open, one closed, one renamed (F8.21).
func TestGoldenConsolesNode(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	a := sized(160, 45, "nerd")
	consoleFile(t, "console_2", "two\n")
	consoleFile(t, "console_3", "three\n")
	consoleFile(t, "report", "select 1\n")
	a.state.Consoles = map[string]config.ConsoleState{"console_2": {Conn: "doraemon"}, "report": {Conn: "doraemon"}}
	consoleNode(t, a, "console_3")
	a.sess.open["workspace"] = false
	golden.RequireEqual(t, a.render().String())
}
