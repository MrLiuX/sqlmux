package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kackerx/sqlmux/internal/i18n"
)

// connections_test.go and keychain_test.go read the Chinese texts: the
// language they run in is not the default, English (F7.6).
func init() { i18n.Set("zh") }

// connectionsFile writes connections.toml under a fresh XDG_CONFIG_HOME.
func connectionsFile(t *testing.T, body string) string {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	path := filepath.Join(Dir(), "connections.toml")
	if err := os.MkdirAll(Dir(), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

const twoConns = `
[[connection]]
name = "a"
engine = "postgres"
dsn = "postgres://localhost/a"

[[connection]]
name = "b"
engine = "postgres"
dsn = "postgres://localhost/b"
password = "pw"
`

// Every connection comes, the one named the start, else the first (§5).
func TestLoadConnectionsByName(t *testing.T) {
	connectionsFile(t, twoConns)
	for name, want := range map[string]int{"": 0, "b": 1} {
		cs, start, err := LoadConnections(name)
		if err != nil || len(cs) != 2 || cs[1].Name != "b" || start != want {
			t.Errorf("LoadConnections(%q) = %d of %d %v, want %d", name, start, len(cs), err, want)
		}
	}
	_, _, err := LoadConnections("x")
	if err == nil || !strings.Contains(err.Error(), `没有名为 "x" 的连接，已有：a, b`) {
		t.Errorf("an unknown name: %v", err)
	}
}

func TestLoadConnectionErrors(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	path := filepath.Join(Dir(), "connections.toml")
	if _, _, err := LoadConnections(""); err == nil || !strings.Contains(err.Error(), path) {
		t.Errorf("no file: %v, want the path %s", err, path)
	}
	for body, want := range map[string]string{
		"":                               "里面没有 [[connection]]",
		"[[connection]]\nname = \"a\"\n": "第 1 个连接：缺少 engine",
		"[[connection]]\nengine = \"postgres\"\n":                                       "第 1 个连接：缺少 name",
		"[[connection]]\nname = \"a\"\nengine = \"oracle\"\ndsn = \"x\"\n":              `第 1 个连接：engine = "oracle"：只能是 postgres、mysql 或 sqlite`,
		"[[connection]]\nname = \"a/b\"\nengine = \"postgres\"\ndsn = \"x\"\n":          `第 1 个连接：name = "a/b"：不能含 / 或 \，也不能以 . 开头`,
		"[[connection]]\nname = 'a\\b'\nengine = \"postgres\"\ndsn = \"x\"\n":           `第 1 个连接：name = "a\\b"：不能含 / 或 \，也不能以 . 开头`,
		"[[connection]]\nname = \"..\"\nengine = \"postgres\"\ndsn = \"x\"\n":           `第 1 个连接：name = ".."：不能含 / 或 \，也不能以 . 开头`,
		twoConns + "[[connection]]\nname = \"a\"\nengine = \"postgres\"\ndsn = \"x\"\n": `第 3 个连接：name = "a" 重复了`,
	} {
		path := connectionsFile(t, body)
		if _, _, err := LoadConnections(""); err == nil || !strings.Contains(err.Error(), path+": "+want) {
			t.Errorf("%q: %v, want %q", body, err, want)
		}
	}
}

// password_cmd wins over password_env, which wins over password (§13).
func TestSecret(t *testing.T) {
	t.Setenv("SQLMUX_TEST_SECRET", "from-env")
	for _, c := range []struct {
		conn Connection
		want string
	}{
		{Connection{PasswordCmd: "printf 'from-cmd\\n'", PasswordEnv: "SQLMUX_TEST_SECRET", Password: "plain"}, "from-cmd"},
		{Connection{PasswordEnv: "SQLMUX_TEST_SECRET", Password: "plain"}, "from-env"},
		{Connection{Password: "plain"}, "plain"},
		{Connection{}, ""},
	} {
		if got, err := c.conn.Secret(); got != c.want || err != nil {
			t.Errorf("%+v: %q %v, want %q", c.conn, got, err, c.want)
		}
	}
	if _, err := (Connection{PasswordCmd: "echo nope >&2; exit 3"}).Secret(); err == nil || !strings.Contains(err.Error(), "nope") {
		t.Errorf("a failing password_cmd: %v, want its stderr", err)
	}
	if _, err := (Connection{PasswordEnv: "SQLMUX_TEST_UNSET"}).Secret(); err == nil {
		t.Error("an unset password_env: no error")
	}
}

// A connection made in the app goes at the end of connections.toml, as
// text after the user's, comments and all; none there, the file is made
// the user's only (§14).
func TestAppendConnection(t *testing.T) {
	path := connectionsFile(t, "# mine\n[[connection]]\nname = \"a\" # the first\nengine = \"postgres\"\ndsn = \"x\"")
	if err := AppendConnection(Connection{Name: "b", Engine: "postgres", DSN: "postgres://localhost/b", ReadOnly: true}); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	want := "# mine\n[[connection]]\nname = \"a\" # the first\nengine = \"postgres\"\ndsn = \"x\"\n\n[[connection]]\nname = \"b\"\nengine = \"postgres\"\ndsn = \"postgres://localhost/b\"\nread_only = true\n"
	if string(data) != want {
		t.Errorf("file:\n%s", data)
	}
	if cs, _, err := LoadConnections("b"); err != nil || len(cs) != 2 || !cs[1].ReadOnly {
		t.Errorf("loaded back: %+v %v", cs, err)
	}
	os.Remove(path)
	if err := AppendConnection(Connection{Name: "c", Engine: "postgres", DSN: "y"}); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat(path); err != nil || info.Mode().Perm() != 0o600 {
		t.Errorf("made: %v %v", info, err)
	}
	// connections in an inline array: a [[connection]] after it would break
	// the file, the next start exiting; it stays as it is
	inline := "connection = [{ name = \"a\", engine = \"postgres\", dsn = \"x\" }]\n"
	path = connectionsFile(t, inline)
	if err := AppendConnection(Connection{Name: "b", Engine: "postgres", DSN: "y"}); err == nil {
		t.Error("appended to an inline array")
	}
	if data, _ := os.ReadFile(path); string(data) != inline {
		t.Errorf("written: %s", data)
	}
}

// No file and no connection in it are the one case the program starts
// without a connection (F7.4); a file that does not read is an error.
func TestNoConnections(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	for _, file := range []struct {
		text string
		none bool
	}{{"", true}, {"# empty\n", true}, {"[[connection]\n", false}, {"[[connection]]\nname = \"x\"\n", false}} {
		if file.text != "" {
			if err := os.MkdirAll(filepath.Join(dir, "sqlmux"), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, "sqlmux", "connections.toml"), []byte(file.text), 0o600); err != nil {
				t.Fatal(err)
			}
		}
		_, _, err := LoadConnections("")
		if err == nil || NoConnections(err) != file.none {
			t.Errorf("%q: %v", file.text, err)
		}
	}
}

// A file that cannot be appended to is the error of CheckAppend, one that is
// not there yet is not (F8.16).
func TestCheckAppendUnwritable(t *testing.T) {
	c := Connection{Name: "n", Engine: "postgres", DSN: "postgres://h/n"}
	path := connectionsFile(t, "[[connection]]\nname = \"a\"\nengine = \"postgres\"\ndsn = \"postgres://h/a\"\n")
	if err := CheckAppend(c); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o444); err != nil {
		t.Fatal(err)
	}
	if os.Geteuid() != 0 && CheckAppend(c) == nil {
		t.Error("a read-only file")
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := CheckAppend(c); err != nil {
		t.Errorf("no file yet: %v", err)
	}
	if os.Geteuid() == 0 {
		return
	}
	if err := os.Chmod(filepath.Dir(path), 0o500); err != nil { // no file yet, and none can be made
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(filepath.Dir(path), 0o700) })
	if CheckAppend(c) == nil {
		t.Error("a directory no file can be made in")
	}
}

// An ssh is a destination for the local ssh: one it would take for an option
// or for two words is no connection (F15.2).
func TestCheckSSH(t *testing.T) {
	for ssh, bad := range map[string]bool{"me@jump": false, "jump": false, "": false, "-oProxyCommand=x": true, "-J x": true, "me@jump x": true, "me@jump\tx": true} {
		c := Connection{Name: "a", Engine: "postgres", DSN: "postgres://h/d", SSH: ssh}
		if field, _ := c.Check(nil); (field == "ssh") != bad || field != "" && field != "ssh" {
			t.Errorf("%q: field %q", ssh, field)
		}
	}
}
