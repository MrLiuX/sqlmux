package redis

import (
	"errors"
	"io"
	"slices"
	"testing"

	goredis "github.com/redis/go-redis/v9"

	"github.com/kackerx/sqlmux/internal/db"
)

// The commands are those COMMAND lists with their subcommands, and a line is a
// write or dangerous by its command's flags and categories: a subcommand by its
// own (CONFIG GET reads, CONFIG SET writes), a command the server did not list or
// a server that did not say is a write (§9.8).
func TestCommandsJudge(t *testing.T) {
	reply := []any{
		[]any{"get", int64(2), []any{"readonly", "fast"}, int64(1), int64(1), int64(1), []any{"@read", "@string"}, []any{}, []any{}, []any{}},
		[]any{"set", int64(-3), []any{"write", "denyoom"}, int64(1), int64(1), int64(1), []any{"@write", "@string"}, []any{}, []any{}, []any{}},
		[]any{"eval", int64(-3), []any{"noscript"}, int64(0), int64(0), int64(0), []any{"@slow", "@scripting"}, []any{}, []any{}, []any{}},
		[]any{"eval_ro", int64(-3), []any{"noscript", "readonly"}, int64(0), int64(0), int64(0), []any{"@slow", "@scripting"}, []any{}, []any{}, []any{}},
		[]any{"publish", int64(3), []any{"pubsub", "may_replicate"}, int64(0), int64(0), int64(0), []any{"@pubsub"}, []any{}, []any{}, []any{}},
		[]any{"flushall", int64(-1), []any{"write"}, int64(0), int64(0), int64(0), []any{"@keyspace", "@write", "@dangerous"}, []any{}, []any{}, []any{}},
		[]any{"config", int64(-2), []any{}, int64(0), int64(0), int64(0), []any{"@slow"}, []any{}, []any{}, []any{
			[]any{"config|get", int64(-3), []any{"admin"}, int64(0), int64(0), int64(0), []any{"@admin", "@dangerous"}, []any{}, []any{}, []any{}},
			[]any{"config|help", int64(2), []any{"loading"}, int64(0), int64(0), int64(0), []any{"@slow"}, []any{}, []any{}, []any{}},
			[]any{"config|set", int64(-4), []any{"admin", "write"}, int64(0), int64(0), int64(0), []any{"@admin", "@dangerous"}, []any{}, []any{}, []any{}},
		}},
	}
	cs := Commands{}
	cs.add(reply)
	for _, c := range []struct {
		line              string
		writes, dangerous bool
	}{
		{"GET k", false, false},
		{"  get   k  ", false, false},
		{`set k "a b"`, true, false},
		{"EVAL 'return 1' 0", true, false},
		{"EVAL_RO 'return 1' 0", false, false},
		{"publish c m", true, false},
		{"FLUSHALL", true, true},
		{"config get maxmemory", true, true},
		{"CONFIG HELP", false, false},
		{"config set maxmemory 1", true, true},
		{"config", false, false},
		{"nosuch k", true, false},
		{`get "unclosed`, false, false}, // not sent
		{"", false, false},
	} {
		if w, d := cs.Writes(c.line), cs.Dangerous(c.line); w != c.writes || d != c.dangerous {
			t.Errorf("%q: writes %v dangerous %v, want %v %v", c.line, w, d, c.writes, c.dangerous)
		}
	}
	var none Commands
	if !none.Writes("get k") || none.Dangerous("flushall") {
		t.Error("a server that did not say: everything writes, nothing is dangerous")
	}
	// the readonly flag, and nothing else, is what a line is repeated on (§8.7)
	for _, c := range []struct {
		line string
		want bool
	}{
		{"GET k", true},
		{"  get   k  ", true},
		{"EVAL_RO 'return 1' 0", true},
		{"EVAL 'return 1' 0", false},
		{"set k 1", false},
		{"nosuch k", false},
		{"config help", false}, // not a write, and not flagged readonly either
		{`get "unclosed`, false},
		{"# GET k", false},
		{"", false},
	} {
		if got := cs.ReadOnly(c.line); got != c.want {
			t.Errorf("ReadOnly(%q) = %v", c.line, got)
		}
	}
	if none.ReadOnly("get k") {
		t.Error("a server that did not say: nothing is known to be readonly")
	}
}

// The history keeps no line that has a password in it.
func TestSensitive(t *testing.T) {
	for line, want := range map[string]bool{
		"AUTH hunter2": true, "auth user pw": true, "HELLO 3 AUTH u p": true, "migrate h 6379 k 0 1000 AUTH2 u p": true,
		"ACL SETUSER bob on >pw": true, "acl getuser bob": false, "CONFIG SET requirepass x": true, "config set maxmemory 1": false,
		"config set maxmemory 1 requirepass x": true, "CONFIG SET masterauth x": true, "config set masteruser u": true, "config get requirepass": false, "GET auth": false, "": false,
	} {
		if got := Sensitive(line); got != want {
			t.Errorf("%q: %v", line, got)
		}
	}
}

