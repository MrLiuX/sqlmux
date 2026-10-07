package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const editFile = `# my databases

# the one for play
[[connection]]
name = "play"        # not a seed
engine = "postgres"
dsn = "postgres://localhost/play?sslmode=disable"

# staging, read only
[[connection]]
name = "stage"
engine = "mysql"
dsn = "u:p@tcp(h:3306)/stage"
read_only = true

[[connection]]
name = "last"
engine = "postgres"
dsn = "postgres://localhost/last"`

const oneConn = "[[connection]]\nname = \"a\"\nengine = \"postgres\"\ndsn = \"postgres://h/a\"\n"

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func connA(dsn string, readOnly bool) Connection {
	return Connection{Name: "a", Engine: "postgres", DSN: dsn, ReadOnly: readOnly}
}

// An edit rewrites the lines of its block that changed, adds a new key after
// the block's last, takes a cleared one out, and leaves every other byte of
// the file and its permissions as they were; what comes back is the block as
// the file says it now (F8.16).
func TestUpdateConnection(t *testing.T) {
	path := connectionsFile(t, editFile)
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}
	was := Connection{Name: "stage", Engine: "mysql", DSN: "u:p@tcp(h:3306)/stage", ReadOnly: true}
	c := was
	c.DSN = "u:p@tcp(other:3306)/stage"
	got, err := UpdateConnection("stage", was, c)
	if err != nil || got != c {
		t.Fatalf("a dsn: %+v %v", got, err)
	}
	if got, want := readFile(t, path), strings.Replace(editFile, "tcp(h:3306)", "tcp(other:3306)", 1); got != want {
		t.Errorf("a dsn:\n%s", got)
	}
	was = c
	c.ReadOnly, c.PasswordCmd, c.Name = false, "pass show stage", "staging"
	if _, err := UpdateConnection("stage", was, c); err != nil {
		t.Fatal(err)
	}
	want := strings.Replace(editFile, "tcp(h:3306)", "tcp(other:3306)", 1)
	want = strings.Replace(want, `name = "stage"`, `name = "staging"`, 1)
	want = strings.Replace(want, "read_only = true\n", `password_cmd = "pass show stage"`+"\n", 1)
	if got := readFile(t, path); got != want {
		t.Errorf("a rename, a key added after the last, one cleared:\n%s", got)
	}
	last := Connection{Name: "last", Engine: "postgres", DSN: "postgres://localhost/last"}
	withRO := last
	withRO.ReadOnly = true
	if _, err := UpdateConnection("last", last, withRO); err != nil { // the last line has no break
		t.Fatal(err)
	}
	if got := readFile(t, path); !strings.HasSuffix(got, "dsn = \"postgres://localhost/last\"\nread_only = true\n") || !strings.HasPrefix(got, "# my databases\n\n# the one for play\n") {
		t.Errorf("the last block:\n%s", got)
	}
	if fi, err := os.Stat(path); err != nil || fi.Mode().Perm() != 0o644 {
		t.Errorf("permissions: %v %v", fi.Mode(), err)
	}
	if _, err := UpdateConnection("nobody", was, c); err == nil {
		t.Error("a connection that is not there")
	}
}

// Only what the caller changed is written, in the file as it is now: a key
// the user changed in an editor meanwhile stays, one they added too (F8.16).
func TestUpdateConnectionKeepsEditsMadeMeanwhile(t *testing.T) {
	path := connectionsFile(t, oneConn+"read_only = true\n")
	was := connA("postgres://h/a", true)
	disk := oneConn + "password_env = \"PW\"\nread_only = true\n"
	disk = strings.Replace(disk, "h/a", "h/edited", 1)
	if err := os.WriteFile(path, []byte(disk), 0o600); err != nil {
		t.Fatal(err)
	}
	c := was
	c.ReadOnly = false
	got, err := UpdateConnection("a", was, c)
	if want := (Connection{Name: "a", Engine: "postgres", DSN: "postgres://h/edited", PasswordEnv: "PW"}); err != nil || got != want {
		t.Errorf("read back %+v %v, want %+v", got, err, want)
	}
	if got, want := readFile(t, path), strings.Replace(disk, "read_only = true\n", "", 1); got != want {
		t.Errorf("file:\n%s", got)
	}
}

