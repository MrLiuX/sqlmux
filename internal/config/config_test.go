package config

import (
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"sqlmux/internal/ui"
)

func TestParse(t *testing.T) {
	c, err := Parse(`
icons = "ascii"
timeoutlen = 300
tab_width = 4
result_height = 0.3
keyword_case = "upper"
formatprg = "pg_format -"
autopairs = false
confirm_dangerous = false
[console]
max_rows = 50
[grid]
full_width = 12
`)
	if err != nil {
		t.Fatal(err)
	}
	if c.Icons != ui.ASCIIIcons || c.Timeoutlen != 300 || c.TabWidth != 4 || c.ResultHeight != 0.3 || c.MaxRows != 50 || c.GridFullWidth != 12 || c.KeywordCase != "upper" || c.FormatPrg != "pg_format -" || c.AutoPairs || c.ConfirmDangerous {
		t.Fatalf("got %+v", c)
	}
}

func TestParseKeysInFileOrder(t *testing.T) {
	leader, bs, err := ParseKeys(`
[keys]
leader = "<C-a>"
[keys.normal]
"<C-w>v" = "pane.split.right"
x = ""
[map.normal]
J = "5j"
[map.console.normal]
L = "5l"
`)
	want := []Binding{
		{"keys.normal", "<C-w>v", "pane.split.right"},
		{"keys.normal", "x", ""},
		{"map.normal", "J", "5j"},
		{"map.console.normal", "L", "5l"},
	}
	if err != nil || leader != "<C-a>" || !reflect.DeepEqual(bs, want) {
		t.Fatalf("leader %q, bindings %+v, %v", leader, bs, err)
	}
}

// [grid] order and transpose are what a table opened anew starts with
// (F8.8); order is "asc" or "desc".
func TestGridNewTableDefaults(t *testing.T) {
	if c, err := Parse(""); err != nil || c.GridDesc || c.GridTranspose {
		t.Errorf("default: %+v %v", c, err)
	}
	if c, err := Parse("[grid]\norder = \"desc\"\ntranspose = true"); err != nil || !c.GridDesc || !c.GridTranspose {
		t.Errorf("desc, transpose: %+v %v", c, err)
	}
	if c, err := Parse("[grid]\norder = \"asc\""); err != nil || c.GridDesc {
		t.Errorf("asc: %+v %v", c, err)
	}
	for _, s := range []string{"[grid]\norder = \"down\"", "[grid]\ntranspose = \"yes\""} {
		if _, err := Parse(s); err == nil {
			t.Errorf("%q: expected an error", s)
		}
	}
}

// border names the boxes' lines; another name is an error that lists them
// (F8.10).
func TestBorder(t *testing.T) {
	single, _ := ui.BorderOf("single")
	if c, err := Parse(""); err != nil || !reflect.DeepEqual(c.Border, single) {
		t.Errorf("default: %+v %v", c.Border, err)
	}
	for _, b := range []string{"single", "rounded", "double", "bold", "hidden"} {
		want, _ := ui.BorderOf(b)
		if c, err := Parse(`border = "` + b + `"`); err != nil || !reflect.DeepEqual(c.Border, want) {
			t.Errorf("%s: %v", b, err)
		}
	}
	if _, err := Parse(`border = "dotted"`); err == nil || err.Error() != `border = "dotted": must be "single", "rounded", "double", "bold" or "hidden"` {
		t.Errorf("dotted: %v", err)
	}
}

func TestGridFullWidthDefault(t *testing.T) {
	if c, err := Parse(""); err != nil || c.GridFullWidth != 22 {
		t.Errorf("default: %d %v", c.GridFullWidth, err)
	}
}

// Keys belong in keymaps.toml, and only keys (F5.9).
func TestParseErrors(t *testing.T) {
	for _, s := range []string{
		`icons = "emoji"`,
		`timeoutlen = 0`,
		`tab_width = 0`,
		`result_height = 1`,
		`keyword_case = "title"`,
		`autopairs = "yes"`,
		`confirm_dangerous = "no"`,
		"[console]\nmax_rows = 0",
		"[grid]\nfull_width = 0",
		"[keys]\nleader = \"<C-a>\"",
		"[map.normal]\nJ = \"5j\"",
		"[keys.normal\n",
	} {
		if _, err := Parse(s); err == nil {
			t.Errorf("config.toml %q: expected an error", s)
		}
	}
	for _, s := range []string{
		"[keys.normal]\nx = 1",
		"[keys]\nx = \"a\"",
		`theme = "x"`,
		"[console]\nmax_rows = 50",
		"[keys.normal\n",
	} {
		if _, _, err := ParseKeys(s); err == nil {
			t.Errorf("keymaps.toml %q: expected an error", s)
		}
	}
}

