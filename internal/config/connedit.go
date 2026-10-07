package config

import (
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/BurntSushi/toml"

	"sqlmux/internal/i18n"
)

// A connection is edited, or deleted, in the lines of connections.toml: the
// file is never encoded again, so what the user wrote around it, comments,
// blank lines, other blocks, stays byte for byte (F8.16). A block runs from
// its [[connection]] line to the next line that opens a table, or the end;
// the comment lines right above it, no blank between, go with it.

var (
	connHeader = regexp.MustCompile(`^\s*\[\[\s*connection\s*\]\]`)
	tableHead  = regexp.MustCompile(`^\s*\[`)
)

// connFields are the keys of a connection, in the order they are written.
var connFields = []string{"name", "engine", "dsn", "password_cmd", "password_env", "password", "read_only"}

func (c Connection) field(key string) any {
	switch key {
	case "name":
		return c.Name
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
	}
	return c.ReadOnly
}

// connBlock is a block's lines in a file split into them: start the first of
// its comments, head its [[connection]], end past its last line.
type connBlock struct{ start, head, end int }

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

func connBlocks(lines []string) []connBlock {
	var bs []connBlock
	for i, l := range lines {
		switch {
		case connHeader.MatchString(l):
			start := i
			for start > 0 && strings.HasPrefix(strings.TrimSpace(lines[start-1]), "#") && (len(bs) == 0 || start-1 > bs[len(bs)-1].head) {
				start--
			}
			if n := len(bs); n > 0 {
				bs[n-1].end = min(bs[n-1].end, start)
			}
			bs = append(bs, connBlock{start, i, len(lines)})
		case tableHead.MatchString(l): // another table ends the block before it
			if n := len(bs); n > 0 {
				bs[n-1].end = min(bs[n-1].end, i)
			}
		}
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
	return connBlock{}, Connection{}, fmt.Errorf(i18n.T("%s 里没有叫 %s 的连接"), connectionsPath(), name)
}

// keyLine is the line of the key in lines[from:to], -1 for none; an error
// when its value goes over lines or is written in a way edited line by line
// would break (a quoted or dotted key, an inline table, a multi-line string).
var keyLine = regexp.MustCompile(`^\s*([A-Za-z0-9_-]+)\s*=\s*(.*)$`)

func lineOfKey(lines []string, from, to int, key string) (int, error) {
	for i := from; i < to; i++ {
		m := keyLine.FindStringSubmatch(strings.TrimRight(lines[i], "\r\n"))
		if len(m) == 0 || m[1] != key {
			continue
		}
		if v := strings.TrimSpace(m[2]); strings.HasPrefix(v, `"""`) || strings.HasPrefix(v, "'''") || strings.HasPrefix(v, "[") || strings.HasPrefix(v, "{") {
			return -1, fmt.Errorf(i18n.T("%s 的 %s 写得太特别，请手工改"), connectionsPath(), key)
		}
		return i, nil
	}
	return -1, nil
}

// tomlLine is `key = value` for v, as TOML writes it.
func tomlLine(key string, v any) (string, error) {
	var b strings.Builder
	enc := toml.NewEncoder(&b)
	if err := enc.Encode(map[string]any{key: v}); err != nil {
		return "", err
	}
	return b.String(), nil
}

// UpdateConnection makes the connection called old what c says, in its
// block only: a key changed is its line rewritten, a key new is added after
// the block's last key, one now empty is its line taken out; the rest of the
// file, and the file's permissions, are the user's (§14, F8.16).
// ponytail: a comment at the end of a rewritten line goes with the line.
func UpdateConnection(old string, c Connection) error {
	path := connectionsPath()
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	lines := splitLines(string(data))
	b, was, err := findConnection(lines, old)
	if err != nil {
		return err
	}
	nl := "\n"
	if strings.HasSuffix(lines[b.head], "\r\n") {
		nl = "\r\n"
	}
	for _, key := range connFields {
		have, want := was.field(key), c.field(key)
		if have == want {
			continue
		}
		at, err := lineOfKey(lines, b.head+1, b.end, key)
		if err != nil {
			return err
		}
		switch zero := want == "" || want == false; {
		case zero && at >= 0: // cleared: its line goes
			lines = append(lines[:at], lines[at+1:]...)
			b.end--
		case zero:
		default:
			text, err := tomlLine(key, want)
			if err != nil {
				return err
			}
			text = strings.TrimSuffix(text, "\n") + nl
			if at >= 0 {
				lines[at] = text
				break
			}
			last := b.head // after the block's last key line
			for i := b.head + 1; i < b.end; i++ {
				if keyLine.MatchString(strings.TrimRight(lines[i], "\r\n")) {
					last = i
				}
			}
			if !strings.HasSuffix(lines[last], "\n") { // the file's last line has no break yet
				lines[last] += nl
			}
			lines = append(lines[:last+1], append([]string{text}, lines[last+1:]...)...)
			b.end++
		}
	}
	out := strings.Join(lines, "")
	if _, err := parseConnections(path, out); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(out), 0o600) // an existing file keeps its permissions
}

// DeleteConnection takes the block of the connection called name out of
// connections.toml, with the comment lines right above it and the blank
// lines after it, which are its own (F8.16). The last one may go: the
// program then starts with the form (§14).
func DeleteConnection(name string) error {
	path := connectionsPath()
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	lines := splitLines(string(data))
	b, _, err := findConnection(lines, name)
	if err != nil {
		return err
	}
	return os.WriteFile(path, []byte(strings.Join(append(lines[:b.start], lines[b.end:]...), "")), 0o600)
}
