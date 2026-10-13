package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/BurntSushi/toml"

	"github.com/kackerx/sqlmux/internal/i18n"
)

// A connection is edited, or deleted, in the lines of connections.toml: the
// file is never encoded again, so what the user wrote around it, comments,
// blank lines, other blocks, stays byte for byte (F8.16). A block is its
// [[connection]] line, the keys under it up to the last one, and the blank
// lines after that; the comment lines right above the header, no blank
// between, go with it. A comment on a paragraph of its own, below a blank
// line, is the user's, whichever block it follows.

const tomlKey = `(?:[A-Za-z0-9_-]+|"(?:[^"\\]|\\.)*"|'[^']*')`

var (
	connHeader = regexp.MustCompile(`^\s*\[\[\s*connection\s*\]\]`)
	// tableHead is a line that is a table's header and nothing else, its name
	// a TOML key, dotted or in quotes: not the first line of an array written
	// over several lines ([1, 2],).
	tableHead = regexp.MustCompile(`^\s*\[\[?\s*` + tomlKey + `(?:\s*\.\s*` + tomlKey + `)*\s*\]\]?\s*(?:#.*)?$`)
	keyLine   = regexp.MustCompile(`^(\s*)([A-Za-z0-9_-]+)\s*=`)
)

// connFields are the keys of a connection, in the order they are written.
var connFields = []string{"name", "group", "engine", "dsn", "password_cmd", "password_env", "password", "read_only", "ssh"}

func (c Connection) field(key string) any {
	switch key {
	case "name":
		return c.Name
	case "group":
		return c.Group
	case "engine":
		return c.Engine
	case "dsn":
		return c.DSN
	case "password_cmd":
		return c.PasswordCmd
	case "password_env":
		return c.PasswordEnv
	case "password":
		return c.Password
	case "ssh":
		return c.SSH
	}
	return c.ReadOnly
}

// with is c with its key taken from from.
func (c Connection) with(key string, from Connection) Connection {
	switch key {
	case "name":
		c.Name = from.Name
	case "group":
		c.Group = from.Group
	case "engine":
		c.Engine = from.Engine
	case "dsn":
		c.DSN = from.DSN
	case "password_cmd":
		c.PasswordCmd = from.PasswordCmd
	case "password_env":
		c.PasswordEnv = from.PasswordEnv
	case "password":
		c.Password = from.Password
	case "ssh":
		c.SSH = from.SSH
	default:
		c.ReadOnly = from.ReadOnly
	}
	return c
}

// connBlock is a block's lines in a file split into them: start the first of
// its comments, head its [[connection]], last its last line with a key or a
// value on it, end past the blank lines after that.
type connBlock struct{ start, head, last, end int }

// splitLines is text in its lines, each with the line break it had.
func splitLines(text string) []string {
	var lines []string
	for text != "" {
		i := strings.IndexByte(text, '\n') + 1
		if i == 0 {
			i = len(text)
		}
		lines, text = append(lines, text[:i]), text[i:]
	}
	return lines
}

func isComment(line string) bool { return strings.HasPrefix(strings.TrimSpace(line), "#") }

func connBlocks(lines []string) []connBlock {
	var bs []connBlock
	for i, l := range lines {
		if !connHeader.MatchString(l) {
			continue
		}
		b := connBlock{start: i, head: i, last: i}
		for b.start > 0 && isComment(lines[b.start-1]) {
			b.start--
		}
		next := i + 1
		for ; next < len(lines) && !tableHead.MatchString(strings.TrimRight(lines[next], "\r\n")); next++ {
			if s := strings.TrimSpace(lines[next]); s != "" && !isComment(s) {
				b.last = next
			}
		}
		b.end = b.last + 1
		for b.end < next && strings.TrimSpace(lines[b.end]) == "" {
			b.end++
		}
		bs = append(bs, b)
	}
	return bs
}