// Load reads config.toml and keymaps.toml, each optional; an error names
// its file, and one with keys in config.toml says where they go (F5.9).
func TestLoadUsesXDGConfigHome(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	if c, err := Load(); err != nil || c.Icons != ui.NerdIcons {
		t.Fatalf("missing files: %+v %v", c, err)
	}
	conf, keys := filepath.Join(dir, "sqlmux", "config.toml"), filepath.Join(dir, "sqlmux", "keymaps.toml")
	if err := os.MkdirAll(filepath.Join(dir, "sqlmux"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keys, []byte("[keys.grid]\nx = \"\""), 0o644); err != nil {
		t.Fatal(err)
	}
	if c, err := Load(); err != nil || len(c.Bindings) != 1 {
		t.Fatalf("keymaps.toml alone: %+v %v", c, err)
	}
	if err := os.WriteFile(conf, []byte(`icons = "ascii"`), 0o644); err != nil {
		t.Fatal(err)
	}
	if c, err := Load(); err != nil || c.Icons != ui.ASCIIIcons || len(c.Bindings) != 1 {
		t.Fatalf("got %+v %v", c, err)
	}
	if err := os.WriteFile(conf, []byte("[keys.grid]\nx = \"\""), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(); err == nil || !strings.HasPrefix(err.Error(), conf+": ") || !strings.Contains(err.Error(), "keymaps.toml") {
		t.Errorf("keys in config.toml: %v", err)
	}
	if err := os.WriteFile(conf, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keys, []byte(`theme = "x"`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(); err == nil || !strings.HasPrefix(err.Error(), keys+": theme: ") {
		t.Errorf("theme in keymaps.toml: %v", err)
	}
}

func TestThemeFile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	themes := filepath.Join(dir, "sqlmux", "themes")
	if err := os.MkdirAll(themes, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(themes, "ristretto.toml"), []byte("pane_bg = \"#393333\"\n[icon]\nconsole = { text = \"C\" }"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(themes, "typo.toml"), []byte(`rwo = "#393333"`), 0o644); err != nil {
		t.Fatal(err)
	}
	load := func(config string) (*Config, error) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, "sqlmux", "config.toml"), []byte(config), 0o644); err != nil {
			t.Fatal(err)
		}
		return Load()
	}

	// [icon] lands on the set `icons` picks, wherever `icons` is written
	c, err := load("theme = \"ristretto\"\nicons = \"ascii\"")
	if err != nil {
		t.Fatal(err)
	}
	if names := ThemeNames(); !slices.Equal(names, []string{"ristretto", "tokyonight-storm", "typo"}) || c.ThemeName != "ristretto" || c.BaseIcons == c.Icons {
		t.Errorf("themes %v, in use %q, base icons kept apart: %v", names, c.ThemeName, c.BaseIcons != c.Icons)
	}
	if c.Theme.PaneBg == ui.TokyonightStorm.PaneBg || c.Theme.Fg != ui.TokyonightStorm.Fg {
		t.Errorf("pane_bg %v fg %v", c.Theme.PaneBg, c.Theme.Fg)
	}
	if c.Icons.Console.Text != "C" || c.Icons.Table != ui.ASCIIIcons.Table {
		t.Errorf("icons %+v", c.Icons)
	}
	if c, err := load(`theme = "tokyonight-storm"`); err != nil || c.Theme != ui.TokyonightStorm {
		t.Errorf("built-in: %v", err)
	}
	for name, want := range map[string]string{"nope": "nope.toml", "typo": "rwo"} {
		if _, err := load(`theme = "` + name + `"`); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("theme %s: error %v should name %s", name, err, want)
		}
	}
	if c, err := Parse(`theme = "nope"`); err != nil || c.Theme != ui.TokyonightStorm {
		t.Errorf("Parse reads no theme file: %v", err)
	}
}

// language is "en" or "zh", en when not written; another one is an error
// that says, in English, what is allowed (§14「界面语言」, F7.6).
func TestLanguage(t *testing.T) {
	if c, err := Parse(""); err != nil || c.Language != "en" {
		t.Errorf("default: %q %v", c.Language, err)
	}
	if c, err := Parse(`language = "zh"`); err != nil || c.Language != "zh" {
		t.Errorf("zh: %v", err)
	}
	if _, err := Parse(`language = "fr"`); err == nil || err.Error() != `language = "fr": must be "en" or "zh"` {
		t.Errorf("fr: %v", err)
	}
}

// vim is true unless it is written false (§6.9).
func TestVimOption(t *testing.T) {
	if c, err := Parse(""); err != nil || !c.Vim {
		t.Errorf("default: %v %v", c.Vim, err)
	}
	if c, err := Parse("vim = false"); err != nil || c.Vim {
		t.Errorf("false: %v %v", c.Vim, err)
	}
	if _, err := Parse(`vim = "no"`); err == nil {
		t.Error("a string is not a bool")
	}
}

// The confirmation of dangerous statements is on unless config.toml turns it
// off (§13, F6.7).
func TestConfirmDangerousDefault(t *testing.T) {
	if c, err := Parse(""); err != nil || !c.ConfirmDangerous {
		t.Errorf("default: %v %v", c, err)
	}
	if c, err := Parse("confirm_dangerous = false"); err != nil || c.ConfirmDangerous {
		t.Errorf("off: %v %v", c, err)
	}
}

func TestTreeWidth(t *testing.T) {
	if c, err := Parse(""); err != nil || c.TreeWidth != 32 {
		t.Errorf("default: %d %v", c.TreeWidth, err)
	}
	if c, err := Parse("[tree]\nwidth = 38"); err != nil || c.TreeWidth != 38 {
		t.Errorf("38: %d %v", c.TreeWidth, err)
	}
	if _, err := Parse("[tree]\nwidth = 0"); err == nil {
		t.Error("0 accepted")
	}
}
