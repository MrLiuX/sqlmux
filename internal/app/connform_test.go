package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/x/exp/golden"

	"sqlmux/internal/config"
)

// keychain puts a fake security and secret-tool first on PATH: what they
// are given they keep in $FAKE/pw, or they refuse with "locked". PATH is
// bare with none: the tests never reach the real keychain.
func keychain(t *testing.T, mode string) (pw string) {
	t.Helper()
	bin, notes := t.TempDir(), t.TempDir()
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
	a, _ := withSessions()
	feed(t, a, "<C-p>conn.new<CR>play<S-Tab>") // round to the last row that shows: the fold
	if a.form.sel != fieldAdvanced {
		t.Fatalf("closed: S-Tab from name is on %d", a.form.sel)
	}
	feed(t, a, "<Space><Tab>mysql://u@h/d<Tab>printf pw")
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
	if a.form != nil || connections() != "[[connection]]\nname = \"play\"\nengine = \"postgres\"\ndsn = \"mysql://u@h/d\"\npassword_cmd = \"printf pw\"\n" {
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
		seen[c.key()] = true
	}
	if len(seen) != 3 {
		t.Errorf("consoles %v", seen)
	}
}
