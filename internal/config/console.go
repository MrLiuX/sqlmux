package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// DataDir is $XDG_DATA_HOME/sqlmux, falling back to ~/.local/share/sqlmux (§14).
func DataDir() string {
	if d := os.Getenv("XDG_DATA_HOME"); d != "" {
		return filepath.Join(d, "sqlmux")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "share", "sqlmux")
}

// ConsoleName is what the nth console is called until it is renamed (§11).
func ConsoleName(n int) string { return fmt.Sprintf("console_%d", n) }

// ConsolePath is the file of the console called name (§11), whatever
// connection it is on.
func ConsolePath(name string) string {
	return filepath.Join(DataDir(), "consoles", name+".sql")
}

// CheckConsoleName is why name is no name for a console: it is a file's,
// so the rule is a connection's (F8.11). The caller has trimmed it, and not
// asked for a blank one.
func CheckConsoleName(name string) error { return checkFileName(name) }

// ReadConsole is the console file at path; "" when there is none yet.
func ReadConsole(path string) (string, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return "", nil
	}
	return string(data), err
}

// WriteConsole writes a console's text to path, the user's only (§11, §13).
func WriteConsole(path, text string) error { return writeFile(path, []byte(text)) }
