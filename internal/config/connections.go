package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/BurntSushi/toml"

	"sqlmux/internal/i18n"
)

// Connection is one [[connection]] in connections.toml (§14).
type Connection struct {
	Name        string `toml:"name"`
	Engine      string `toml:"engine"`
	DSN         string `toml:"dsn"`
	PasswordCmd string `toml:"password_cmd,omitempty"`
	PasswordEnv string `toml:"password_env,omitempty"`
	Password    string `toml:"password,omitempty"`
	ReadOnly    bool   `toml:"read_only,omitempty"` // §13, from F5.3
}

// connectionsPath is connections.toml (§14).
func connectionsPath() string { return filepath.Join(Dir(), "connections.toml") }

// noConnections is connections.toml having none: no file, or none in it.
// The program starts all the same, with the form open, unless a connection
// was asked for (§14「启动时找不到连接」).
type noConnections string

func (e noConnections) Error() string { return string(e) }

// NoConnections reports whether err is that.
func NoConnections(err error) bool {
	var e noConnections
	return errors.As(err, &e)
}

// LoadConnections reads every connection in connections.toml, and which
// of them is connection name, or the first one when name is "" (§5).
func LoadConnections(name string) (conns []Connection, start int, err error) {
	path := connectionsPath()
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, 0, noConnections(fmt.Sprintf(i18n.T("找不到 %s"), path))
	}
	if err != nil {
		return nil, 0, err
	}
	if conns, err = parseConnections(path, string(data)); err != nil {
		return nil, 0, err
	}
	var names []string
	for _, c := range conns {
		names = append(names, c.Name)
	}
	if name != "" {
		if start = slices.Index(names, name); start < 0 {
			return nil, 0, fmt.Errorf(i18n.T("%s: 没有名为 %q 的连接，已有：%s"), path, name, strings.Join(names, ", "))
		}
	}
	return conns, start, nil
}

// parseConnections is the connections text, the file at path, holds, each
// checked.
func parseConnections(path, text string) ([]Connection, error) {
	var file struct {
		Connection []Connection `toml:"connection"`
	}
	if _, err := toml.Decode(text, &file); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if len(file.Connection) == 0 {
		return nil, noConnections(fmt.Sprintf(i18n.T("%s: 里面没有 [[connection]]"), path))
	}
	var names []string
	for i, c := range file.Connection {
		if _, p := c.Check(names); p != "" {
			return nil, fmt.Errorf(i18n.T("%s: 第 %d 个连接：%s"), path, i+1, p)
		}
		names = append(names, c.Name)
	}
	return file.Connection, nil
}

// Check is what makes c unusable beside the connections names, and the
// field at fault: "", "" when nothing does (§14).
func (c Connection) Check(names []string) (field, problem string) {
	for _, f := range [][2]string{{"name", c.Name}, {"engine", c.Engine}, {"dsn", c.DSN}} {
		if f[1] == "" {
			return f[0], i18n.T("缺少 %s", f[0])
		}
	}
	// the name keys state.json's tables and tree node IDs (§14)
	if checkFileName(c.Name) != nil {
		return "name", fmt.Sprintf(i18n.T(`name = %q：不能含 / 或 \，也不能以 . 开头`), c.Name)
	}
	if slices.Contains(names, c.Name) { // the tree's roots go by it too (§5)
		return "name", fmt.Sprintf(i18n.T("name = %q 重复了"), c.Name)
	}
	if c.Engine != "postgres" && c.Engine != "mysql" {
		return "engine", fmt.Sprintf(i18n.T("engine = %q：只能是 postgres 或 mysql"), c.Engine)
	}
	return "", ""
}

// appendText is the text AppendConnection writes for c, a blank line
// before it, a newline first if the file lacks one. What the file would be
// is parsed first: one it would break (its connections an inline array, a
// name taken since it was read) is the error.
func appendText(c Connection) (string, error) {
	var b strings.Builder
	enc := toml.NewEncoder(&b)
	enc.Indent = ""
	if err := enc.Encode(struct {
		Connection []Connection `toml:"connection"`
	}{[]Connection{c}}); err != nil {
		return "", err
	}
	old, err := os.ReadFile(connectionsPath())
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return "", err
	}
	text := b.String()
	if len(old) > 0 {
		if old[len(old)-1] != '\n' {
			text = "\n" + text
		}
		text = "\n" + text
	}
	if _, err := parseConnections(connectionsPath(), string(old)+text); err != nil {
		return "", err
	}
	return text, nil
}

// CheckAppend is the error AppendConnection would give for c, nothing
// written, a file that cannot be appended to included: asked before a
// password goes into the keychain for it (F7.3, F8.16).
func CheckAppend(c Connection) error {
	if f, err := os.OpenFile(connectionsPath(), os.O_WRONLY|os.O_APPEND, 0); err == nil {
		_ = f.Close()
	} else if !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	_, err := appendText(c)
	return err
}

// AppendConnection adds c at the end of connections.toml, made 0600 if
// there is none; the permissions of one there is are the user's (§13, §14):
// its text written after what is there, never the file encoded again, so the
// user's comments stay.
// ponytail: called in Update, the file read and written as the UI waits; a
// Cmd if a slow disk ever shows it
func AppendConnection(c Connection) error {
	text, err := appendText(c)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(Dir(), 0o700); err != nil {
		return err
	}
	f, err := os.OpenFile(connectionsPath(), os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString(text)
	return err
}

// envRef is a ${NAME} in dsn or password.
var envRef = regexp.MustCompile(`\$\{([^}]+)\}`)

// Expanded is c with every ${NAME} in dsn and password replaced by the
// environment variable's value (§13), password only when it is the source
// of the password. A variable not set, or empty, is an error naming it, the
// rest of c as it is.
// ponytail: only ${NAME}; $NAME is left alone (a password or DSN may hold
// a $), and there is no ${NAME:-default}
func (c Connection) Expanded() (Connection, error) {
	var err error
	expand := func(s string) string {
		return envRef.ReplaceAllStringFunc(s, func(ref string) string {
			v := os.Getenv(ref[2 : len(ref)-1])
			if v == "" && err == nil {
				err = fmt.Errorf(i18n.T("%s：这个环境变量没有设置"), ref)
			}
			return v
		})
	}
	c.DSN = expand(c.DSN)
	if c.PasswordCmd == "" && c.PasswordEnv == "" { // else password is not the source: Secret() never reads it
		c.Password = expand(c.Password)
	}
	return c, err
}

// Secret is the password from the first source set: password_cmd,
// password_env, password (§13). "" leaves it to the DSN and ~/.pgpass.
func (c Connection) Secret() (string, error) {
	switch {
	case c.PasswordCmd != "":
		out, err := exec.Command("sh", "-c", c.PasswordCmd).Output()
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			return "", fmt.Errorf("password_cmd: %w: %s", err, strings.TrimSpace(string(exit.Stderr)))
		}
		if err != nil {
			return "", fmt.Errorf("password_cmd: %w", err)
		}
		return strings.TrimSuffix(string(out), "\n"), nil
	case c.PasswordEnv != "":
		if s := os.Getenv(c.PasswordEnv); s != "" {
			return s, nil
		}
		return "", fmt.Errorf(i18n.T("password_env = %q：这个环境变量没有设置"), c.PasswordEnv)
	}
	return c.Password, nil
}
