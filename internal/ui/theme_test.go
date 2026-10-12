package ui

import (
	"reflect"
	"strings"
	"testing"
)

// Every icon and color token can be set from a theme file (§7.3, §7.7).
func TestThemeFileNamesEverything(t *testing.T) {
	icons, want := reflect.TypeOf(Icons{}), 0
	for i := range icons.NumField() {
		if icons.Field(i).Type == reflect.TypeOf(Icon{}) {
			want++
		}
	}
	if n := len(NerdIcons.byName()); n != want {
		t.Errorf("[icon] names %d of %d icons", n, want)
	}
	if n, want := len(TokyonightStorm.tokens()), reflect.TypeOf(Theme{}).NumField(); n != want {
		t.Errorf("theme files name %d of %d tokens", n, want)
	}
}

func TestParseTheme(t *testing.T) {
	th, ic, _, err := ParseTheme(`
row = "#6c6a6d"
string = "#FFD866"
match = "#00ff00"
visual = "#6c6a6d"
sql_column = "#010203"
[icon]
query = { text = "C", fg = "#ff0000" }
table = { fg = "#a9dc76" }
sort_asc = { fg = "#ff0000" }
`, ASCIIIcons)
	if err != nil {
		t.Fatal(err)
	}
	if th.Row != c("#6c6a6d") || th.String != c("#ffd866") || th.Match != c("#00ff00") || th.Visual != c("#6c6a6d") || th.SQLColumn != c("#010203") {
		t.Errorf("tokens not set: row %v string %v", th.Row, th.String)
	}
	if th.Bar != TokyonightStorm.Bar || th.Fg != TokyonightStorm.Fg {
		t.Error("tokens the file doesn't name must stay")
	}
	if ic.Query != (Icon{"C", c("#ff0000")}) || ic.Table != (Icon{"+", c("#a9dc76")}) || ic.Schema != ASCIIIcons.Schema || ic.SortAsc != (Icon{"↑", c("#ff0000")}) {
		t.Errorf("icons: query %v table %v schema %v", ic.Query, ic.Table, ic.Schema)
	}
	if ASCIIIcons.Query.Text != ">" || TokyonightStorm.Row == th.Row {
		t.Error("the built-in sets must not change")
	}
}

// The zoomed pane's border color is the theme's zoom, which a theme without one
// takes from its own warn, not from tokyonight's (F9.6).
func TestThemeZoomDefaultsToWarn(t *testing.T) {
	for _, tc := range []struct{ file, want string }{
		{"", "#e0af68"}, // tokyonight's own
		{`warn = "#112233"`, "#112233"},
		{`warn = "#112233"` + "\n" + `zoom = "#445566"`, "#445566"},
	} {
		th, _, _, err := ParseTheme(tc.file, ASCIIIcons)
		if err != nil || th.Zoom != c(tc.want) {
			t.Errorf("%q: zoom %v, want %s (%v)", tc.file, th.Zoom, tc.want, err)
		}
	}
}

// A color that is no #rrggbb is an error, naming the key.
func TestParseThemeErrors(t *testing.T) {
	for data, want := range map[string]string{
		`row = "red"`:                       "row",
		`row = 1`:                           "row",
		"[icon]\nquery = { fg = \"#f00\" }": "icon.query.fg",
	} {
		if _, _, _, err := ParseTheme(data, NerdIcons); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%q: error %v should name %s", data, err, want)
		}
	}
}

// A key it does not know, a token or an icon or a field of one that a later
// version has, loads and is told, sorted; the rest of the file is read (F14.7).
func TestParseThemeUnknown(t *testing.T) {
	th, ic, unknown, err := ParseTheme(`
selectd = "#ffffff"
row = "#6c6a6d"
rwo = "#000000"
[icon]
consol = { text = "C" }
query = { text = "Q", bg = "#ff0000" }
`, NerdIcons)
	if err != nil {
		t.Fatal(err)
	}
	if want := "icon.consol icon.query.bg rwo selectd"; strings.Join(unknown, " ") != want {
		t.Errorf("unknown %q, want %q", unknown, want)
	}
	if th.Row != c("#6c6a6d") || ic.Query.Text != "Q" {
		t.Errorf("the rest is read: row %v, query %v", th.Row, ic.Query)
	}
}
