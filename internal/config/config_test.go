package config

import (
	"os"
	"path/filepath"
	"reflect"
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
[console]
max_rows = 50
`)
	if err != nil {
		t.Fatal(err)
	}
	if c.Icons != ui.ASCIIIcons || c.Timeoutlen != 300 || c.TabWidth != 4 || c.ResultHeight != 0.3 || c.MaxRows != 50 || c.KeywordCase != "upper" || c.FormatPrg != "pg_format -" || c.AutoPairs {
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

// Keys belong in keymaps.toml, and only keys (F5.9).
func TestParseErrors(t *testing.T) {
	for _, s := range []string{
		`icons = "emoji"`,
		`timeoutlen = 0`,
		`tab_width = 0`,
		`result_height = 1`,
		`keyword_case = "title"`,
		`autopairs = "yes"`,
		"[console]\nmax_rows = 0",
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
	os.MkdirAll(filepath.Join(dir, "sqlmux"), 0o755)
	os.WriteFile(keys, []byte("[keys.grid]\nx = \"\""), 0o644)
	if c, err := Load(); err != nil || len(c.Bindings) != 1 {
		t.Fatalf("keymaps.toml alone: %+v %v", c, err)
	}
	os.WriteFile(conf, []byte(`icons = "ascii"`), 0o644)
	if c, err := Load(); err != nil || c.Icons != ui.ASCIIIcons || len(c.Bindings) != 1 {
		t.Fatalf("got %+v %v", c, err)
	}
	os.WriteFile(conf, []byte("[keys.grid]\nx = \"\""), 0o644)
	if _, err := Load(); err == nil || !strings.HasPrefix(err.Error(), conf+": ") || !strings.Contains(err.Error(), "keymaps.toml") {
		t.Errorf("keys in config.toml: %v", err)
	}
	os.WriteFile(conf, nil, 0o644)
	os.WriteFile(keys, []byte(`theme = "x"`), 0o644)
	if _, err := Load(); err == nil || !strings.HasPrefix(err.Error(), keys+": theme：") {
		t.Errorf("theme in keymaps.toml: %v", err)
	}
}

func TestThemeFile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	themes := filepath.Join(dir, "sqlmux", "themes")
	os.MkdirAll(themes, 0o755)
	os.WriteFile(filepath.Join(themes, "ristretto.toml"), []byte("pane_bg = \"#393333\"\n[icon]\nconsole = { text = \"C\" }"), 0o644)
	os.WriteFile(filepath.Join(themes, "typo.toml"), []byte(`rwo = "#393333"`), 0o644)
	load := func(config string) (*Config, error) {
		t.Helper()
		os.WriteFile(filepath.Join(dir, "sqlmux", "config.toml"), []byte(config), 0o644)
		return Load()
	}

	// [icon] lands on the set `icons` picks, wherever `icons` is written
	c, err := load("theme = \"ristretto\"\nicons = \"ascii\"")
	if err != nil {
		t.Fatal(err)
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
