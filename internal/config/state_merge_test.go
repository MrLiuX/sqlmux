package config

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
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

// A state.json that does not read is set aside, and this program's own state
// is what is written, whole: its unchanged entries are not lost with the file
// (F13.3).
func TestSnapshotOverABrokenFile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_STATE_HOME", dir)
	s, _ := LoadState()
	s.Queries = map[string]QueryState{"query_1": {Conn: "c"}}
	s.SQL = map[string][]string{"c": {"select 1"}}
	if err := Snapshot(s)(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(statePath(), []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	s.Queries["query_2"] = QueryState{Conn: "c"}
	if err := Snapshot(s)(); err != nil {
		t.Fatal(err)
	}
	got, err := LoadState()
	if err != nil || len(got.Queries) != 2 || len(got.SQL["c"]) != 1 {
		t.Errorf("after a broken file: %+v %v", got, err)
	}
	if b, err := os.ReadFile(statePath() + ".broken"); err != nil || string(b) != "{not json" {
		t.Errorf("set aside: %q %v", b, err)
	}
}

// A write waits for another program's: the lock is held across the read, the
// merge and the rename (F13.3).
func TestSnapshotWaitsForTheLock(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	unlock, err := lockFile(statePath() + ".lock")
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- Snapshot(&State{SQL: map[string][]string{"c": {"x"}}})() }()
	select {
	case <-done:
		t.Fatal("wrote while another held the lock")
	case <-time.After(300 * time.Millisecond):
	}
	unlock()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

// A null table of the file does not stop a merge, and the other program's
// newer is first (F13.3).
func TestMergeEdges(t *testing.T) {
	q := func(w string, at int) Query { return Query{Where: w, At: time.Unix(int64(at), 0)} }
	got := merge(&State{}, &State{Tables: map[string]*TableState{"t": {History: []Query{q("o", 20)}}}},
		&State{Tables: map[string]*TableState{"t": {History: []Query{q("d", 30)}}}})
	if h := got.Tables["t"].History; len(h) != 2 || h[0].Where != "d" { // the other's newer is first
		t.Errorf("history order: %+v", h)
	}
	got = merge(&State{}, &State{Tables: map[string]*TableState{"t": {History: []Query{q("o", 20)}}}}, &State{Tables: map[string]*TableState{"t": nil}})
	if got.Tables["t"] == nil || len(got.Tables["t"].History) != 1 {
		t.Errorf("a null table of the file: %+v", got.Tables)
	}
}

// What the file's writer dropped stays dropped, as what this program dropped:
// a favorite taken out there does not come back from here, nor a quick SQL
// cut, nor a pick; what both have, or only this program has, stays (F13.3).
func TestMergeKeepsTheirDrops(t *testing.T) {
	q := func(w string) Query { return Query{Where: w} }
	base := &State{
		Tables: map[string]*TableState{"t": {Favorites: []Query{q("f1"), q("f2")}, History: []Query{q("h1")}}},
		SQL:    map[string][]string{"c": {"s2", "s1"}},
		Recent: []Recent{{"table", "a"}, {"table", "b"}},
	}
	ours := &State{ // a WHERE run here, a quick SQL, a pick
		Tables: map[string]*TableState{"t": {Favorites: []Query{q("f1"), q("f2")}, History: []Query{q("h2"), q("h1")}}},
		SQL:    map[string][]string{"c": {"s3", "s2", "s1"}},
		Recent: []Recent{{"table", "c"}, {"table", "a"}, {"table", "b"}},
	}
	disk := &State{ // there f1 was unstarred, h1 cut, s1 cut, b dropped
		Tables: map[string]*TableState{"t": {Favorites: []Query{q("f2")}, History: nil}},
		SQL:    map[string][]string{"c": {"s2"}},
		Recent: []Recent{{"table", "a"}},
	}
	got := merge(base, ours, disk)
	var wheres []string
	for _, h := range got.Tables["t"].History {
		wheres = append(wheres, h.Where)
	}
	switch {
	case len(got.Tables["t"].Favorites) != 1 || got.Tables["t"].Favorites[0].Where != "f2":
		t.Errorf("favorites: %+v", got.Tables["t"].Favorites)
	case !slices.Equal(wheres, []string{"h2"}):
		t.Errorf("history: %v", wheres)
	case !slices.Equal(got.SQL["c"], []string{"s3", "s2"}):
		t.Errorf("sql: %v", got.SQL["c"])
	case !slices.Equal(got.Recent, []Recent{{"table", "c"}, {"table", "a"}}):
		t.Errorf("recent: %v", got.Recent)
	}
}

// Both programs add favorites, or change the same query: the favorites of both
// stay; a WHERE is the same one only with the same ORDER, DESC and LIMIT; the
// entry both changed is this program's; a WHERE both ran has the later time
// and the lists are cut at the limit (F13.3).
func TestMergeBothChanged(t *testing.T) {
	at := func(n int) time.Time { return time.Unix(int64(n), 0) }
	base := &State{Queries: map[string]QueryState{"query_1": {Conn: "c"}}}
	ours := &State{
		Tables:  map[string]*TableState{"t": {Favorites: []Query{{Where: "o"}}, History: []Query{{Where: "w", At: at(10)}, {Where: "w", Limit: 500, At: at(5)}}}},
		Queries: map[string]QueryState{"query_1": {Conn: "mine"}},
	}
	disk := &State{
		Tables:  map[string]*TableState{"t": {Favorites: []Query{{Where: "d"}}, History: []Query{{Where: "w", At: at(20)}}}},
		Queries: map[string]QueryState{"query_1": {Conn: "theirs"}},
	}
	got := merge(base, ours, disk)
	if len(got.Tables["t"].Favorites) != 2 {
		t.Errorf("favorites of both: %+v", got.Tables["t"].Favorites)
	}
	h := got.Tables["t"].History
	if len(h) != 2 || h[0].Limit != 0 || !h[0].At.Equal(at(20)) || h[1].Limit != 500 {
		t.Errorf("history: %+v", h)
	}
	if got.Queries["query_1"].Conn != "mine" {
		t.Errorf("queries: %+v", got.Queries)
	}
	var many []Query
	for i := range WhereRows {
		many = append(many, Query{Where: "o" + strconv.Itoa(i), At: at(i)})
	}
	var theirs []Query
	for i := range WhereRows {
		theirs = append(theirs, Query{Where: "d" + strconv.Itoa(i), At: at(1000 + i)})
	}
	got = merge(&State{}, &State{Tables: map[string]*TableState{"t": {History: many}}}, &State{Tables: map[string]*TableState{"t": {History: theirs}}})
	if n := len(got.Tables["t"].History); n != WhereRows || got.Tables["t"].History[0].Where != "d99" {
		t.Errorf("two full histories: %d, first %q", n, got.Tables["t"].History[0].Where)
	}
}
