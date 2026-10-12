package app

import (
	"testing"

	"github.com/kackerx/sqlmux/internal/config"
)

// The session of a connection of the command line has a new query, query_2
// where query_1 is a file: query_1 is the user's, and what the state keeps by
// its name (F14.1).
func TestOpenTempQuery(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	if err := config.WriteQuery(config.QueryPath("query_1"), "select 'mine'"); err != nil {
		t.Fatal(err)
	}
	s := OpenTemp(config.Connection{Name: "x", Engine: "sqlite", DSN: "/tmp/x.db", Temp: true})
	qs := s.queries()
	if len(qs) != 1 || qs[0].name != "query_2" || !s.Conns[0].Spec.Temp {
		t.Fatalf("queries %v", qs)
	}
}
