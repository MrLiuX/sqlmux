package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kackerx/sqlmux/internal/config"
)

// fakeExec is a command that answers a few statements as a PG over a command would:
// where it landed, the schemas, the tables, and one row for anything else.
func fakeExec(t *testing.T) config.Connection {
	t.Helper()
	dir := t.TempDir()
	script := `#!/bin/sh
case "$1" in
*"select current_database()"*) echo '[{"columns":["a","b"],"rows":[["shop","public"]]}]' ;;
*"from pg_namespace n where"*) echo '[{"columns":["nspname","cs"],"rows":[["public","public"]]}]' ;;
*"from pg_class c join pg_namespace"*) echo '[{"columns":["n","r","t","k"],"rows":[["public","orders",12,"r"]]}]' ;;
*"from pg_proc p"*|*"cl.relkind = 'S'"*) echo '[{"columns":[],"rows":[]}]' ;;
*pg_database*) echo '[{"columns":["datname"],"rows":[["shop"]]}]' ;;
*) echo '[{"columns":["x"],"rows":[["one"],["two"]]}]' ;;
esac
`
	path := filepath.Join(dir, "cmd.sh")
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return config.Connection{Name: "prod", Engine: "exec", Dialect: "postgres", Command: []string{"/bin/sh", path, "{sql}"}, ReadOnly: true}
}

// A connection that is a command dials, loads its catalog into the tree, runs a query,
// and takes no write: read only is all it is (F17.7, §8.8).
func TestExecConnection(t *testing.T) {
	a, q := inQuery(t, "", "")
	c := a.sess.Conns[0]
	spec := fakeExec(t)
	c.setSpec(spec)
	d, err := dial(context.Background(), spec)
	if err != nil {
		t.Fatal(err)
	}
	c.take(d)
	if c.Addr != "sh" || c.DB != "shop" || !c.sql() || !c.readOnly || c.redis() {
		t.Fatalf("dialed: addr %q db %q sql %v read only %v", c.Addr, c.DB, c.sql(), c.readOnly)
	}
	m, ok := a.loadCatalog(c)().(catalogMsg)
	if !ok || m.err != nil || len(m.tables) != 1 || m.tables[0].Name != "orders" {
		t.Fatalf("catalog: %+v", m)
	}
	a.Update(m)
	if !strings.Contains(treeTexts(a), "orders") {
		t.Errorf("the tree:\n%s", treeTexts(a))
	}
	q.ed.Load("select x from t")
	q.conn = c
	deliver(a, a.queryRun(""))
	if got := logText(a); !strings.Contains(got, "select x from t") || strings.Contains(got, "error") {
		t.Errorf("the log: %s", got)
	}
	q.ed.Load("update t set x = 1")
	a.queryRun("")
	if got := logText(a); !strings.Contains(got, "prod 是只读连接") {
		t.Errorf("a write: %s", got)
	}
	// a command that does not work is the dial's error, with what it said
	bad := spec
	bad.Command = []string{"/bin/sh", "-c", "echo not logged in >&2; exit 2", "{sql}"}
	if _, err := dial(context.Background(), bad); err == nil || err.Error() != "not logged in" {
		t.Errorf("a command that fails: %v", err)
	}
}
