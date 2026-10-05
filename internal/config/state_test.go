package config

import (
	"bufio"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"slices"
	"testing"
	"time"
)

// state.json round-trips, is 0600, and an older snapshot never overwrites
// a newer one (§14); a table picked before its id named the connection is
// dropped (F5.10).
func TestState(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	if s, err := LoadState(); err != nil || len(s.Tables) != 0 {
		t.Fatalf("no file: %+v %v", s, err)
	}
	at := time.Date(2026, 9, 24, 14, 5, 0, 0, time.UTC)
	want := &State{
		Recent:   []Recent{{Kind: "table", ID: "doraemon/public.t_order"}, {Kind: "table", ID: "public.t_order"}, {Kind: "command", ID: "quit"}},
		SQL:      map[string][]string{"doraemon": {"select 2", "select 1"}},
		Consoles: map[string]ConsoleState{"console_1": {Conn: "doraemon", Schema: "agentable"}, "console_2": {Conn: "play"}},
		Tables: map[string]*TableState{"doraemon/public.t_order": {
			History:   []Query{{Where: "id > 5", At: at}},
			Favorites: []Query{{Where: "status = 'done'", Order: "amount", Desc: true, Limit: 500, At: at}},
		}},
	}
	older := Snapshot(&State{})
	if err := Snapshot(want)(); err != nil {
		t.Fatal(err)
	}
	if err := older(); err != nil { // an older snapshot, written late
		t.Fatal(err)
	}
	got, err := LoadState()
	want.Recent = slices.Delete(want.Recent, 1, 2)
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v %v", got, err)
	}
	info, _ := os.Stat(filepath.Join(StateDir(), "state.json"))
	if info.Mode().Perm() != 0o600 {
		t.Errorf("mode %v", info.Mode().Perm())
	}
	path := filepath.Join(StateDir(), "state.json")
	os.WriteFile(path, []byte("{"), 0o600)
	if s, err := LoadState(); err == nil || len(s.Tables) != 0 {
		t.Error("a broken file is an error, and an empty state")
	}
	if b, err := os.ReadFile(path + ".broken"); err != nil || string(b) != "{" {
		t.Errorf("the broken file is kept aside: %q %v", b, err)
	}
	if _, err := os.Stat(path); err == nil {
		t.Error("and out of the next save's way")
	}
}

// A process killed after the temp file is written and before it is renamed
// leaves the state.json there was whole (§14, F6.3).
func TestStateKilledMidWrite(t *testing.T) {
	if os.Getenv("SQLMUX_KILL_HELPER") != "" { // the child: write, and stop at the rename
		beforeRename = func() {
			os.Stdout.WriteString("ready\n")
			select {}
		}
		Snapshot(&State{Recent: []Recent{{Kind: "command", ID: "new"}}})()
		return
	}
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	old := &State{Recent: []Recent{{Kind: "command", ID: "old"}}}
	if err := Snapshot(old)(); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestStateKilledMidWrite$")
	cmd.Env = append(os.Environ(), "SQLMUX_KILL_HELPER=1")
	out, _ := cmd.StdoutPipe()
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	if line, _ := bufio.NewReader(out).ReadString('\n'); line != "ready\n" {
		t.Fatalf("the child said %q", line)
	}
	cmd.Process.Kill()
	cmd.Wait()
	got, err := LoadState()
	if err != nil || !reflect.DeepEqual(got, old) {
		t.Fatalf("after the kill: %+v %v", got, err)
	}
	if _, err := os.Stat(filepath.Join(StateDir(), "state.json.broken")); err == nil {
		t.Error("the file was moved aside")
	}
}
