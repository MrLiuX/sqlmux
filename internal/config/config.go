// Package config reads config.toml and keymaps.toml from the XDG config dir
// (tech-design §14).
package config

import (
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"sqlmux/internal/i18n"
	"strings"

	"github.com/BurntSushi/toml"

	"sqlmux/internal/ui"
)

type Config struct {
	Theme      *ui.Theme // `theme = "<name>"` (§7.3), resolved by Load
	Icons      *ui.Icons // `icons = "nerd" | "ascii"`, with the theme file's [icon] on top (§7.7)
	BaseIcons  *ui.Icons // what `icons` chose, a theme's [icon] not on it: a theme switched to goes on this
	ThemeName  string    // the theme in use: `theme`, or tokyonight-storm
	Language   string    // `language = "zh" | "en"` (§14「界面语言」)
	Vim        bool      // `vim = false`: console and WHERE are plain editors, the grid and tree get arrow keys too (§6.9)
	Timeoutlen int       // ms to wait on an ambiguous key sequence
	TabWidth   int       // the console's indent, and the formatter's (§14)
	// ResultHeight is the share of the window the result area first takes;
	// MaxRows the rows a console's run keeps of each result (§11).
	ResultHeight float64
	MaxRows      int
	// KeywordCase and FormatPrg are gq's (§9.5): sql-formatter's keywordCase,
	// or a command to format with instead of it.
	KeywordCase string
	FormatPrg   string
	AutoPairs   bool   // brackets and quotes typed get their other half (§7.9)
	theme       string // the name Parse read; Load finds the theme, as it touches the disk

	// Leader and Bindings are keymaps.toml's raw [keys] and [map.*]
	// entries; the keymap package gives them meaning.
	Leader   string
	Bindings []Binding
}

// Binding is one `key = "value"` line under [keys.<scope>] or [map.<...>].
type Binding struct {
	Table string // "keys.normal", "map.console.normal"
	Key   string // as written, e.g. "<C-p>"
	Value string // action for keys.*, key sequence for map.*; "" unbinds
}

func Default() *Config {
	return &Config{Theme: ui.TokyonightStorm, Icons: ui.NerdIcons, BaseIcons: ui.NerdIcons, ThemeName: "tokyonight-storm", Language: "zh", Vim: true, Timeoutlen: 1000, TabWidth: 2, ResultHeight: 0.4, MaxRows: 1000, KeywordCase: "lower", AutoPairs: true}
}

// Dir is $XDG_CONFIG_HOME/sqlmux, falling back to ~/.config/sqlmux on every
// Unix, macOS included (§14).
func Dir() string {
	if d := os.Getenv("XDG_CONFIG_HOME"); d != "" {
		return filepath.Join(d, "sqlmux")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "sqlmux")
}

// Load reads config.toml, then keymaps.toml; a missing file means its
// defaults.
func Load() (*Config, error) {
	c := Default()
	path := filepath.Join(Dir(), "config.toml")
	data, err := os.ReadFile(path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
	case err != nil:
		return nil, err
	default:
		if c, err = Parse(string(data)); err == nil && c.theme != "" {
			if c.Theme, c.Icons, err = LoadTheme(c.theme, c.BaseIcons); err != nil {
				err = fmt.Errorf("theme = %q: %w", c.theme, err)
			}
			c.ThemeName = c.theme
		}
		if err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
	}
	path = filepath.Join(Dir(), "keymaps.toml")
	data, err = os.ReadFile(path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
	case err != nil:
		return nil, err
	default:
		if c.Leader, c.Bindings, err = ParseKeys(string(data)); err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
	}
	return c, nil
}

