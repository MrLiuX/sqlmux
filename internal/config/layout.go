package config

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/BurntSushi/toml"

	"sqlmux/internal/i18n"
)

// Layout is the working set as it is saved (§14「布局文件」): the sessions,
// each with its windows, their split trees and panes, and the tabs in them;
// what is not saved is what cannot be: results, queries running, edits not
// saved. Plain values, a TOML file a person can write.
type Layout struct {
	Version  int             `toml:"version"`
	Active   int             `toml:"active"` // the session attached
	Sessions []LayoutSession `toml:"session"`
}

// LayoutVersion is the one format there is.
const LayoutVersion = 1

type LayoutSession struct {
	Name       string         `toml:"name"`
	Connection string         `toml:"connection"` // the one it starts on
	Active     int            `toml:"active"`     // the window shown
	Windows    []LayoutWindow `toml:"window"`
}

type LayoutWindow struct {
	Name         string       `toml:"name"`
	Layout       *LayoutNode  `toml:"layout"`
	Focus        int          `toml:"focus,omitempty"` // a pane ID; 0 for the sidebar, or none: the result area is not saved
	Zoom         int          `toml:"zoom,omitempty"`
	SidebarOpen  bool         `toml:"sidebar_open"`
	SidebarWidth int          `toml:"sidebar_width,omitempty"`
	ResultHeight float64      `toml:"result_height,omitempty"` // the share the rest above the result area keeps
	Panes        []LayoutPane `toml:"pane"`
}

// LayoutNode is the split tree: a split of two, or a leaf, a pane by ID.
type LayoutNode struct {
	Split string      `toml:"split,omitempty"` // "h" side by side, "v" one over the other; "" for a leaf
	Ratio float64     `toml:"ratio,omitempty"` // the share of a
	A     *LayoutNode `toml:"a,omitempty"`
	B     *LayoutNode `toml:"b,omitempty"`
	Pane  int         `toml:"pane,omitempty"`
}

type LayoutPane struct {
	ID      int         `toml:"id"`
	Current int         `toml:"current,omitempty"` // the tab shown, from 0
	Tabs    []LayoutTab `toml:"tab"`
}

// LayoutTab is a table's or a console's: Kind says which.
type LayoutTab struct {
	Kind       string `toml:"kind"` // "table" | "console"
	Connection string `toml:"connection,omitempty"`
	Schema     string `toml:"schema,omitempty"`
	// a table's
	Table     string   `toml:"table,omitempty"`
	Where     string   `toml:"where,omitempty"`
	Order     string   `toml:"order,omitempty"`
	Desc      bool     `toml:"desc,omitempty"`
	Limit     int      `toml:"limit,omitempty"`
	Page      int      `toml:"page,omitempty"` // from 0
	Cols      []string `toml:"cols,omitempty"` // the columns COLS hides
	Transpose bool     `toml:"transpose,omitempty"`
	// a console's: console_<n>
	N int `toml:"n,omitempty"`
}

// LayoutsDir is where the named layouts are.
func LayoutsDir() string { return filepath.Join(Dir(), "layouts") }

// LastPath is where the working set is kept as the program ends.
func LastPath() string { return filepath.Join(StateDir(), "last.toml") }

// LayoutNames are the layouts of LayoutsDir, by name, sorted.
func LayoutNames() []string {
	files, _ := filepath.Glob(filepath.Join(LayoutsDir(), "*.toml"))
	names := make([]string, len(files))
	for i, f := range files {
		names[i] = strings.TrimSuffix(filepath.Base(f), ".toml")
	}
	slices.Sort(names)
	return names
}

// LayoutPath is the file of the layout called name: layouts/<name>.toml.
func LayoutPath(name string) string { return filepath.Join(LayoutsDir(), name+".toml") }

// LayoutArg is the file --layout names: a path as it is (it has a / or ends
// in .toml), else a layout's by name. Only the command line takes both; the
// commands go by name (§14「命令」).
func LayoutArg(nameOrPath string) string {
	if nameOrPath == "last" { // where the program left off, as layout.open's
		return LastPath()
	}
	if strings.ContainsRune(nameOrPath, '/') || strings.HasSuffix(nameOrPath, ".toml") {
		return nameOrPath
	}
	return LayoutPath(nameOrPath)
}

// CheckLayoutName says why name cannot name a layout: it is a file of
// LayoutsDir, and last is last.toml's. The caller has trimmed it, and not
// asked for a blank one.
func CheckLayoutName(name string) error {
	switch {
	case strings.ContainsAny(name, `/\`) || strings.HasPrefix(name, "."):
		return errors.New(i18n.T("名字不能含 / 或 \\，也不能以 . 开头"))
	case name == "last":
		return errors.New(i18n.T("last 是保留名，指上次退出时的布局"))
	}
	return nil
}

// LoadLayout reads the layout at path. A file that is not there is
// fs.ErrNotExist in the error; one that does not read, or is of another
// version, says so with its path.
func LoadLayout(path string) (*Layout, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	l := &Layout{}
	if err := toml.Unmarshal(data, l); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if l.Version != LayoutVersion {
		return nil, fmt.Errorf(i18n.T("%s: version = %d，只认识 %d"), path, l.Version, LayoutVersion)
	}
	if len(l.Sessions) == 0 {
		return nil, fmt.Errorf(i18n.T("%s: 里面没有 [[session]]"), path)
	}
	return l, nil
}

// SaveLayout writes l to path whole, through a file renamed into place, the
// directories made for the user only.
func SaveLayout(path string, l *Layout) error {
	l.Version = LayoutVersion
	var b bytes.Buffer
	if err := toml.NewEncoder(&b).Encode(l); err != nil {
		return err
	}
	return writeFile(path, b.Bytes())
}

// BrokenLast moves last.toml aside, as state.json's is when it cannot be
// read, so the next exit does not write over what the user had.
func BrokenLast() error {
	err := os.Rename(LastPath(), LastPath()+".broken")
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	return err
}
