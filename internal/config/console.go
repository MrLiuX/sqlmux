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

// ConsolePath is the file of console n (§11), whatever connection it is on.
func ConsolePath(n int) string {
	return filepath.Join(DataDir(), "consoles", fmt.Sprintf("console_%d.sql", n))
}

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