// What the edit would write is read back and compared with what was asked:
// a key written in a way the lines cannot take (in quotes, in a value of
// several lines) is refused, the file untouched, while the ways that only
// look odd (dotted keys, inline tables, arrays, values of several lines
// left alone) are edited as any (F8.16).
func TestUpdateConnectionReadsBack(t *testing.T) {
	multi := "[[connection]]\nname = \"a\"\nengine = \"postgres\"\ndsn = \"\"\"\npostgres://h/a\"\"\"\n"
	for _, c := range []struct {
		label, body string
		to          Connection
		odd         bool
	}{
		{"no spaces", "[[connection]]\nname=\"a\"\nengine=\"postgres\"\ndsn=\"postgres://h/a\"\n", connA("postgres://h/b", false), false},
		{"wide spaces", "[[connection]]\nname   =   \"a\"\nengine = \"postgres\"\ndsn   =   \"postgres://h/a\"\n", connA("postgres://h/b", false), false},
		{"single quotes", "[[connection]]\nname = 'a'\nengine = 'postgres'\ndsn = 'postgres://h/a'\n", connA("postgres://h/b", false), false},
		{"a # in a password", oneConn + "password = \"a#b\"\n", Connection{Name: "a", Engine: "postgres", DSN: "postgres://h/b", Password: "a#b"}, false},
		{"a dotted key beside", oneConn + "options.sslmode = \"require\"\n", connA("postgres://h/b", false), false},
		{"an inline table beside", oneConn + "opts = { a = 1 }\n", connA("postgres://h/b", false), false},
		{"an array beside", oneConn + "m = [\n  [1, 2],\n]\n", connA("postgres://h/b", false), false},
		{"a key after a value of lines", multi, connA("postgres://h/a", true), false},
		{"a key after a string of lines", oneConn + "note = '''\nline one\n'''\n", connA("postgres://h/a", true), false},
		{"a key after an array of lines", oneConn + "tags = [\n  \"x\",\n]\n", connA("postgres://h/a", true), false},
		{"a quoted key cleared", oneConn + "\"read_only\" = true\n", connA("postgres://h/a", false), true},
		{"a quoted key changed", "[[connection]]\nname = \"a\"\nengine = \"postgres\"\n\"dsn\" = \"postgres://h/a\"\n", connA("postgres://h/b", false), true},
		{"a quoted name changed", "[[connection]]\n\"name\" = \"a\"\nengine = \"postgres\"\ndsn = \"postgres://h/a\"\n", Connection{Name: "z", Engine: "postgres", DSN: "postgres://h/a"}, true},
		{"a value of lines changed", multi, connA("postgres://h/b", false), true},
	} {
		path := connectionsFile(t, c.body)
		was := connA("postgres://h/a", strings.Contains(c.body, "read_only"))
		got, err := UpdateConnection("a", was, c.to)
		switch {
		case c.odd && (err == nil || err.Error() != errOdd().Error()):
			t.Errorf("%s: %v", c.label, err)
		case c.odd && readFile(t, path) != c.body:
			t.Errorf("%s: the file changed:\n%s", c.label, readFile(t, path))
		case !c.odd && err != nil:
			t.Errorf("%s: %v", c.label, err)
		case !c.odd && got != c.to:
			t.Errorf("%s: read back %+v, want %+v", c.label, got, c.to)
		}
	}
}

// Nothing is written when the edit gives a file that no longer reads, a name
// that is taken or an engine that is none; CheckUpdate says so without
// writing it (F8.16).
func TestCheckUpdate(t *testing.T) {
	body := oneConn + "\n[[connection]]\nname = \"b\"\nengine = \"mysql\"\ndsn = \"u@tcp(h:3306)/b\"\n"
	path := connectionsFile(t, body)
	was := connA("postgres://h/a", false)
	for label, c := range map[string]Connection{
		"an engine that is none": {Name: "a", Engine: "oracle", DSN: "x"},
		"a name taken":           {Name: "b", Engine: "postgres", DSN: "postgres://h/a"},
		"a name with a /":        {Name: "x/y", Engine: "postgres", DSN: "postgres://h/a"},
	} {
		if err := CheckUpdate("a", was, c); err == nil {
			t.Errorf("%s: no error", label)
		}
		if _, err := UpdateConnection("a", was, c); err == nil || readFile(t, path) != body {
			t.Errorf("%s: %v, written %v", label, err, readFile(t, path) != body)
		}
	}
	if err := CheckUpdate("a", was, connA("postgres://h/other", true)); err != nil || readFile(t, path) != body {
		t.Errorf("a good edit: %v, written %v", err, readFile(t, path) != body)
	}
}

