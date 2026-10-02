package app

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/x/exp/golden"

	"sqlmux/internal/config"
	"sqlmux/internal/db"
	"sqlmux/internal/ui"
)

// withSessions is sized with a second session, report, of two windows, as
// gotSessionDialed would make one.
func withSessions() (*App, *Session) {
	a := sized(160, 45, "nerd")
	c := readyConn("doraemon")
	s := newSession("report", []*Conn{c}, c, a.freeConsole())
	s.Windows = append(s.Windows, &Window{Name: "logs"})
	a.sessions = append(a.sessions, s)
	return a, s
}

// The session list (§7.8): open on the session attached; l shows a
// session's windows, h hides them; ↵ attaches, on a window switching to
// it too; a click on a row is ↵ on it; the status bar's session block and
// the palette's $ scope get there as well (§12).
func TestSessionList(t *testing.T) {
	a, report := withSessions()
	doraemon := a.sess
	click(a, find(t, a, ui.Target{Kind: ui.KindButton, Action: "session.list"}).Min)
	if a.sessList == nil || a.sessList.sel != 0 || a.mode().String() != "command" {
		t.Fatalf("SPC s: %+v", a.sessList)
	}
	feed(t, a, "jl")
	golden.RequireEqual(t, a.render().String())
	feed(t, a, "jjh")
	if a.sessList.sel != 1 || len(a.sessionRows()) != 2 {
		t.Fatalf("h on a window: row %d of %d", a.sessList.sel, len(a.sessionRows()))
	}
	feed(t, a, "lj<CR>")
	if a.sess != report || report.Active != 0 || a.sessList != nil {
		t.Fatalf("↵ on 0: data: session %s window %d", a.sess.Name, report.Active)
	}
	feed(t, a, "<Space>s")
	click(a, find(t, a, ui.Target{Kind: ui.KindRow, I: 0}).Min)
	if a.sess != doraemon || a.sessList != nil {
		t.Errorf("a click on doraemon: %s", a.sess.Name)
	}
	if feed(t, a, "<C-p>$rep<CR>"); a.sess != report || a.palette != nil {
		t.Errorf("$rep: %s", a.sess.Name)
	}
}

// $ renames in place: nothing typed, or another session's name, keeps the
// name (§7.8).
func TestSessionRename(t *testing.T) {
	a, report := withSessions()
	feed(t, a, "<Space>sj$")
	if a.mode().String() != "insert" {
		t.Fatalf("mode %v", a.mode())
	}
	feed(t, a, "<C-u>doraemon<CR>")
	if report.Name != "report" || a.toast != "已有同名 session" {
		t.Errorf("a name taken: %s, toast %q", report.Name, a.toast)
	}
	feed(t, a, "$<C-u> <CR>$<C-u>logs<Esc>")
	if report.Name != "report" || a.sessList.rename != nil {
		t.Errorf("blank, then esc: %s", report.Name)
	}
	if feed(t, a, "$<BS><BS>rt2<CR>"); report.Name != "report2" {
		t.Errorf("renamed: %s", report.Name)
	}
}

// x closes a session: asked first with changes or a transaction in it;
// the one attached gives way to the one before it; the list stays; the
// last one is quitting. A window's row takes no x (§7.8).
func TestSessionKill(t *testing.T) {
	a, report := withSessions()
	doraemon := a.sess
	a.attach(report)
	report.Conns[0].tx = 'T'
	feed(t, a, "<Space>sjlj")
	if feed(t, a, "x"); len(a.sessions) != 2 || a.confirm != nil {
		t.Fatal("x on a window's row")
	}
	feed(t, a, "kx")
	if a.confirm == nil || a.confirm.text != "有未提交的事务，关闭会回滚。" {
		t.Fatalf("confirm %+v", a.confirm)
	}
	if feed(t, a, "y"); len(a.sessions) != 1 || a.sess != doraemon || a.sessList == nil || a.sessList.sel != 0 {
		t.Fatalf("killed: %d left, on %s, list %+v", len(a.sessions), a.sess.Name, a.sessList)
	}
	tab := loadOrders(t, a, 3)
	tab.edits = map[editKey]edit{{"1", "note"}: {val: db.Val{S: "x"}}}
	feed(t, a, "<Space>sx")
	if a.confirm == nil || !strings.Contains(a.confirm.text, "退出会丢弃") {
		t.Errorf("the last one quits: %+v", a.confirm)
	}
}

// n makes a session once its connection is connected; not connecting, it
// makes none (§7.8「新建 session」).
func TestNewSessionFails(t *testing.T) {
	a := sized(160, 45, "nerd")
	feed(t, a, "<Space>s")
	_, cmd := a.Update(teaKey("n"))
	if cmd == nil || a.sessList != nil || a.toast != "正在连接 doraemon…" {
		t.Fatalf("n: list %v, toast %q", a.sessList, a.toast)
	}
	if a.Update(sessionDialed{specs: []config.Connection{a.sess.Conns[0].Spec}, err: errors.New("refused")}); len(a.sessions) != 1 || a.toast != "doraemon：refused" {
		t.Errorf("failed: %d sessions, toast %q", len(a.sessions), a.toast)
	}
}

// conn.new opens the form; ↵ checks the connection as connections.toml is
// read, the field at fault getting the keys; one that checks goes at the
// end of the file and into every session's tree, not connected (§7.8).
func TestConnForm(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	a, report := withSessions()
	feed(t, a, "<C-p>conn.new<CR><Tab><Space><Tab>postgres://localhost/x<Tab>printf pw<Tab> <CR>")
	if a.form == nil || a.form.sel != 0 || a.form.err != "缺少 name" || a.form.fields[4].On != 1 {
		t.Fatalf("no name: %+v", a.form)
	}
	feed(t, a, "doraemon<CR>")
	golden.RequireEqual(t, a.render().String())
	feed(t, a, "<C-u>play<CR>")
	if a.form != nil || len(a.sess.Conns) != 2 || len(report.Conns) != 2 || a.sess.Conns[1].State != connIdle {
		t.Fatalf("saved: form %+v, conns %d %d", a.form, len(a.sess.Conns), len(report.Conns))
	}
	data, _ := os.ReadFile(filepath.Join(config.Dir(), "connections.toml"))
	if string(data) != "[[connection]]\nname = \"play\"\nengine = \"postgres\"\ndsn = \"postgres://localhost/x\"\npassword_cmd = \"printf pw\"\nread_only = true\n" {
		t.Errorf("file:\n%s", data)
	}
}