// findConnection is the block of connection name in lines, and what it says.
func findConnection(lines []string, name string) (connBlock, Connection, error) {
	for _, b := range connBlocks(lines) {
		var f struct {
			Connection []Connection `toml:"connection"`
		}
		if _, err := toml.Decode(strings.Join(lines[b.head:b.end], ""), &f); err == nil && len(f.Connection) == 1 && f.Connection[0].Name == name {
			return b, f.Connection[0], nil
		}
	}
	var f struct {
		Connection []Connection `toml:"connection"`
	}
	if _, err := toml.Decode(strings.Join(lines, ""), &f); err == nil && slices.ContainsFunc(f.Connection, func(c Connection) bool { return c.Name == name }) {
		return connBlock{}, Connection{}, errOdd() // there, in lines no block of ours holds whole (a # or a [x] inside a value of lines)
	}
	return connBlock{}, Connection{}, fmt.Errorf(i18n.T("%s 里没有叫 %s 的连接"), connectionsPath(), name)
}

// ReadConnection is the connection called name as connections.toml says it
// now: the panel reads it again where it shows it, the file being the
// user's too (F8.16).
func ReadConnection(name string) (Connection, error) {
	data, err := os.ReadFile(connectionsPath())
	if err != nil {
		return Connection{}, err
	}
	_, c, err := findConnection(splitLines(string(data)), name)
	return c, err
}

// errOdd is an edit this way of editing cannot make: the text it would give
// does not read back as asked (a key in quotes, a value over several lines).
func errOdd() error {
	return fmt.Errorf(i18n.T("%s 里这一项写法太特别，请手工改"), connectionsPath())
}

// lineOfKey is the line of key in lines[from:to], of any key when key is
// "", -1 for none.
func lineOfKey(lines []string, from, to int, key string) int {
	for i := from; i < to; i++ {
		if m := keyLine.FindStringSubmatch(strings.TrimRight(lines[i], "\r\n")); len(m) > 0 && (key == "" || m[2] == key) {
			return i
		}
	}
	return -1
}

// indentOf is the white space the key on line starts with.
func indentOf(line string) string {
	return keyLine.FindStringSubmatch(strings.TrimRight(line, "\r\n"))[1]
}

// tomlLine is `key = value` for v, as TOML writes it.
func tomlLine(key string, v any) (string, error) {
	var b strings.Builder
	if err := toml.NewEncoder(&b).Encode(map[string]any{key: v}); err != nil {
		return "", err
	}
	return strings.TrimSuffix(b.String(), "\n"), nil
}

// lineEnd is the line break line has, "" for the file's last without one.
func lineEnd(line string) string {
	if strings.HasSuffix(line, "\r\n") {
		return "\r\n"
	}
	if strings.HasSuffix(line, "\n") {
		return "\n"
	}
	return ""
}