// The file's own ways: CRLF breaks, tabs, a last line with no break, a block
// right below another, and the [[ connection ]] spelled with spaces
// (F8.16).
func TestUpdateConnectionFileForms(t *testing.T) {
	two := oneConn + "\n[[connection]]\nname = \"b\"\nengine = \"mysql\"\ndsn = \"u@tcp(h:3306)/b\"\n"
	for _, c := range []struct{ label, body, want string }{
		{"CRLF", strings.ReplaceAll("# top\n"+two, "\n", "\r\n"), strings.ReplaceAll("# top\n"+strings.Replace(oneConn, "h/a", "h/b", 1)+"read_only = true\n"+strings.TrimPrefix(two, oneConn), "\n", "\r\n")},
		{"tabs", "[[connection]]\n\tname = \"a\"\n\tengine = \"postgres\"\n\tdsn = \"postgres://h/a\"\n", "[[connection]]\n\tname = \"a\"\n\tengine = \"postgres\"\n\tdsn = \"postgres://h/b\"\n\tread_only = true\n"},
		{"no break at the end", strings.TrimSuffix(oneConn, "\n"), strings.Replace(oneConn, "h/a", "h/b", 1)[:len(oneConn)-1] + "\nread_only = true\n"},
		{"no blank between blocks", strings.Replace(two, "\n\n[[", "\n[[", 1), strings.Replace(strings.Replace(two, "\n\n[[", "\n[[", 1), "dsn = \"postgres://h/a\"\n", "dsn = \"postgres://h/b\"\nread_only = true\n", 1)},
		{"spaced header", "[[ connection ]]  # first\nname = \"a\"\nengine = \"postgres\"\ndsn = \"postgres://h/a\"\n", "[[ connection ]]  # first\nname = \"a\"\nengine = \"postgres\"\ndsn = \"postgres://h/b\"\nread_only = true\n"},
		{"[defaults] before", "[defaults]\ntimeout = 5\n\n" + oneConn, "[defaults]\ntimeout = 5\n\n" + strings.Replace(oneConn, "h/a", "h/b", 1) + "read_only = true\n"},
	} {
		path := connectionsFile(t, c.body)
		if c.label == "no break at the end" { // the dsn is the last line, rewritten with no break either
			if _, err := UpdateConnection("a", connA("postgres://h/a", false), connA("postgres://h/b", false)); err != nil {
				t.Fatal(err)
			}
			if got := readFile(t, path); got != strings.Replace(c.body, "h/a", "h/b", 1) {
				t.Errorf("rewritten, still no break: %q", got)
			}
			path = connectionsFile(t, c.body)
		}
		if _, err := UpdateConnection("a", connA("postgres://h/a", false), connA("postgres://h/b", true)); err != nil {
			t.Errorf("%s: %v", c.label, err)
		} else if got := readFile(t, path); got != c.want {
			t.Errorf("%s:\n%q\nwant\n%q", c.label, got, c.want)
		}
	}
}

// A block ends at its last key and the blank lines after it: a comment on a
// paragraph of its own below is the user's, whichever block it follows, and
// the comment lines glued to the next header are that block's (F8.16).
func TestConnectionBlockEnds(t *testing.T) {
	two := oneConn + "\n# ---- legacy below ----\n\n# old b\n[[connection]]\nname = \"b\"\nengine = \"mysql\"\ndsn = \"u@tcp(h:3306)/b\"\n"
	path := connectionsFile(t, two)
	if err := DeleteConnection("a"); err != nil {
		t.Fatal(err)
	}
	if got, want := readFile(t, path), "# ---- legacy below ----\n\n# old b\n[[connection]]\nname = \"b\"\nengine = \"mysql\"\ndsn = \"u@tcp(h:3306)/b\"\n"; got != want {
		t.Errorf("delete a:\n%s", got)
	}
	connectionsFile(t, two)
	if err := DeleteConnection("b"); err != nil {
		t.Fatal(err)
	}
	if got, want := readFile(t, connectionsPath()), oneConn+"\n# ---- legacy below ----\n\n"; got != want {
		t.Errorf("delete b, the comment above it kept with it:\n%q", got)
	}
	connectionsFile(t, two)
	if _, err := UpdateConnection("a", connA("postgres://h/a", false), connA("postgres://h/a", true)); err != nil {
		t.Fatal(err)
	}
	if got, want := readFile(t, connectionsPath()), strings.Replace(two, "dsn = \"postgres://h/a\"\n", "dsn = \"postgres://h/a\"\nread_only = true\n", 1); got != want {
		t.Errorf("a key added before the paragraph:\n%s", got)
	}
	commented := "# keep\n\n[[connection]]\nname = \"z\"\nengine = \"postgres\"\ndsn = \"postgres://h/z\"\n\n" + oneConn + "\n# prod, off for now\n# [[connection]]\n# name = \"prod\"\n"
	connectionsFile(t, commented)
	if err := DeleteConnection("a"); err != nil {
		t.Fatal(err)
	}
	if got, want := readFile(t, connectionsPath()), strings.Replace(commented, oneConn+"\n", "", 1); got != want {
		t.Errorf("a commented-out block at the end:\n%s", got)
	}
	defaults := two + "\n# my defaults\n[defaults]\ntimeout = 5\n"
	connectionsFile(t, defaults)
	if err := DeleteConnection("b"); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, connectionsPath()); !strings.HasSuffix(got, "\n# my defaults\n[defaults]\ntimeout = 5\n") || strings.Contains(got, "old b") {
		t.Errorf("[defaults] after:\n%s", got)
	}
}

