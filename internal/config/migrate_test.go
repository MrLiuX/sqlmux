package config

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func writeFiles(t *testing.T, dir string, names ...string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	for _, n := range names {
		if err := os.WriteFile(filepath.Join(dir, n), []byte(n), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

func listed(t *testing.T, dir string) []string {
	t.Helper()
	es, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, e := range es {
		out = append(out, e.Name())
	}
	return out
}

// consoles/ becomes queries/ and console_<n>.sql in it query_<n>.sql, a
// renamed file or one that only starts like one staying; done once (F10.11).
func TestMigrateConsoles(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	writeFiles(t, filepath.Join(DataDir(), "consoles"), "console_2.sql", "mine.sql", "console_x.sql", "console_3.sql")
	if err := MigrateConsoles(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(DataDir(), "consoles")); err == nil {
		t.Error("consoles/ is still there")
	}
	want := []string{"console_x.sql", "mine.sql", "query_2.sql", "query_3.sql"}
	if got := listed(t, filepath.Join(DataDir(), "queries")); !slices.Equal(got, want) {
		t.Errorf("queries/: %v, want %v", got, want)
	}
	if text, _ := ReadQuery(QueryPath("query_2")); text != "console_2.sql" {
		t.Errorf("query_2 is %q", text)
	}
	if err := MigrateConsoles(); err != nil {
		t.Errorf("again: %v", err)
	}
}

// queries/ there is left alone and consoles/ not merged into it; a file of
// the new name there already is not written over (F10.11).
func TestMigrateConsolesKeepsWhatIsThere(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	writeFiles(t, filepath.Join(DataDir(), "consoles"), "console_1.sql")
	writeFiles(t, filepath.Join(DataDir(), "queries"), "query_5.sql")
	if err := MigrateConsoles(); err != nil {
		t.Fatal(err)
	}
	if got := listed(t, filepath.Join(DataDir(), "consoles")); !slices.Equal(got, []string{"console_1.sql"}) {
		t.Errorf("consoles/: %v", got)
	}
	if got := listed(t, filepath.Join(DataDir(), "queries")); !slices.Equal(got, []string{"query_5.sql"}) {
		t.Errorf("queries/: %v", got)
	}

	t.Setenv("XDG_DATA_HOME", t.TempDir())
	writeFiles(t, filepath.Join(DataDir(), "consoles"), "console_1.sql", "query_1.sql")
	if err := MigrateConsoles(); err != nil {
		t.Fatal(err)
	}
	if text, _ := ReadQuery(QueryPath("query_1")); text != "query_1.sql" {
		t.Errorf("query_1 was written over: %q", text)
	}
	if got := listed(t, filepath.Join(DataDir(), "queries")); !slices.Equal(got, []string{"console_1.sql", "query_1.sql"}) {
		t.Errorf("queries/: %v", got)
	}
}

// A state.json of before F10.11 says consoles, console_<n> by file name; it
// is read as queries, what is under the new name already kept (F10.11).
func TestLoadStateConsoles(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	writeFiles(t, StateDir())
	old := `{"consoles":{"console_1":{"conn":"a","schema":"s"},"mine":{"conn":"b"},"console_2":{"conn":"old"}},"queries":{"query_2":{"conn":"new"}}}`
	if err := os.WriteFile(statePath(), []byte(old), 0o600); err != nil {
		t.Fatal(err)
	}
	s, err := LoadState()
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]QueryState{"query_1": {Conn: "a", Schema: "s"}, "mine": {Conn: "b"}, "query_2": {Conn: "new"}}
	if len(s.Queries) != 3 || s.Queries["query_1"] != want["query_1"] || s.Queries["mine"] != want["mine"] || s.Queries["query_2"] != want["query_2"] {
		t.Errorf("queries: %v, want %v", s.Queries, want)
	}
	if err := Snapshot(s)(); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(statePath()); !strings.Contains(string(data), `"queries":{`) || strings.Contains(string(data), "console") {
		t.Errorf("written as %s", data)
	}
}

// A layout of before F10.11 says kind = "console" and console_<n>: read as
// queries, not written back as it was (F10.11).
func TestLoadLayoutConsoles(t *testing.T) {
	path := filepath.Join(t.TempDir(), "old.toml")
	old := `version = 1
[[session]]
name = "1"
connection = "doraemon"
[[session.window]]
name = "data"
[[session.window.pane]]
id = 1
[[session.window.pane.tab]]
kind = "console"
name = "console_4"
[[session.window.pane.tab]]
kind = "console"
name = "report"
[[session.window.pane.tab]]
kind = "console"
n = 2
`
	if err := os.WriteFile(path, []byte(old), 0o600); err != nil {
		t.Fatal(err)
	}
	l, err := LoadLayout(path)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, tb := range l.Sessions[0].Windows[0].Panes[0].Tabs {
		got = append(got, tb.Kind+" "+tb.QueryName())
	}
	if want := []string{"query query_4", "query report", "query query_2"}; !slices.Equal(got, want) {
		t.Errorf("tabs %v, want %v", got, want)
	}
}
