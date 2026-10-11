package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/kackerx/sqlmux/internal/db"
)

// State is what the app keeps between runs (§14), in state.json: the
// palette's recent picks, each table's WHERE history and favorites, each
// connection's quick SQL history, and the connection and schema each
// query file last ran on.
type State struct {
	Recent  []Recent               `json:"recent,omitempty"`
	Tables  map[string]*TableState `json:"tables,omitempty"`  // by "<connection>/<schema>.<table>"
	SQL     map[string][]string    `json:"sql,omitempty"`     // by connection, newest first (§12)
	Queries map[string]QueryState  `json:"queries,omitempty"` // by file name: "query_1" (§11)
}

// RenameConnection makes what the state keeps by the connection called old
// kept by to: the table picks and table keys that start with its name, its
// quick SQL, the queries that run on it (F8.16). Data under the new name
// from a connection of that name since deleted is written over by the
// renamed one's, a pick in both once. The layout files are the user's, and
// say the old name still.
func (s *State) RenameConnection(old, to string) {
	prefix := old + "/"
	for i, r := range s.Recent {
		if id, ok := strings.CutPrefix(r.ID, prefix); ok && r.Kind == "table" {
			s.Recent[i].ID = to + "/" + id
		}
	}
	seen := map[Recent]bool{}
	s.Recent = slices.DeleteFunc(s.Recent, func(r Recent) bool { dup := seen[r]; seen[r] = true; return dup })
	for k, v := range s.Tables {
		if rest, ok := strings.CutPrefix(k, prefix); ok {
			delete(s.Tables, k)
			s.Tables[to+"/"+rest] = v
		}
	}
	if sql, ok := s.SQL[old]; ok {
		delete(s.SQL, old)
		s.SQL[to] = sql
	}
	for k, c := range s.Queries {
		if c.Conn == old {
			c.Conn = to
			s.Queries[k] = c
		}
	}
}

// QueryState is where a query runs: its connection, by name, and its
// schema (§8.6).
type QueryState struct {
	Conn   string `json:"conn"`
	Schema string `json:"schema,omitempty"`
}

// Recent is one palette pick (§12): its kind and its id within the kind.
type Recent struct {
	Kind string `json:"kind"`
	ID   string `json:"id"`
}

type TableState struct {
	History   []Query `json:"history,omitempty"` // newest first
	Favorites []Query `json:"favorites,omitempty"`
}

// Query is a WHERE as it ran, with the ORDER and LIMIT it ran with (Q-02).
// Desc is the default order turned, with no Order.
type Query struct {
	Where string        `json:"where"`
	Order []db.OrderCol `json:"order,omitempty"`
	Desc  bool          `json:"desc,omitempty"`
	Limit int           `json:"limit,omitempty"`
	At    time.Time     `json:"at"`
}

// UnmarshalJSON reads what a state.json of before F12.1 says, "order": "col"
// with "desc" beside it, as one column.
func (q *Query) UnmarshalJSON(data []byte) error {
	var raw struct {
		Where string          `json:"where"`
		Order json.RawMessage `json:"order"`
		Desc  bool            `json:"desc"`
		Limit int             `json:"limit"`
		At    time.Time       `json:"at"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	*q = Query{Where: raw.Where, Desc: raw.Desc, Limit: raw.Limit, At: raw.At}
	var col string
	switch {
	case len(raw.Order) == 0:
	case json.Unmarshal(raw.Order, &col) == nil:
		if col != "" {
			q.Order, q.Desc = []db.OrderCol{{Col: col, Desc: raw.Desc}}, false
		}
	default:
		if err := json.Unmarshal(raw.Order, &q.Order); err != nil {
			return err
		}
	}
	return nil
}

// StateDir is $XDG_STATE_HOME/sqlmux, falling back to ~/.local/state/sqlmux (§14).
func StateDir() string {
	if d := os.Getenv("XDG_STATE_HOME"); d != "" {
		return filepath.Join(d, "sqlmux")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "state", "sqlmux")
}

func statePath() string { return filepath.Join(StateDir(), "state.json") }

// LoadState reads state.json; a missing file is an empty state. One that
// doesn't read is moved to state.json.broken, so the next save can't
// overwrite what the user had, and the state starts empty (§14). A table
// picked before its id named the connection (F5.10) is dropped.
func LoadState() (*State, error) {
	data, err := os.ReadFile(statePath())
	if errors.Is(err, fs.ErrNotExist) {
		return &State{}, nil
	}
	s := &State{}
	if err == nil {
		err = json.Unmarshal(data, s)
	}
	if err != nil {
		if rerr := os.Rename(statePath(), statePath()+".broken"); rerr != nil { // the next save will overwrite it
			err = errors.Join(err, rerr)
		}
		return &State{}, fmt.Errorf("%s: %w", statePath(), err)
	}
	s.Recent = slices.DeleteFunc(s.Recent, func(r Recent) bool { return r.Kind == "table" && !strings.Contains(r.ID, "/") })
	var old struct {
		Consoles map[string]QueryState `json:"consoles"`
	}
	if json.Unmarshal(data, &old) == nil { // what the queries were called, F10.11: read, written as queries
		for k, v := range old.Consoles {
			if _, ok := s.Queries[currentName(k)]; !ok {
				if s.Queries == nil {
					s.Queries = map[string]QueryState{}
				}
				s.Queries[currentName(k)] = v
			}
		}
	}
	return s, nil
}

var (
	saveMu          sync.Mutex
	tick, savedTick int // the last snapshot's, and the one on disk's
)

// Snapshot serializes s as it is now and returns what writes it (§14):
// call it in Update, run what it returns on a Cmd's goroutine. Writes run
// in any order; one of an older snapshot than the file holds does nothing.
func Snapshot(s *State) func() error {
	data, err := json.Marshal(s)
	saveMu.Lock()
	tick++
	t := tick
	saveMu.Unlock()
	return func() error {
		if err != nil {
			return err
		}
		return save(data, t)
	}
}

// save writes the file, readable by the user only (§13: history holds
// literals).
func save(data []byte, t int) error {
	saveMu.Lock()
	defer saveMu.Unlock()
	if t <= savedTick {
		return nil
	}
	if err := writeFile(statePath(), data); err != nil {
		return err
	}
	savedTick = t
	return nil
}

// beforeRename is the test that kills a write between the temp file and the
// rename, hooked here; nothing in the program.
var beforeRename = func() {}

// writeFile writes path whole through a file renamed into place, so a
// crash leaves the old one; the directories it makes and the file are the
// user's only.
func writeFile(path string, data []byte) error { return writeFileMode(path, data, 0o600) }

// writeFileMode is writeFile with the file made perm.
func writeFileMode(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, filepath.Base(path)+"-*") // 0600
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name()) // gone once renamed
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(perm); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	beforeRename()
	return os.Rename(tmp.Name(), path)
}
