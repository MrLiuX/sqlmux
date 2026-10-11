package config

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"testing"
	"time"
)

// Two processes (this test binary run twice) load state.json at start, each
// records a WHERE, each writes its snapshot: the file keeps both (F13.3).
func TestTwoProcessesState(t *testing.T) {
	if who := os.Getenv("SQLMUX_TEST_STATE_CHILD"); who != "" {
		dir := os.Getenv("SQLMUX_TEST_STATE_DIR")
		s, err := LoadState()
		if err != nil {
			panic(err)
		}
		_ = os.WriteFile(filepath.Join(dir, "loaded-"+who), nil, 0o600)
		for {
			if _, err := os.Stat(filepath.Join(dir, "go-"+who)); err == nil {
				break
			}
			time.Sleep(10 * time.Millisecond)
		}
		if s.Tables == nil {
			s.Tables = map[string]*TableState{}
		}
		s.Tables["c/public."+who] = &TableState{History: []Query{{Where: "id = 1"}}}
		if err := Snapshot(s)(); err != nil {
			panic(err)
		}
		return
	}
	dir := t.TempDir()
	t.Setenv("XDG_STATE_HOME", dir)
	if err := Snapshot(&State{SQL: map[string][]string{"c": {"select 1"}}})(); err != nil {
		t.Fatal(err)
	}
	start := func(who string) *exec.Cmd {
		c := exec.Command(os.Args[0], "-test.run=^TestTwoProcessesState$")
		c.Env = append(os.Environ(), "SQLMUX_TEST_STATE_CHILD="+who, "SQLMUX_TEST_STATE_DIR="+dir)
		if err := c.Start(); err != nil {
			t.Fatal(err)
		}
		return c
	}
	wait := func(f string) {
		for range 300 {
			if _, err := os.Stat(filepath.Join(dir, f)); err == nil {
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
		t.Fatal("no " + f)
	}
	a, b := start("a"), start("b")
	wait("loaded-a")
	wait("loaded-b")
	_ = os.WriteFile(filepath.Join(dir, "go-a"), nil, 0o600)
	_ = a.Wait()
	_ = os.WriteFile(filepath.Join(dir, "go-b"), nil, 0o600)
	_ = b.Wait()
	s, err := LoadState()
	if err != nil {
		t.Fatal(err)
	}
	if s.Tables["c/public.a"] == nil || s.Tables["c/public.b"] == nil || len(s.SQL["c"]) != 1 {
		t.Errorf("one process's WHERE history is gone: %+v", s)
	}
}

// What a program changed is its own, what it left alone the file's, what it
// dropped stays dropped, and a list both added to has both (F13.3).
func TestMergeState(t *testing.T) {
	q := func(w string) Query { return Query{Where: w} }
	base := &State{
		Tables:  map[string]*TableState{"c/p.t": {History: []Query{q("a = 1")}}, "c/p.old": {History: []Query{q("x")}}, "c/p.kept": {History: []Query{q("k")}}},
		SQL:     map[string][]string{"c": {"s3", "s2", "s1"}},
		Queries: map[string]QueryState{"query_1": {Conn: "c"}, "query_2": {Conn: "c"}},
		Recent:  []Recent{{"table", "c/p.t"}, {"table", "c/p.old"}},
	}
	// this program: a WHERE on t, c/p.old and query_2 dropped, the oldest quick SQL trimmed, a new one, the old table pick gone
	ours := &State{
		Tables:  map[string]*TableState{"c/p.t": {History: []Query{q("a = 2"), q("a = 1")}}, "c/p.kept": {History: []Query{q("k")}}},
		SQL:     map[string][]string{"c": {"s4", "s3", "s2"}},
		Queries: map[string]QueryState{"query_1": {Conn: "c"}},
		Recent:  []Recent{{"table", "c/p.t"}},
	}
	// the file, as another program left it: its own WHERE on t, a table of its own, query_1 moved, kept changed, a pick
	disk := &State{
		Tables: map[string]*TableState{
			"c/p.t":    {History: []Query{q("a = 3"), q("a = 1")}},
			"c/p.old":  {History: []Query{q("x")}},
			"c/p.new":  {History: []Query{q("n")}},
			"c/p.kept": {History: []Query{q("k2"), q("k")}},
		},
		SQL:     map[string][]string{"c": {"s5", "s3", "s2", "s1"}},
		Queries: map[string]QueryState{"query_1": {Conn: "d"}, "query_2": {Conn: "c"}, "query_3": {Conn: "c"}},
		Recent:  []Recent{{"table", "c/p.new"}, {"table", "c/p.t"}, {"table", "c/p.old"}},
	}
	has := func(m map[string]*TableState, k string) bool { _, ok := m[k]; return ok }
	got := merge(base, ours, disk)
	where := func(h []Query) (out []string) {
		for _, x := range h {
			out = append(out, x.Where)
		}
		return out
	}
	switch {
	case !slices.Equal(where(got.Tables["c/p.t"].History), []string{"a = 2", "a = 1", "a = 3"}): // both added: ours first
		t.Errorf("t: %v", where(got.Tables["c/p.t"].History))
	case has(got.Tables, "c/p.old") || !has(got.Tables, "c/p.new"): // dropped stays, another's new comes
		t.Errorf("tables %v", got.Tables)
	case !slices.Equal(where(got.Tables["c/p.kept"].History), []string{"k2", "k"}): // left alone: the file's
		t.Errorf("kept: %v", where(got.Tables["c/p.kept"].History))
	case !slices.Equal(got.SQL["c"], []string{"s4", "s3", "s2", "s5"}): // s1 was trimmed here, s5 is another's
		t.Errorf("sql: %v", got.SQL["c"])
	case got.Queries["query_1"].Conn != "d" || got.Queries["query_2"].Conn != "" || got.Queries["query_3"].Conn != "c": // query_1 left alone here: the other's move
		t.Errorf("queries: %v", got.Queries)
	case !slices.Equal(got.Recent, []Recent{{"table", "c/p.t"}, {"table", "c/p.new"}}): // the pick dropped here stays dropped
		t.Errorf("recent: %v", got.Recent)
	}
	// a list left alone here is the file's, as another program trimmed it
	if got := merge(base, &State{Recent: base.Recent}, &State{Recent: []Recent{{"table", "c/p.t"}}}); !slices.Equal(got.Recent, []Recent{{"table", "c/p.t"}}) {
		t.Errorf("recent left alone: %v", got.Recent)
	}
}

// After a write the state it was told apart by is the one written: what is
// dropped next is dropped from the file, not found there again (F13.3).
func TestSnapshotKeepsDropping(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	s, err := LoadState()
	if err != nil {
		t.Fatal(err)
	}
	s.Queries = map[string]QueryState{"query_1": {Conn: "c"}, "query_2": {Conn: "c"}}
	if err := Snapshot(s)(); err != nil {
		t.Fatal(err)
	}
	delete(s.Queries, "query_2")
	if err := Snapshot(s)(); err != nil {
		t.Fatal(err)
	}
	got, err := LoadState()
	if err != nil || len(got.Queries) != 1 {
		t.Errorf("queries after a drop: %v %v", got.Queries, err)
	}
}
