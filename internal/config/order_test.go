package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/kackerx/sqlmux/internal/db"
)

// A table tab's ORDER is written as an array and read back, written as tables
// or inline; a layout of before F12.1 has order = "col" with desc beside it,
// which is one column, and writes back as the array (F12.1).
func TestLayoutOrder(t *testing.T) {
	layout := func(tab string) *LayoutTab {
		path := filepath.Join(t.TempDir(), "l.toml")
		text := "version = 1\n[[session]]\nname = \"1\"\nconnection = \"c\"\n[[session.window]]\nname = \"w\"\n[[session.window.pane]]\nid = 1\n[[session.window.pane.tab]]\nkind = \"table\"\ntable = \"t\"\n" + tab
		if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
		l, err := LoadLayout(path)
		if err != nil {
			t.Fatalf("%s: %v", tab, err)
		}
		return &l.Sessions[0].Windows[0].Panes[0].Tabs[0]
	}
	two := LayoutOrder{{Col: "source", Desc: true}, {Col: "id"}}
	for name, c := range map[string]struct {
		tab  string
		want LayoutOrder
		desc bool
	}{
		"inline":       {"order = [{col = \"source\", desc = true}, {col = \"id\"}]\n", two, false},
		"tables":       {"[[session.window.pane.tab.order]]\ncol = \"source\"\ndesc = true\n[[session.window.pane.tab.order]]\ncol = \"id\"\n", two, false},
		"old, desc":    {"order = \"amount\"\ndesc = true\n", LayoutOrder{{Col: "amount", Desc: true}}, false},
		"old, asc":     {"order = \"amount\"\n", LayoutOrder{{Col: "amount"}}, false},
		"default desc": {"desc = true\n", nil, true},
		"empty":        {"order = []\n", nil, false},
		"old, default": {"order = \"\"\ndesc = true\n", nil, true},
		"two and desc": {"order = [{col = \"source\", desc = true}, {col = \"id\"}]\ndesc = true\n", two, true}, // not the old format: it stays two
	} {
		tab := layout(c.tab)
		if len(tab.Order) != len(c.want) || (len(c.want) > 0 && !reflect.DeepEqual(tab.Order, c.want)) || tab.Desc != c.desc {
			t.Errorf("%s: %+v desc %v, want %+v desc %v", name, tab.Order, tab.Desc, c.want, c.desc)
		}
	}
	for _, bad := range []string{"order = [{col = 1}]\n", "order = [{desc = true}]\n", "order = [{col = \"\"}]\n", "order = [{col = \"a\", desc = \"yes\"}]\n", "order = [\"a\"]\n"} {
		path := filepath.Join(t.TempDir(), "bad.toml")
		text := "version = 1\n[[session]]\nname = \"1\"\nconnection = \"c\"\n[[session.window]]\nname = \"w\"\n[[session.window.pane]]\nid = 1\n[[session.window.pane.tab]]\nkind = \"table\"\ntable = \"t\"\n" + bad
		if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := LoadLayout(path); err == nil {
			t.Errorf("%s: read", bad)
		}
	}
	path := filepath.Join(t.TempDir(), "out.toml")
	if err := SaveLayout(path, &Layout{Sessions: []LayoutSession{{Name: "1", Connection: "c", Windows: []LayoutWindow{{Name: "w", Panes: []LayoutPane{{ID: 1, Tabs: []LayoutTab{{Kind: "table", Table: "t", Order: two}}}}}}}}}); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(path); !strings.Contains(string(data), "col = \"source\"") || strings.Contains(string(data), "desc = false") {
		t.Errorf("written as\n%s", data)
	}
	back, err := LoadLayout(path)
	if err != nil || !reflect.DeepEqual(back.Sessions[0].Windows[0].Panes[0].Tabs[0].Order, two) {
		t.Errorf("read back: %+v %v", back, err)
	}
}

// A WHERE in the history of before F12.1 says "order": "col" with "desc"
// beside it (F12.1).
func TestQueryJSONOrder(t *testing.T) {
	for in, want := range map[string]Query{
		`{"where":"a","order":"amount","desc":true,"limit":500}`:       {Where: "a", Order: []db.OrderCol{{Col: "amount", Desc: true}}, Limit: 500},
		`{"where":"a","order":"amount"}`:                               {Where: "a", Order: []db.OrderCol{{Col: "amount"}}},
		`{"where":"a","desc":true}`:                                    {Where: "a", Desc: true},
		`{"where":"a","order":[{"col":"s","desc":true},{"col":"id"}]}`: {Where: "a", Order: []db.OrderCol{{Col: "s", Desc: true}, {Col: "id"}}},
	} {
		var got Query
		if err := json.Unmarshal([]byte(in), &got); err != nil || !reflect.DeepEqual(got, want) {
			t.Errorf("%s: %+v %v, want %+v", in, got, err, want)
		}
	}
	out, _ := json.Marshal(Query{Where: "a", Order: []db.OrderCol{{Col: "s", Desc: true}}})
	if string(out) != `{"where":"a","order":[{"col":"s","desc":true}],"at":"0001-01-01T00:00:00Z"}` {
		t.Errorf("written as %s", out)
	}
}