// A deletion takes the block, the comment lines right above it and the
// blank lines after it; nothing else (F8.16).
func TestDeleteConnection(t *testing.T) {
	path := connectionsFile(t, editFile)
	if err := DeleteConnection("stage"); err != nil {
		t.Fatal(err)
	}
	want := strings.Replace(editFile, "# staging, read only\n[[connection]]\nname = \"stage\"\nengine = \"mysql\"\ndsn = \"u:p@tcp(h:3306)/stage\"\nread_only = true\n\n", "", 1)
	if got := readFile(t, path); got != want {
		t.Errorf("the middle one:\n%s", got)
	}
	if err := DeleteConnection("last"); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, path); !strings.HasSuffix(got, "dsn = \"postgres://localhost/play?sslmode=disable\"\n\n") {
		t.Errorf("the last one:\n%q", got)
	}
	if err := DeleteConnection("play"); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, path); strings.Contains(got, "[[connection]]") || !strings.HasPrefix(got, "# my databases\n") {
		t.Errorf("the only one left, then none:\n%q", got)
	}
	if err := DeleteConnection("play"); err == nil {
		t.Error("deleted twice")
	}
	crlf := strings.ReplaceAll("# top\n\n"+oneConn+"\n"+strings.Replace(oneConn, `"a"`, `"b"`, 1), "\n", "\r\n")
	connectionsFile(t, crlf)
	if err := DeleteConnection("a"); err != nil {
		t.Fatal(err)
	}
	if got, want := readFile(t, connectionsPath()), strings.ReplaceAll("# top\n\n"+strings.Replace(oneConn, `"a"`, `"b"`, 1), "\n", "\r\n"); got != want {
		t.Errorf("CRLF:\n%q", got)
	}
}

// What is left must read: a block's own sub-table does not survive it, and
// the file is not written (F8.16).
func TestDeleteConnectionRefusesWhatLeavesUnreadable(t *testing.T) {
	body := oneConn + "[connection.opts]\nx = 1\n\n[[connection]]\nname = \"b\"\nengine = \"mysql\"\ndsn = \"u@tcp(h:3306)/b\"\n"
	path := connectionsFile(t, body)
	if err := DeleteConnection("a"); err == nil || readFile(t, path) != body {
		t.Errorf("deleted anyway: %v\n%s", err, readFile(t, path))
	}
}

// The file is replaced whole, never cut and written over: a failure on the
// way leaves what the user wrote; its permissions stay, a link stays a link
// with the target written, and one that cannot be written is refused with
// the file as it was (F8.16).
func TestConnectionsWrite(t *testing.T) {
	path := connectionsFile(t, oneConn)
	was, to := connA("postgres://h/a", false), connA("postgres://h/b", false)
	swaps := 0
	beforeRename = func() {
		swaps++
		if got := readFile(t, path); got != oneConn {
			t.Errorf("the file was touched before the swap: %q", got)
		}
	}
	defer func() { beforeRename = func() {} }()
	if _, err := UpdateConnection("a", was, to); err != nil || swaps != 1 {
		t.Fatalf("%v, %d swaps", err, swaps)
	}
	beforeRename = func() {}

	target := filepath.Join(t.TempDir(), "dotfiles.toml")
	if err := os.WriteFile(target, []byte(oneConn), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, path); err != nil {
		t.Fatal(err)
	}
	if _, err := UpdateConnection("a", was, to); err != nil {
		t.Fatal(err)
	}
	if li, err := os.Lstat(path); err != nil || li.Mode()&os.ModeSymlink == 0 {
		t.Errorf("no longer a link: %v %v", li, err)
	}
	if fi, err := os.Stat(target); err != nil || fi.Mode().Perm() != 0o640 || !strings.Contains(readFile(t, target), "h/b") {
		t.Errorf("the target: %v %v\n%s", fi, err, readFile(t, target))
	}

	path = connectionsFile(t, oneConn)
	if err := os.Chmod(path, 0o444); err != nil {
		t.Fatal(err)
	}
	if _, err := UpdateConnection("a", was, to); err == nil || readFile(t, path) != oneConn {
		t.Errorf("a read-only file: %v\n%s", err, readFile(t, path))
	}
	if err := os.Chmod(path, 0o444); err != nil {
		t.Fatal(err)
	}
	if err := DeleteConnection("a"); err == nil || readFile(t, path) != oneConn {
		t.Errorf("a read-only file, a delete: %v", err)
	}
}