// A read of a key that finds it gone, or of another type, says so as one thing; any
// other error is itself (F16.3).
func TestKeyErr(t *testing.T) {
	for name, c := range map[string]struct {
		err  error
		same bool
	}{
		"nil": {goredis.Nil, false}, "wrong type": {redisError("WRONGTYPE Operation against a key holding the wrong kind of value"), false},
		"another reply": {redisError("ERR syntax error"), true}, "a network one": {io.EOF, true},
	} {
		got := keyErr(c.err)
		if c.same && !errors.Is(got, c.err) || !c.same && !errors.Is(got, ErrNoKey) {
			t.Errorf("%s: %v", name, got)
		}
	}
}

// The commands of a save, as Preview writes them: plan run on the key as the tab
// loaded it, each type, a value with a blank quoted, a rename a delete and a set, what
// a rename keeps said when the changes do not, and none for a type it does not know
// or a change plan refuses (F17.10).
func TestKeyPreview(t *testing.T) {
	val := func(s string) db.Val { return db.Val{S: s} }
	chg := func(name, old, now string) db.Change { return db.Change{Name: name, Old: val(old), Val: val(now)} }
	row := func(key string, cols ...db.Change) db.Row { return db.Row{Key: []string{key}, Cols: cols} }
	add := func(pairs ...string) (r []db.Change) {
		for i := 0; i < len(pairs); i += 2 {
			r = append(r, db.Change{Name: pairs[i], Val: val(pairs[i+1])})
		}
		return r
	}
	for _, tc := range []struct {
		typ  string
		ch   db.Changes
		want []string
	}{
		{"string", db.Changes{Updates: []db.Row{row("", chg("value", "v", "a b"))}}, []string{`SET k "a b" KEEPTTL`}},
		{"string", db.Changes{Deletes: [][]string{{}}}, []string{"UNLINK k"}},
		{"ReJSON-RL", db.Changes{Updates: []db.Row{row("", chg("value", "1", "{}"))}}, []string{"JSON.SET k $ {}"}},
		{"hash", db.Changes{Deletes: [][]string{{"c"}}, Updates: []db.Row{row("a", chg("value", "1", "x y")), row("b", chg("field", "b", "bee"))}, Inserts: [][]db.Change{add("field", "e", "value", "5")}},
			[]string{"HDEL k c", `HSET k a "x y"`, "HDEL k b", "HSET k bee <its value>", "HSET k e 5"}},
		{"hash", db.Changes{Updates: []db.Row{row("b", chg("field", "b", "bee"), chg("value", "2", "BEE"))}}, []string{"HDEL k b", "HSET k bee BEE"}},
		{"zset", db.Changes{Deletes: [][]string{{"d"}}, Updates: []db.Row{row("a", chg("score", "1", "5")), row("b", chg("member", "b", "B"))}, Inserts: [][]db.Change{add("member", "m", "score", "9")}},
			[]string{"ZREM k d", "ZADD k 5 a", "ZREM k b", "ZADD k <its score> B", "ZADD k 9 m"}},
		{"list", db.Changes{Deletes: [][]string{{"1"}}, Updates: []db.Row{row("0", chg("value", "a", "A"))}, Inserts: [][]db.Change{add("value", "g")}},
			[]string{"LSET k 1 <deleted>", "LSET k 0 A", "LREM k 0 <deleted>", "RPUSH k g"}},
		{"set", db.Changes{Deletes: [][]string{{"y"}}, Updates: []db.Row{row("x", chg("member", "x", "X"))}, Inserts: [][]db.Change{add("member", "n")}},
			[]string{"SREM k y", "SREM k x", "SADD k X", "SADD k n"}},
		{"stream", db.Changes{Deletes: [][]string{{"1-1"}}}, []string{"XDEL k 1-1"}},
		// what is data is not a placeholder: a member called NaN, a value that reads <its value>
		{"zset", db.Changes{Deletes: [][]string{{"NaN"}}, Updates: []db.Row{row("NaN", chg("member", "NaN", "n"))}}, []string{"ZREM k NaN", "ZREM k NaN", "ZADD k <its score> n"}},
		{"hash", db.Changes{Inserts: [][]db.Change{add("field", "f", "value", "<its value>")}}, []string{`HSET k f "<its value>"`}},
		{"", db.Changes{Deletes: [][]string{{"c"}}}, nil},
		{"string", db.Changes{Inserts: [][]db.Change{add("value", "x")}}, nil}, // a string has one value: Save says so
		{"hash", db.Changes{Updates: []db.Row{row("a", db.Change{Name: "value", Val: db.Val{Null: true}})}}, nil},
	} {
		if got := (Key{Name: "k", Type: tc.typ}).Preview(tc.ch); !slices.Equal(got, tc.want) {
			t.Errorf("%s: %q, want %q", tc.typ, got, tc.want)
		}
	}
}
