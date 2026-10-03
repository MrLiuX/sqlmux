package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// connectionsFile writes connections.toml under a fresh XDG_CONFIG_HOME.
func connectionsFile(t *testing.T, body string, perm os.FileMode) string {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	path := filepath.Join(Dir(), "connections.toml")
	if err := os.MkdirAll(Dir(), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), perm); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, perm); err != nil { // past the umask
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
	connectionsFile(t, twoConns, 0o600)
	for name, want := range map[string]int{"": 0, "b": 1} {
		cs, start, warn, err := LoadConnections(name)
		if err != nil || len(cs) != 2 || cs[1].Name != "b" || start != want || warn != "" {
			t.Errorf("LoadConnections(%q) = %d of %d %q %v, want %d", name, start, len(cs), warn, err, want)
		}
	}
	_, _, _, err := LoadConnections("x")
	if err == nil || !strings.Contains(err.Error(), `没有名为 "x" 的连接，已有：a, b`) {
		t.Errorf("an unknown name: %v", err)
	}
}

// A plain password in a file group or others can read warns (§13).
func TestLoadConnectionWarnsOnReadablePassword(t *testing.T) {
	connectionsFile(t, twoConns, 0o644)
	if _, _, warn, _ := LoadConnections("a"); !strings.Contains(warn, "明文密码") {
		t.Errorf("0644 with a password: warning %q", warn)
	}
	connectionsFile(t, strings.ReplaceAll(twoConns, `password = "pw"`, ""), 0o644)
	if _, _, warn, _ := LoadConnections("a"); warn != "" {
		t.Errorf("0644 without a password: warning %q", warn)
	}
}

func TestLoadConnectionErrors(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	path := filepath.Join(Dir(), "connections.toml")
	if _, _, _, err := LoadConnections(""); err == nil || !strings.Contains(err.Error(), path) {
		t.Errorf("no file: %v, want the path %s", err, path)
	}
	for body, want := range map[string]string{
		"":                               "里面没有 [[connection]]",
		"[[connection]]\nname = \"a\"\n": "第 1 个连接：缺少 engine",
		"[[connection]]\nengine = \"postgres\"\n":                                       "第 1 个连接：缺少 name",
		"[[connection]]\nname = \"a\"\nengine = \"sqlite\"\ndsn = \"x\"\n":              `第 1 个连接：engine = "sqlite"：只能是 postgres 或 mysql`,
		"[[connection]]\nname = \"a/b\"\nengine = \"postgres\"\ndsn = \"x\"\n":          `第 1 个连接：name = "a/b"：不能含 / 或 \，也不能以 . 开头`,
		"[[connection]]\nname = 'a\\b'\nengine = \"postgres\"\ndsn = \"x\"\n":           `第 1 个连接：name = "a\\b"：不能含 / 或 \，也不能以 . 开头`,
		"[[connection]]\nname = \"..\"\nengine = \"postgres\"\ndsn = \"x\"\n":           `第 1 个连接：name = ".."：不能含 / 或 \，也不能以 . 开头`,
		twoConns + "[[connection]]\nname = \"a\"\nengine = \"postgres\"\ndsn = \"x\"\n": `第 3 个连接：name = "a" 重复了`,
	} {
		path := connectionsFile(t, body, 0o600)
		if _, _, _, err := LoadConnections(""); err == nil || !strings.Contains(err.Error(), path+": "+want) {
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
	path := connectionsFile(t, "# mine\n[[connection]]\nname = \"a\" # the first\nengine = \"postgres\"\ndsn = \"x\"", 0o600)
	if err := AppendConnection(Connection{Name: "b", Engine: "postgres", DSN: "postgres://localhost/b", ReadOnly: true}); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	want := "# mine\n[[connection]]\nname = \"a\" # the first\nengine = \"postgres\"\ndsn = \"x\"\n\n[[connection]]\nname = \"b\"\nengine = \"postgres\"\ndsn = \"postgres://localhost/b\"\nread_only = true\n"
	if string(data) != want {
		t.Errorf("file:\n%s", data)
	}
	if cs, _, _, err := LoadConnections("b"); err != nil || len(cs) != 2 || !cs[1].ReadOnly {
		t.Errorf("loaded back: %+v %v", cs, err)
	}
	os.Remove(path)
	if err := AppendConnection(Connection{Name: "c", Engine: "postgres", DSN: "y"}); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat(path); err != nil || info.Mode().Perm() != 0o600 {
		t.Errorf("made: %v %v", info, err)
	}
}
