//go:build integration

package redis

import (
	"context"
	"testing"
)

// The server's own list says what writes and what is dangerous (§9.8).
func TestLoadCommands(t *testing.T) {
	cs, err := LoadCommands(context.Background(), connectTo(t, IntegrationDSN(t)))
	if err != nil || len(cs) < 200 {
		t.Fatalf("%d commands, %v", len(cs), err)
	}
	for line, want := range map[string][2]bool{"GET k": {false, false}, "SET k v": {true, false}, "HSET h f v": {true, false}, "FLUSHALL": {true, true}, "CONFIG GET *": {true, true}, "CONFIG SET a b": {true, true}, "EVAL 'return 1' 0": {true, false}, "PING": {false, false}, "KEYS *": {false, true}} {
		if w, d := cs.Writes(line), cs.Dangerous(line); w != want[0] || d != want[1] {
			t.Errorf("%s: writes %v dangerous %v", line, w, d)
		}
	}
}
