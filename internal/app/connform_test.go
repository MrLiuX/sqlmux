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
	feed(t, a, "<C-p>conn.new<CR>play<Tab><Tab>postgres://u@h/db<Tab>secret")
	if view := a.render().String(); strings.Contains(view, "secret") || !strings.Contains(view, "******") {
		t.Errorf("the password is on screen:\n%s", view)
	}
	golden.RequireEqual(t, a.render().String())
	feed(t, a, "<CR>")
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
	feed(t, a, "<C-p>conn.new<CR>play<Tab><Tab>postgres://u@h/db<Tab>secret<CR>")
	info, err := os.Stat(filepath.Join(config.Dir(), "connections.toml"))
	if err != nil || a.form != nil || !strings.Contains(connections(), `password = "secret"`) || info.Mode().Perm() != 0o600 || a.toast != "没有钥匙串工具，密码以明文存进了 connections.toml" {
		t.Errorf("no tool: form %v, err %v, toast %q, file:\n%s", a.form, err, a.toast, connections())
	}

	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	keychain(t, "locked")
	a, _ = withSessions()
	feed(t, a, "<C-p>conn.new<CR>play<Tab><Tab>postgres://u@h/db<Tab>secret<CR>")
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
	feed(t, a, "<C-p>conn.new<CR>play<Tab><Tab>postgres://u@h/db<Tab>secret<Tab>pass show x<CR>")
	if a.form == nil || a.form.sel != fieldPassword || a.form.err != "password 和 password_cmd 填一个就够了" || connections() != "" {
		t.Errorf("two sources: form %+v, file:\n%s", a.form, connections())
	}
}