// editedText is connections.toml as it would be with connection old, which
// the caller read as was, made c: a key that differs between the two is
// written in the block's lines as it is on disk now, so a key the user
// changed there meanwhile stays; one new goes after the block's last, one
// now empty is taken out. got is what the new text says the block is.
// ponytail: a comment at the end of a rewritten line goes with the line.
func editedText(old string, was, c Connection) (text string, got Connection, err error) {
	path := connectionsPath()
	data, err := os.ReadFile(path)
	if err != nil {
		return "", Connection{}, err
	}
	if _, err := toml.Decode(string(data), &map[string]any{}); err != nil { // not ours to say it is odd
		return "", Connection{}, fmt.Errorf("%s: %w", path, err)
	}
	lines := splitLines(string(data))
	b, want, err := findConnection(lines, old)
	if err != nil {
		return "", Connection{}, err
	}
	nl := "\n"
	if lineEnd(lines[b.head]) == "\r\n" {
		nl = "\r\n"
	}
	for _, key := range connFields {
		have, to := was.field(key), c.field(key)
		if have == to {
			continue
		}
		want = want.with(key, c)
		at := lineOfKey(lines, b.head+1, b.last+1, key)
		zero := to == "" || to == false
		switch line, err := tomlLine(key, to); {
		case err != nil:
			return "", Connection{}, err
		case zero && at >= 0: // cleared: its line goes
			lines = slices.Delete(lines, at, at+1)
			b.last--
		case zero:
		case at >= 0: // keeps the indent and the break the line had
			lines[at] = indentOf(lines[at]) + line + lineEnd(lines[at])
		default:
			indent := ""
			if k := lineOfKey(lines, b.head+1, b.last+1, ""); k >= 0 {
				indent = indentOf(lines[k])
			}
			if lineEnd(lines[b.last]) == "" { // the file's last line has no break yet
				lines[b.last] += nl
			}
			lines = slices.Insert(lines, b.last+1, indent+line+nl)
			b.last++
		}
	}
	text = strings.Join(lines, "")
	conns, err := parseConnections(path, text)
	var perr toml.ParseError
	if errors.As(err, &perr) { // the old text read: this edit broke it
		return "", Connection{}, errOdd()
	}
	if err != nil {
		return "", Connection{}, err
	}
	i := slices.IndexFunc(conns, func(o Connection) bool { return o.Name == c.Name })
	if i < 0 || conns[i] != want {
		return "", Connection{}, errOdd()
	}
	return text, conns[i], nil
}

// CheckUpdate is the error UpdateConnection would give, nothing written, a
// file that cannot be replaced included: asked before a password goes into
// the keychain for it (F8.16).
func CheckUpdate(old string, was, c Connection) error {
	if _, _, err := writableConnections(); err != nil {
		return err
	}
	_, _, err := editedText(old, was, c)
	return err
}

// UpdateConnection makes the connection called old, which the caller read
// as was, what c says, in its block of connections.toml only (§14, F8.16),
// and returns the block as the file now says it. The file is written
// through a temporary one, renamed over it with its permissions: a
// failure keeps what the user wrote, and a link stays a link.
func UpdateConnection(old string, was, c Connection) (Connection, error) {
	text, got, err := editedText(old, was, c)
	if err != nil {
		return Connection{}, err
	}
	return got, rewriteConnections(text)
}

// writableConnections is the file connections.toml is, behind its links,
// and its permissions, when it can be replaced: it is not read-only, and a
// file can be made beside it.
func writableConnections() (path string, perm os.FileMode, err error) {
	if path, err = filepath.EvalSymlinks(connectionsPath()); err != nil {
		return "", 0, err
	}
	fi, err := os.Stat(path)
	if err != nil {
		return "", 0, err
	}
	if fi.Mode().Perm()&0o200 == 0 {
		return "", 0, fmt.Errorf(i18n.T("%s 不可写"), path)
	}
	return path, fi.Mode().Perm(), probeDir(filepath.Dir(path))
}

// probeDir is the error of making a file in dir, made first when there is
// none: nothing is left of it.
func probeDir(dir string) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, "probe-*")
	if err != nil {
		return err
	}
	_ = tmp.Close()
	return os.Remove(tmp.Name())
}

// rewriteConnections writes the text of connections.toml in place.
func rewriteConnections(text string) error {
	path, perm, err := writableConnections()
	if err != nil {
		return err
	}
	return writeFileMode(path, []byte(text), perm)
}

// DeleteConnection takes the block of the connection called name out of
// connections.toml, with the comment lines right above it and the blank
// lines after it, which are its own (F8.16). The last one may go: the
// program then starts with the form (§14).
func DeleteConnection(name string) error {
	data, err := os.ReadFile(connectionsPath())
	if err != nil {
		return err
	}
	lines := splitLines(string(data))
	b, _, err := findConnection(lines, name)
	if err != nil {
		return err
	}
	text := strings.Join(slices.Delete(lines, b.start, b.end), "")
	if _, err := parseConnections(connectionsPath(), text); err != nil && !NoConnections(err) { // [connection.opts] left without its [[connection]]
		return err
	}
	return rewriteConnections(text)
}
