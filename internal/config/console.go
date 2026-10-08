package config

import (
	"cmp"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"
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

// ConsoleFiles is the names of the console files (the .sql files of
// consoles/, without it), console_<n> by n first, the others by name (§7.8
// 「Consoles 节点」); and when the directory last changed, for a cache of it
// to know. None, no time, when there is no directory yet.
func ConsoleFiles() (names []string, changed time.Time, err error) {
	dir := filepath.Dir(ConsolePath("x"))
	fi, err := os.Stat(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, time.Time{}, nil
	}
	if err != nil {
		return nil, time.Time{}, err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, time.Time{}, err
	}
	for _, e := range entries {
		if name, ok := strings.CutSuffix(e.Name(), ".sql"); ok && !e.IsDir() {
			names = append(names, name)
		}
	}
	SortConsoles(names)
	return names, fi.ModTime(), nil
}

// SortConsoles puts console_<n> by n first, the renamed ones by name after.
func SortConsoles(names []string) {
	number := func(name string) int {
		if n, ok := strings.CutPrefix(name, "console_"); ok {
			if i, err := strconv.Atoi(n); err == nil && i >= 0 && strconv.Itoa(i) == n {
				return i
			}
		}
		return -1
	}
	slices.SortFunc(names, func(a, b string) int {
		na, nb := number(a), number(b)
		switch {
		case na >= 0 && nb >= 0:
			return cmp.Compare(na, nb)
		case na >= 0:
			return -1
		case nb >= 0:
			return 1
		}
		return strings.Compare(a, b)
	})
}

// ConsoleExists is whether the console called name has a file: it is
// written once there is a word in it, and not deleted by closing it (§11).
func ConsoleExists(name string) bool {
	_, err := os.Stat(ConsolePath(name))
	return err == nil
}

// DeleteConsole deletes the console file called name; none is no error.
func DeleteConsole(name string) error {
	if err := os.Remove(ConsolePath(name)); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}
