package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"github.com/BurntSushi/toml"
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

// LoadConnections reads every connection in connections.toml, and which
// of them is connection name, or the first one when name is "" (§5).
// warning is set when the file holds a plain password that group or others
// can read (§13).
func LoadConnections(name string) (conns []Connection, start int, warning string, err error) {
	path := connectionsPath()
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, 0, "", fmt.Errorf("找不到 %s", path)
	}
	if err != nil {
		return nil, 0, "", err
	}
	var file struct {
		Connection []Connection `toml:"connection"`
	}
	if _, err := toml.Decode(string(data), &file); err != nil {
		return nil, 0, "", fmt.Errorf("%s: %w", path, err)
	}
	conns = file.Connection
	if len(conns) == 0 {
		return nil, 0, "", fmt.Errorf("%s: 里面没有 [[connection]]", path)
	}
	var names []string
	for i, c := range conns {
		if _, p := c.Check(names); p != "" {
			return nil, 0, "", fmt.Errorf("%s: 第 %d 个连接：%s", path, i+1, p)
		}
		names = append(names, c.Name)
	}
	if name != "" {
		if start = slices.Index(names, name); start < 0 {
			return nil, 0, "", fmt.Errorf("%s: 没有名为 %q 的连接，已有：%s", path, name, strings.Join(names, ", "))
		}
	}
	plain := slices.ContainsFunc(conns, func(c Connection) bool { return c.Password != "" })
	if info, err := os.Stat(path); err == nil && plain && info.Mode().Perm()&0o044 != 0 {
		warning = "connections.toml 里有明文密码，且其他用户可读，建议 chmod 600"
	}
	return conns, start, warning, nil
}

// Check is what makes c unusable beside the connections names, and the
// field at fault: "", "" when nothing does (§14).
func (c Connection) Check(names []string) (field, problem string) {
	for _, f := range [][2]string{{"name", c.Name}, {"engine", c.Engine}, {"dsn", c.DSN}} {
		if f[1] == "" {
			return f[0], "缺少 " + f[0]
		}
	}
	// the name keys state.json's tables and tree node IDs (§14)
	if strings.ContainsAny(c.Name, `/\`) || strings.HasPrefix(c.Name, ".") {
		return "name", fmt.Sprintf(`name = %q：不能含 / 或 \，也不能以 . 开头`, c.Name)
	}
	if slices.Contains(names, c.Name) { // the tree's roots go by it too (§5)
		return "name", fmt.Sprintf("name = %q 重复了", c.Name)
	}
	if c.Engine != "postgres" {
		return "engine", fmt.Sprintf("engine = %q，目前只支持 postgres", c.Engine)
	}
	return "", ""
}

// AppendConnection adds c at the end of connections.toml, made 0600 if
// there is none (§14): its text written after what is there, never the
// file encoded again, so the user's comments stay.
func AppendConnection(c Connection) error {
	var b strings.Builder
	enc := toml.NewEncoder(&b)
	enc.Indent = ""
	if err := enc.Encode(struct {
		Connection []Connection `toml:"connection"`
	}{[]Connection{c}}); err != nil {
		return err
	}
	if err := os.MkdirAll(Dir(), 0o700); err != nil {
		return err
	}
	f, err := os.OpenFile(connectionsPath(), os.O_RDWR|os.O_APPEND|os.O_CREATE, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	text := b.String()
	if info, err := f.Stat(); err == nil && info.Size() > 0 { // a blank line before it, a newline first if the file lacks one
		last := make([]byte, 1)
		if _, err := f.ReadAt(last, info.Size()-1); err == nil && last[0] != '\n' {
			text = "\n" + text
		}
		text = "\n" + text
	}
	_, err = f.WriteString(text)
	return err
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
		return "", fmt.Errorf("password_env = %q：这个环境变量没有设置", c.PasswordEnv)
	}
	return c.Password, nil
}
