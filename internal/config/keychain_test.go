package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeTool puts a script called name first on PATH (the real keychain is
// never touched); it sees the directory of its notes as $FAKE.
func fakeTool(t *testing.T, name, script string) (notes string) {
	t.Helper()
	bin, notes := t.TempDir(), t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, name), []byte("#!/bin/sh\n"+script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+":/usr/bin:/bin")
	t.Setenv("FAKE", notes)
	return notes
}

// The password goes in with the tool of the system, and the password_cmd
// that comes back reads it out, a name with a quote and a space included
// (§13).
func TestStorePassword(t *testing.T) {
	const name, pw = "my 'db'", `p@ss w'rd $x`
	for _, c := range []struct{ os, tool, script, wantArgs string }{
		{"darwin", "security", `echo "$@" >"$FAKE/args"
case $1 in
add-generic-password) while [ $# -gt 0 ]; do [ "$1" = -w ] && printf %s "$2" >"$FAKE/pw"; shift; done ;;
find-generic-password) cat "$FAKE/pw" ;;
esac
`, "add-generic-password -U -a sqlmux -s sqlmux-my 'db' -w "},
		{"linux", "secret-tool", `echo "$@" >"$FAKE/args"
case $1 in
store) cat >"$FAKE/pw" ;;
lookup) cat "$FAKE/pw" ;;
esac
`, "store --label sqlmux-my 'db' sqlmux my 'db'"},
	} {
		hostOS = c.os
		notes := fakeTool(t, c.tool, c.script)
		cmd, stored, err := StorePassword(name, pw)
		if err != nil || !stored {
			t.Fatalf("%s: stored %v, err %v", c.os, stored, err)
		}
		if args, _ := os.ReadFile(filepath.Join(notes, "args")); !strings.HasPrefix(string(args), c.wantArgs) {
			t.Errorf("%s: the tool was called with %q", c.os, args)
		}
		if got, err := (Connection{PasswordCmd: cmd}).Secret(); err != nil || got != pw {
			t.Errorf("%s: %q reads %q, %v", c.os, cmd, got, err)
		}
	}
	hostOS = "linux"
}

// No tool is no keychain, not an error; one that fails is the error, its
// first line of complaint said.
func TestStorePasswordNoToolOrFailing(t *testing.T) {
	t.Cleanup(func() { hostOS = "linux" })
	for _, goos := range []string{"darwin", "linux"} {
		hostOS = goos
		t.Setenv("PATH", t.TempDir())
		if cmd, stored, err := StorePassword("x", "pw"); cmd != "" || stored || err != nil {
			t.Errorf("%s with no tool: %q %v %v", goos, cmd, stored, err)
		}
	}
	hostOS = "darwin"
	fakeTool(t, "security", "echo 'keychain is locked' >&2\necho second >&2\nexit 1\n")
	if _, stored, err := StorePassword("x", "pw"); stored || err == nil || err.Error() != "security: keychain is locked" {
		t.Errorf("failing: %v %v", stored, err)
	}
}

// ${NAME} in dsn and password is the variable's value; $NAME is not
// touched; one not set, or empty, names itself (§13).
func TestExpanded(t *testing.T) {
	t.Setenv("PGURL", "postgres://u@h/db")
	t.Setenv("PGPW", "s3cret")
	t.Setenv("EMPTY", "")
	c, err := Connection{Name: "a", DSN: "${PGURL}?x=${PGPW}", Password: "pre${PGPW}$PGPW$"}.Expanded()
	if err != nil || c.DSN != "postgres://u@h/db?x=s3cret" || c.Password != "pres3cret$PGPW$" {
		t.Errorf("expanded: %+v %v", c, err)
	}
	for _, in := range []Connection{{DSN: "${NOT_SET_ANYWHERE}"}, {DSN: "x", Password: "${EMPTY}"}} {
		if _, err := in.Expanded(); err == nil || !strings.Contains(err.Error(), "${") || !strings.Contains(err.Error(), "这个环境变量没有设置") {
			t.Errorf("%+v: %v", in, err)
		}
	}
	if _, err := (Connection{DSN: "${NOT_SET_ANYWHERE}"}).Expanded(); err == nil || !strings.HasPrefix(err.Error(), "${NOT_SET_ANYWHERE}") {
		t.Errorf("names the variable: %v", err)
	}
	for _, src := range []Connection{{PasswordCmd: "x"}, {PasswordEnv: "X"}} { // password is not the source: left alone
		src.DSN, src.Password = "x", "${NOT_SET_ANYWHERE}"
		if c, err := src.Expanded(); err != nil || c.Password != "${NOT_SET_ANYWHERE}" {
			t.Errorf("%+v: %q %v", src, c.Password, err)
		}
	}
	if c, err := (Connection{DSN: "postgres://u:p$w@h/db"}).Expanded(); err != nil || c.DSN != "postgres://u:p$w@h/db" {
		t.Errorf("a $ alone: %+v %v", c, err)
	}
}
