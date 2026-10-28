package main

import (
	"strings"
	"testing"

	"github.com/kackerx/sqlmux/internal/config"
	"github.com/kackerx/sqlmux/internal/db"
)

func TestSplitTable(t *testing.T) {
	for _, c := range []struct {
		in, schema, table string
	}{
		{"public.users", "public", "users"},
		{"users", "", "users"},
		{"a.b.c", "a", "b.c"},
		{".users", "", "users"},
		{"users.", "users", ""},
	} {
		schema, table := splitTable(c.in)
		if schema != c.schema || table != c.table {
			t.Errorf("splitTable(%q) = %q, %q; want %q, %q", c.in, schema, table, c.schema, c.table)
		}
	}
}

func TestParseOrder(t *testing.T) {
	for _, c := range []struct {
		in   string
		want config.LayoutOrder
	}{
		{"", nil},
		{"id", config.LayoutOrder{{Col: "id"}}},
		{"created_at desc, id", config.LayoutOrder{{Col: "created_at", Desc: true}, {Col: "id"}}},
		{"a DESC , b asc", config.LayoutOrder{{Col: "a", Desc: true}, {Col: "b"}}},
	} {
		got := parseOrder(c.in)
		if len(got) != len(c.want) {
			t.Errorf("parseOrder(%q) = %v; want %v", c.in, got, c.want)
			continue
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Errorf("parseOrder(%q)[%d] = %v; want %v", c.in, i, got[i], c.want[i])
			}
		}
	}
}

func TestSplitCSV(t *testing.T) {
	for _, c := range []struct {
		in   string
		want []string
	}{
		{"", nil},
		{"a", []string{"a"}},
		{"a, b ,c", []string{"a", "b", "c"}},
		{"a,,b", []string{"a", "b"}},
	} {
		got := splitCSV(c.in)
		if len(got) != len(c.want) {
			t.Errorf("splitCSV(%q) = %v; want %v", c.in, got, c.want)
			continue
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Errorf("splitCSV(%q)[%d] = %q; want %q", c.in, i, got[i], c.want[i])
			}
		}
	}
}

func TestTableFlagsValidate(t *testing.T) {
	for _, c := range []struct {
		label, name string
		tf          tableFlags
		want        string
	}{
		{"ok", "conn", tableFlags{table: "users"}, ""},
		{"cols and hide", "conn", tableFlags{table: "users", cols: "a", hide: "b"}, "cols or --hide"},
		{"no table", "conn", tableFlags{table: "users."}, "table name"},
		{"no connection", "", tableFlags{table: "users"}, "connection name"},
	} {
		err := c.tf.validate(c.name)
		switch {
		case c.want == "" && err != nil:
			t.Errorf("%s: unexpected error %v", c.label, err)
		case c.want != "" && (err == nil || !strings.Contains(err.Error(), c.want)):
			t.Errorf("%s: error %v; want it to contain %q", c.label, err, c.want)
		}
	}
}

func TestTableFlagsLayout(t *testing.T) {
	tf := tableFlags{
		table: "public.users", where: "age > 1", cols: "id,name", order: "id desc",
		limit: 500, zoom: true, readonly: true, transpose: true, sidebar: true, db: "play",
	}
	l := tf.layout("conn")
	if l.Version != config.LayoutVersion {
		t.Errorf("Version = %d; want %d", l.Version, config.LayoutVersion)
	}
	if len(l.Sessions) != 1 || l.Sessions[0].Connection != "conn" {
		t.Fatalf("session = %+v; want one on conn", l.Sessions)
	}
	w := l.Sessions[0].Windows[0]
	if w.Focus != 1 || w.Zoom != 1 || !w.SidebarOpen {
		t.Errorf("window focus/zoom/sidebar = %d/%d/%v; want 1/1/true", w.Focus, w.Zoom, w.SidebarOpen)
	}
	if len(w.Panes) != 1 || len(w.Panes[0].Tabs) != 1 {
		t.Fatalf("panes/tabs = %+v", w.Panes)
	}
	tab := w.Panes[0].Tabs[0]
	if tab.Kind != "table" || tab.Connection != "conn" || tab.DB != "play" ||
		tab.Schema != "public" || tab.Table != "users" || tab.Where != "age > 1" ||
		tab.Limit != 500 || tab.Transpose != true {
		t.Errorf("tab = %+v; want the flags' fields", tab)
	}
	if len(tab.Show) != 2 || tab.Show[0] != "id" || tab.Show[1] != "name" {
		t.Errorf("Show = %v; want [id name]", tab.Show)
	}
	if len(tab.Order) != 1 || tab.Order[0] != (db.OrderCol{Col: "id", Desc: true}) {
		t.Errorf("Order = %v; want [id desc]", tab.Order)
	}
}
