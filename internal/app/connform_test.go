package app

import (
	"image/color"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/exp/golden"

	"sqlmux/internal/config"
	"sqlmux/internal/ui"
)

// keychain puts a fake security and secret-tool first on PATH: what they
// are given they keep in $FAKE/pw, or they refuse with "locked". PATH is
// bare with none: the tests never reach the real keychain.
func keychain(t *testing.T, mode string) (pw string) {
	t.Helper()
	bin, notes := t.TempDir(), t.TempDir()
	if cat, err := exec.LookPath("cat"); err == nil && mode != "none" { // the fake secret-tool reads its stdin with it
		if err := os.Symlink(cat, filepath.Join(bin, "cat")); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", bin)
	t.Setenv("FAKE", notes)
	if mode != "none" {
		script := "#!/bin/bash\n"
		if mode == "locked" {
			script += "echo locked >&2; exit 1\n"
		} else {
			script += `case $1 in
add-generic-password) while [ $# -gt 0 ]; do [ "$1" = -w ] && printf %s "$2" >"$FAKE/pw"; shift; done ;;
store) cat >"$FAKE/pw" ;;
esac
`
		}
		for _, name := range []string{"security", "secret-tool"} {
			if err := os.WriteFile(filepath.Join(bin, name), []byte(script), 0o755); err != nil {
				t.Fatal(err)
			}
		}
	}
	return filepath.Join(notes, "pw")
}

func connections() string {
	data, _ := os.ReadFile(filepath.Join(config.Dir(), "connections.toml"))
	return string(data)
}

// The password field draws a * for each character and the password goes
// into the keychain: connections.toml holds the command that reads it, not
// the password (§13, F7.3).
func TestConnFormPassword(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	pw := keychain(t, "ok")
	a, _ := withSessions()
	feed(t, a, "<C-p>conn.new<CR>play<Tab><Tab>h<Tab><Tab>u<Tab>secret")
	if view := a.render().String(); strings.Contains(view, "secret") || !strings.Contains(view, "******") {
		t.Errorf("the password is on screen:\n%s", view)
	}
	golden.RequireEqual(t, a.render().String())
	feed(t, a, "<Tab>d<CR>")
	got, _ := os.ReadFile(pw)
	file := connections()
	if a.form != nil || string(got) != "secret" || strings.Contains(file, "secret") || !strings.Contains(file, "password_cmd = ") || len(a.sess.Conns) != 2 {
		t.Errorf("saved: form %v, keychain %q, conns %d, file:\n%s", a.form, got, len(a.sess.Conns), file)
	}
}

// With no keychain tool the password goes into connections.toml, 0600, and
// the toast says so; one that fails keeps the form, its complaint on the
// error line, nothing saved; a password beside a password_cmd is asked
// about; no password stores nothing.
func TestConnFormPasswordElsewhere(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	keychain(t, "none")
	a, _ := withSessions()
	feed(t, a, "<C-p>conn.new<CR>play<Tab><Tab>h<Tab><Tab>u<Tab>secret<Tab>d<CR>")
	info, err := os.Stat(filepath.Join(config.Dir(), "connections.toml"))
	if err != nil || a.form != nil || !strings.Contains(connections(), `password = "secret"`) || info.Mode().Perm() != 0o600 || a.toast != "没有钥匙串工具，密码以明文存进了 connections.toml" {
		t.Errorf("no tool: form %v, err %v, toast %q, file:\n%s", a.form, err, a.toast, connections())
	}

	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	keychain(t, "locked")
	a, _ = withSessions()
	feed(t, a, "<C-p>conn.new<CR>play<Tab><Tab>h<Tab><Tab>u<Tab>secret<Tab>d<CR>")
	if a.form == nil || !strings.HasSuffix(a.form.err, ": locked") || connections() != "" || len(a.sess.Conns) != 1 {
		t.Errorf("a keychain that fails: form %v, file:\n%s", a.form, connections())
	}
	feed(t, a, "<C-u><CR>") // cleared: no password, nothing to store
	if a.form != nil || strings.Contains(connections(), "password") || len(a.sess.Conns) != 2 {
		t.Errorf("cleared: form %v, file:\n%s", a.form, connections())
	}

	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	keychain(t, "ok")
	a, _ = withSessions()
	feed(t, a, "<C-p>conn.new<CR>play<Tab><Tab>h<Tab><Tab>u<Tab>secret<Tab>d<Tab><Tab> <Tab><Tab>pass show x<CR>")
	if a.form == nil || a.form.sel != fieldPassword || a.form.err != "password 和 password_cmd 填一个就够了" || connections() != "" {
		t.Errorf("two sources: form %+v, file:\n%s", a.form, connections())
	}
}

// The engine sets the port while it is not typed in; the fields host, user
// and database are asked for, the port is a number; the form says which one
// and gives it the keys (§7.8).
func TestConnFormFields(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	keychain(t, "none")
	a, _ := withSessions()
	port := func() string { return a.form.fields[fieldPort].Input.Text }
	feed(t, a, "<C-p>conn.new<CR>play<Tab><Space>")
	if port() != "3306" {
		t.Errorf("mysql: port %q", port())
	}
	feed(t, a, "<Left>")
	if port() != "5432" {
		t.Errorf("postgres again: port %q", port())
	}
	feed(t, a, "<Tab><Tab>1<S-Tab><S-Tab><Right>") // a typed port stays: host, port ("54321"), back to the engine
	if a.form.sel != fieldEngine || port() != "54321" {
		t.Errorf("typed: on %d, port %q", a.form.sel, port())
	}
	for _, c := range []struct {
		keys, err string
		sel       int
	}{
		{"<CR>", "缺少 host", fieldHost},
		{"h<CR>", "缺少 user", fieldUser},
		{"u<CR>", "缺少 database", fieldDatabase},
	} {
		feed(t, a, c.keys)
		if a.form == nil || a.form.err != c.err || a.form.sel != c.sel || !a.form.fields[c.sel].Bad {
			t.Fatalf("%q: %+v", c.keys, a.form)
		}
	}
	feed(t, a, "d<S-Tab><S-Tab><S-Tab><C-u>99999<CR>")
	if a.form.err != "port 要是 1–65535 的数字" || a.form.sel != fieldPort {
		t.Fatalf("a port too big: %+v", a.form)
	}
}

// The 高级 fold holds dsn and password_cmd: closed, they are out of the way;
// open, a dsn typed there is the connection and host, port, user and
// database are dim and passed by Tab; password_cmd is its password.
func TestConnFormAdvanced(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	keychain(t, "none")
	a, _ := withSessions()
	feed(t, a, "<C-p>conn.new<CR>play<S-Tab>") // round to the last row that shows: the fold
	if a.form.sel != fieldAdvanced {
		t.Fatalf("closed: S-Tab from name is on %d", a.form.sel)
	}
	feed(t, a, "<Space><Tab>postgres://u@h/d<Tab>printf pw")
	if !a.form.dsnGiven() || !a.form.skipped(fieldHost) {
		t.Fatal("host is not dim with a dsn")
	}
	feed(t, a, "<Tab>") // round to name
	if a.form.sel != fieldName {
		t.Errorf("Tab round the end: on %d", a.form.sel)
	}
	feed(t, a, "<Tab>")
	if a.form.sel != fieldEngine {
		t.Errorf("name, then the engine: on %d", a.form.sel)
	}
	feed(t, a, "<Tab>")
	if a.form.sel != fieldPassword {
		t.Errorf("past the dim host, port and user: on %d", a.form.sel)
	}
	feed(t, a, "<S-Tab><S-Tab>")
	golden.RequireEqual(t, a.render().String())
	feed(t, a, "<CR>")
	if a.form != nil || connections() != "[[connection]]\nname = \"play\"\nengine = \"postgres\"\ndsn = \"postgres://u@h/d\"\npassword_cmd = \"printf pw\"\n" {
		t.Errorf("saved: form %v, file:\n%s", a.form, connections())
	}
}

func TestComposeDSN(t *testing.T) {
	for _, c := range []struct{ engine, host, port, user, db, want string }{
		{"postgres", "localhost", "5432", "u", "d", "postgres://u@localhost:5432/d?sslmode=prefer"},
		{"mysql", "db.example", "3306", "u", "d", "mysql://u@db.example:3306/d"},
		{"postgres", "::1", "5432", "p@t:x", "d b", "postgres://p%40t%3Ax@[::1]:5432/d%20b?sslmode=prefer"},
	} {
		if got := composeDSN(c.engine, c.host, c.port, c.user, c.db); got != c.want {
			t.Errorf("%+v: %q", c, got)
		}
	}
}

// A host with a port in it is told so, an IPv6 one may have its brackets;
// without a keychain tool a password holding ${ is not saved, dial would take
// it for a variable (§13).
func TestConnFormHostAndPlainPassword(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	keychain(t, "none")
	a, _ := withSessions()
	feed(t, a, "<C-p>conn.new<CR>play<Tab><Tab>db.internal:5433<Tab><Tab>u<Tab>pa${ss}<Tab>d<CR>")
	if a.form == nil || a.form.sel != fieldHost || a.form.err != "host 里不要带端口，端口在下面" {
		t.Fatalf("host with a port: %+v", a.form)
	}
	feed(t, a, "<C-u>[::1]<CR>")
	if a.form == nil || a.form.sel != fieldPassword || connections() != "" {
		t.Fatalf("a ${ in a plain password: %+v\n%s", a.form, connections())
	}
	feed(t, a, "<C-u>plain<CR>")
	if a.form != nil || !strings.Contains(connections(), `dsn = "postgres://u@[::1]:5432/d?sslmode=prefer"`) {
		t.Errorf("saved: %v\n%s", a.form, connections())
	}
}

// The first connection gives a console to every window made before it.
func TestNoConnectionFirstConsoles(t *testing.T) {
	keychain(t, "none")
	a := emptyApp(t)
	a.form = nil
	feed(t, a, "<Space>c<Space>c") // two more windows, the consoles none
	a.run("conn.new", 0)
	feed(t, a, "play<Tab><Tab>h<Tab><Tab>u<Tab><Tab>d<CR>")
	if len(a.sess.Windows) != 3 {
		t.Fatalf("%d windows", len(a.sess.Windows))
	}
	seen := map[string]bool{}
	for _, w := range a.sess.Windows {
		c := consoleOf(w.pane(2))
		if c == nil || c.conn != a.sess.start {
			t.Fatalf("a window without its console: %+v", c)
		}
		seen[c.name] = true
	}
	if len(seen) != 3 {
		t.Errorf("consoles %v", seen)
	}
}

// The form stays out of the keychain when the file would not take the
// connection: a name the file has since got is refused before its password
// is stored over the other's (F7.3).
func TestConnFormKeychainAfterCheck(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	pw := keychain(t, "ok")
	if err := os.MkdirAll(filepath.Join(dir, "sqlmux"), 0o700); err != nil {
		t.Fatal(err)
	}
	theirs := "[[connection]]\nname = \"play\"\nengine = \"postgres\"\ndsn = \"postgres://x@h/d\"\n"
	if err := os.WriteFile(filepath.Join(dir, "sqlmux", "connections.toml"), []byte(theirs), 0o600); err != nil {
		t.Fatal(err)
	}
	a, _ := withSessions() // the tree never had play
	feed(t, a, "<C-p>conn.new<CR>play<Tab><Tab>h<Tab><Tab>u<Tab>secret<Tab>d<CR>")
	if _, err := os.Stat(pw); err == nil || a.form == nil || !strings.Contains(a.form.err, "play") || connections() != theirs {
		t.Errorf("keychain written (%v) or file changed, form %v:\n%s", err, a.form, connections())
	}
}

// A plain password changes no permissions of a file the user has: 0644 stays
// (§13); a file the form makes is 0600.
func TestConnFormPlainPasswordMode(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	keychain(t, "none")
	path := filepath.Join(dir, "sqlmux", "connections.toml")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("[[connection]]\nname = \"doraemon\"\nengine = \"postgres\"\ndsn = \"postgres://x@h/d\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}
	a, _ := withSessions()
	feed(t, a, "<C-p>conn.new<CR>play<Tab><Tab>h<Tab><Tab>u<Tab>secret<Tab>d<CR>")
	if info, err := os.Stat(path); err != nil || a.form != nil || info.Mode().Perm() != 0o644 {
		t.Errorf("mode %v, %v, form %v", info.Mode(), err, a.form)
	}
}

// The port is the engine's default while it is empty or still that: a port
// typed in stays, one put back or cleared follows again; +5432 is saved as
// 5432.
func TestConnFormPortFollows(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	keychain(t, "none")
	a, _ := withSessions()
	port := func() string { return a.form.fields[fieldPort].Input.Text }
	feed(t, a, "<C-p>conn.new<CR><Tab>") // on the engine
	for _, c := range []struct{ keys, want, what string }{
		{"<Space>", "3306", "mysql's"},
		{"<Tab><Tab><C-u>3307<S-Tab><S-Tab><Left>", "3307", "a typed port stays"},
		{"<Tab><Tab><C-u>5432<S-Tab><S-Tab><Space>", "3306", "postgres's put back follows"},
		{"<Tab><Tab><C-u><S-Tab><S-Tab><Left>", "5432", "a cleared one follows"},
	} {
		if feed(t, a, c.keys); port() != c.want {
			t.Fatalf("%s: %q", c.what, port())
		}
	}
	feed(t, a, "<Tab>h<Tab><C-u>+5432<Tab>u<Tab><Tab>d<S-Tab><S-Tab><S-Tab><S-Tab><S-Tab><S-Tab>play<CR>")
	if a.form != nil || !strings.Contains(connections(), `dsn = "postgres://u@h:5432/d?sslmode=prefer"`) {
		t.Errorf("form %v, file:\n%s", a.form, connections())
	}
}

// layout.open on a start with no connection says so, and does not open the
// list of layouts whose connections there are none of (F7.4).
func TestNoConnectionLayoutOpen(t *testing.T) {
	a := emptyApp(t)
	a.form = nil
	if err := os.MkdirAll(config.StateDir(), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(config.LastPath(), []byte("version = 1\n[[session]]\nname = \"s\"\nconnection = \"c\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	feed(t, a, "<C-p>layout.open<CR><CR>")
	if a.drop != nil || a.toast != "还没有连接，先新建一个" {
		t.Errorf("drop %v, toast %q", a.drop, a.toast)
	}
}

// colAt is the column of text in the screen row y.
func colAt(t *testing.T, f *ui.Frame, y int, text string) int {
	t.Helper()
	row := strings.Split(f.String(), "\n")[y]
	i := strings.Index(row, text)
	if i < 0 {
		t.Fatalf("no %q in row %d: %q", text, y, row)
	}
	return ui.Width(row[:i])
}

// The form's colors: the name of the field with the keys is fg, the others
// dim, the one that is wrong error; a field that takes no part has dim text;
// the segment that is on has the select background, the other the sep one.
func TestConnFormColors(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	keychain(t, "none")
	a, _ := withSessions()
	feed(t, a, "<C-p>conn.new<CR>play<Tab><Tab><Tab>")
	th := a.theme
	row := func(field int) int { return find(t, a, ui.Target{Kind: ui.KindRow, I: field}).Min.Y }
	f := a.render()
	name := func(field int, label string) color.Color {
		return f.Buf.CellAt(colAt(t, f, row(field), label), row(field)).Style.Fg
	}
	if name(fieldName, "name") != th.Dim || name(fieldPort, "port") != th.Fg || name(fieldHost, "host") != th.Dim {
		t.Errorf("names: name %v, port %v, host %v", name(fieldName, "name"), name(fieldPort, "port"), name(fieldHost, "host"))
	}
	seg := func(text string) color.Color {
		return f.Buf.CellAt(colAt(t, f, row(fieldEngine), text), row(fieldEngine)).Style.Bg
	}
	if seg("postgres") != th.Select || seg("mysql") != th.Sep {
		t.Errorf("segments: postgres %v, mysql %v", seg("postgres"), seg("mysql"))
	}
	feed(t, a, "<CR>") // host is empty: wrong
	f = a.render()
	if name(fieldHost, "host") != th.Error {
		t.Errorf("a field that is wrong: %v", name(fieldHost, "host"))
	}
	feed(t, a, "h<S-Tab><S-Tab><S-Tab><Space><Tab>postgres://u@h/d") // host typed, then round to the fold, open, a dsn
	f = a.render()
	if fg := f.Buf.CellAt(colAt(t, f, row(fieldHost), "h"), row(fieldHost)).Style.Fg; fg != th.Dim {
		t.Errorf("host with a dsn given: its text is %v, not dim", fg)
	}
}

// Clicks: a row that takes no part is not given the keys, a check and the
// fold turn over, a segment is picked by its own place, the buttons save and
// cancel, and the ＋ of a tree with connections opens the form (§7.8).
func TestConnFormClicks(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	keychain(t, "none")
	a, _ := withSessions()
	sb := a.sidebarRect()
	click(a, uv.Pos(sb.Max.X-3, sb.Min.Y+1)) // the ＋, by its place
	if a.form == nil {
		t.Fatal("the ＋ opens no form")
	}
	row := func(field int) uv.Position {
		return find(t, a, ui.Target{Kind: ui.KindRow, I: field}).Min.Add(uv.Pos(2, 0))
	}
	click(a, row(fieldReadOnly))
	if a.form.fields[fieldReadOnly].On != 1 || a.form.sel != fieldReadOnly {
		t.Errorf("read_only: %+v", a.form.fields[fieldReadOnly])
	}
	click(a, row(fieldAdvanced))
	if !a.form.advanced() {
		t.Error("the fold does not open")
	}
	feed(t, a, "<Tab>postgres://u@h/d")
	a.form.sel = fieldName
	click(a, row(fieldHost)) // dim now
	if a.form.sel != fieldName {
		t.Errorf("a dim row took the keys: %d", a.form.sel)
	}
	click(a, find(t, a, ui.Target{Kind: ui.KindSegment, Pane: fieldEngine, I: 1}).Min)
	if a.form.fields[fieldEngine].On != 1 || a.form.fields[fieldPort].Input.Text != "3306" {
		t.Errorf("mysql by its segment: %+v", a.form.fields[fieldEngine])
	}
	click(a, find(t, a, ui.Target{Kind: ui.KindSegment, Pane: fieldEngine, I: 1}).Min)
	if a.form.fields[fieldEngine].On != 1 {
		t.Error("a click on the segment that is on changes it")
	}
	click(a, find(t, a, ui.Target{Kind: ui.KindButton, Action: "form.save"}).Min)
	if a.form == nil || a.form.err != "缺少 name" {
		t.Errorf("save: %+v", a.form)
	}
	click(a, find(t, a, ui.Target{Kind: ui.KindButton, Action: "form.cancel"}).Min)
	if a.form != nil {
		t.Error("cancel leaves the form open")
	}
}

// What is typed in the fold is not taken when the fold is closed again.
func TestConnFormClosedFoldIgnored(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	keychain(t, "none")
	a, _ := withSessions()
	feed(t, a, "<C-p>conn.new<CR>play<Tab><Tab>h<Tab><Tab>u<Tab><Tab>d<Tab><Tab> ")
	feed(t, a, "<Tab>mysql://x@y/z<Tab>pass show x<S-Tab><S-Tab> ") // typed, then closed again
	if a.form.advanced() {
		t.Fatal("the fold did not close")
	}
	feed(t, a, "<CR>")
	if file := connections(); a.form != nil || strings.Contains(file, "mysql://x@y/z") || strings.Contains(file, "password_cmd") || !strings.Contains(file, "postgres://u@h:5432/d") {
		t.Errorf("form %v, file:\n%s", a.form, file)
	}
}