// Parse reads config.toml's TOML over the defaults. It reads no other
// file: Load finds the theme it names, and the keys in keymaps.toml.
func Parse(data string) (*Config, error) {
	c := Default()
	raw := struct {
		Theme      *string  `toml:"theme"`
		Icons      *string  `toml:"icons"`
		Language   *string  `toml:"language"`
		Vim        *bool    `toml:"vim"`
		Timeoutlen *int     `toml:"timeoutlen"`
		TabWidth   *int     `toml:"tab_width"`
		Height     *float64 `toml:"result_height"`
		Case       *string  `toml:"keyword_case"`
		FormatPrg  string   `toml:"formatprg"`
		AutoPairs  *bool    `toml:"autopairs"`
		Console    struct {
			MaxRows *int `toml:"max_rows"`
		} `toml:"console"`
		Keys any `toml:"keys"`
		Map  any `toml:"map"`
	}{}
	if _, err := toml.Decode(data, &raw); err != nil {
		return nil, err
	}
	if raw.Keys != nil || raw.Map != nil { // F5.9 moved them
		return nil, errors.New("[keys] / [map] 要写在 keymaps.toml 里（sqlmux keys --format toml 生成一份完整的参考）")
	}
	if raw.Icons != nil {
		if *raw.Icons != "nerd" && *raw.Icons != "ascii" {
			return nil, fmt.Errorf(`icons = %q：只能是 "nerd" 或 "ascii"`, *raw.Icons)
		}
		c.Icons = ui.IconSet(*raw.Icons)
		c.BaseIcons = c.Icons
	}
	if raw.Language != nil {
		if *raw.Language != "zh" && *raw.Language != "en" {
			return nil, fmt.Errorf(`language = %q：只能是 "zh" 或 "en"`, *raw.Language)
		}
		c.Language = *raw.Language
	}
	if raw.Vim != nil {
		c.Vim = *raw.Vim
	}
	if raw.Theme != nil {
		c.theme = *raw.Theme
	}
	if raw.Timeoutlen != nil {
		if *raw.Timeoutlen <= 0 {
			return nil, fmt.Errorf("timeoutlen = %d：必须大于 0", *raw.Timeoutlen)
		}
		c.Timeoutlen = *raw.Timeoutlen
	}
	if raw.TabWidth != nil {
		if *raw.TabWidth <= 0 {
			return nil, fmt.Errorf("tab_width = %d：必须大于 0", *raw.TabWidth)
		}
		c.TabWidth = *raw.TabWidth
	}
	if h := raw.Height; h != nil {
		if *h <= 0 || *h >= 1 {
			return nil, fmt.Errorf("result_height = %v：要在 0 和 1 之间", *h)
		}
		c.ResultHeight = *h
	}
	if k := raw.Case; k != nil {
		if *k != "lower" && *k != "upper" && *k != "preserve" {
			return nil, fmt.Errorf(`keyword_case = %q：只能是 "lower"、"upper" 或 "preserve"`, *k)
		}
		c.KeywordCase = *k
	}
	c.FormatPrg = raw.FormatPrg
	if raw.AutoPairs != nil {
		c.AutoPairs = *raw.AutoPairs
	}
	if n := raw.Console.MaxRows; n != nil {
		if *n <= 0 {
			return nil, fmt.Errorf("[console] max_rows = %d：必须大于 0", *n)
		}
		c.MaxRows = *n
	}
	return c, nil
}

// ParseKeys reads keymaps.toml's TOML (§14): [keys]' leader, and the
// [keys.*] and [map.*] lines in file order, so "first binding" questions
// have a stable answer.
func ParseKeys(data string) (leader string, bindings []Binding, err error) {
	var raw map[string]any
	md, err := toml.Decode(data, &raw)
	if err != nil {
		return "", nil, err
	}
	for _, k := range md.Keys() {
		if k[0] != "keys" && k[0] != "map" {
			return "", nil, fmt.Errorf("%s：这里只能写 [keys] 和 [map.*]", k[0])
		}
		if md.Type(k...) == "Hash" {
			continue
		}
		v, ok := lookup(raw, k).(string)
		if !ok {
			return "", nil, fmt.Errorf("%s：值必须是字符串", k)
		}
		if k.String() == "keys.leader" {
			leader = v
			continue
		}
		if len(k) < 3 {
			return "", nil, fmt.Errorf("%s：键位要写在 [keys.normal]、[map.normal] 这样的表里", k)
		}
		bindings = append(bindings, Binding{strings.Join(k[:len(k)-1], "."), k[len(k)-1], v})
	}
	return leader, bindings, nil
}

// ThemeNames are the themes there are: the files of themes/ and the
// built-in ones, by name.
func ThemeNames() []string {
	names := slices.Collect(maps.Keys(ui.Themes))
	files, _ := filepath.Glob(filepath.Join(Dir(), "themes", "*.toml"))
	for _, f := range files {
		names = append(names, strings.TrimSuffix(filepath.Base(f), ".toml"))
	}
	slices.Sort(names)
	return slices.Compact(names)
}

// LoadTheme finds a theme by name (§7.3): themes/<name>.toml first, then a
// built-in theme; with the icons that `icons` chose, a theme's [icon] on
// them.
func LoadTheme(name string, icons *ui.Icons) (*ui.Theme, *ui.Icons, error) {
	path := filepath.Join(Dir(), "themes", name+".toml")
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		if th, ok := ui.Themes[name]; ok {
			return th, icons, nil
		}
		return nil, nil, fmt.Errorf(i18n.T("找不到 %s，也没有这个内置主题"), path)
	}
	if err != nil {
		return nil, nil, err
	}
	th, icons, err := ui.ParseTheme(string(data), icons)
	if err != nil {
		return nil, nil, fmt.Errorf("%s: %w", path, err)
	}
	return th, icons, nil
}

func lookup(m map[string]any, path []string) any {
	var v any = m
	for _, p := range path {
		v = v.(map[string]any)[p]
	}
	return v
}
