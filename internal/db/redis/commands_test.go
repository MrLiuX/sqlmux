package redis

import (
	"errors"
	"io"
	"testing"

	goredis "github.com/redis/go-redis/v9"
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
