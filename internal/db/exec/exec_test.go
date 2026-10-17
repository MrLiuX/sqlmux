package exec

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kackerx/sqlmux/internal/db"
	"github.com/kackerx/sqlmux/internal/db/postgres"
	"github.com/kackerx/sqlmux/internal/sqlkit"
)

// fake is a command that answers by the statement it is given, and keeps what it
// was given in log.
func fake(t *testing.T) (argv []string, log string) {
	t.Helper()
	dir := t.TempDir()
	log = filepath.Join(dir, "log")
	script := `#!/bin/sh
sql="$1"
printf '%s\n' "$sql" >> "` + log + `"
case "$sql" in
*"select current_database()"*) echo '[{"columns":["a","b"],"rows":[["shop","\"$user\", public"]]}]' ;;
*"from pg_namespace n where"*) echo '[{"columns":["nspname","cs"],"rows":[["public","public"],["sales","public"]]}]' ;;
*"from pg_class c join pg_namespace"*) echo '[{"columns":["n","r","t","k"],"rows":[["public","orders",12,"r"],["public","v_orders",-1,"v"]]}]' ;;
*pg_database*) echo '[{"columns":["datname"],"rows":[["shop"],["other"]]}]' ;;
*tagged*) echo '[{"n":3}]' ;;
*bigerr*) head -c 10000 /dev/zero | tr '\000' a >&2; echo END >&2; exit 1 ;;
*stderr*) echo "the panel is not open" >&2; exit 3 ;;
*itemerr*) echo '[{"err":"relation does not exist"}]' ;;
*notjson*) echo 'hello' ;;
*slow*) sleep 5; echo '[]' ;;
*ragged*) echo '[{"columns":["a","b"],"rows":[[1],[1,2,3]]}]' ;;
*one*) echo '{"columns":["c"],"rows":[["only"]]}' ;;
*) echo '[{"columns":["x"],"rows":[[1],[null],[true],["it'"'"'s"],[{"a":1}],[2.50],[false]],"n":7}]' ;;
esac
`
	path := filepath.Join(dir, "cmd.sh")
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return []string{"/bin/sh", path, "{sql}"}, log
}

func ran(t *testing.T, log string) []string {
	t.Helper()
	b, _ := os.ReadFile(log)
	return strings.Split(strings.TrimSpace(string(b)), "\n")
}

// A run is the command with the statement in place of {sql}; the output's values are
// the text protocol's: a number as written, t and f, NULL, nested JSON as text (F17.7).
func TestQueryValues(t *testing.T) {
	argv, log := fake(t)
	c, err := New(argv, sqlkit.PG)
	if err != nil {
		t.Fatal(err)
	}
	r, err := c.Query(context.Background(), "select x from t where a = $1 and b = $2", db.Val{S: "it's"}, db.Val{Null: true})
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, row := range r.Rows {
		s := row[0].S
		if row[0].Null {
			s = "NULL"
		}
		got = append(got, s)
	}
	if want := "1 NULL t it's {\"a\":1} 2.50 f"; strings.Join(got, " ") != want || r.Affected != 7 || len(r.Cols) != 1 || r.Cols[0].Name != "x" {
		t.Errorf("rows %q, affected %d, cols %v", got, r.Affected, r.Cols)
	}
	if sent := ran(t, log); len(sent) != 1 || sent[0] != "select x from t where a = 'it''s' and b = NULL" {
		t.Errorf("the command was given %q", sent)
	}
}

// $n goes in as a literal where it is code, not in a string, an identifier or a comment.
func TestInline(t *testing.T) {
	for _, c := range []struct {
		sql, want string
		args      []db.Val
	}{
		{"select $1, $2::text", "select 'a', NULL::text", []db.Val{{S: "a"}, {Null: true}}},
		{"select '$1', \"c$1\" -- $1\n, $1 /* $2 */", "select '$1', \"c$1\" -- $1\n, 'a' /* $2 */", []db.Val{{S: "a"}}},
		{"select 'it''s $1', $1", "select 'it''s $1', 'x'", []db.Val{{S: "x"}}},
		{"select $10, $1", "select 'j', 'a'", []db.Val{{S: "a"}, {}, {}, {}, {}, {}, {}, {}, {}, {S: "j"}}},
		{"select $$x$$, $1", "select $$x$$, 'a'", []db.Val{{S: "a"}}}, // a $$ is no parameter's
		{"select $q$ $1 $q$, $$ $1 $$, $1", "select $q$ $1 $q$, $$ $1 $$, 'a'", []db.Val{{S: "a"}}},
		{`select E'it\'s $1', $1`, `select E'it\'s $1', 'a'`, []db.Val{{S: "a"}}},
		{"select $ 1, $1", "select $ 1, 'a'", []db.Val{{S: "a"}}},                               // a $ and a number apart are two things
		{"select /* a /* $1 */ $1 */ $1", "select /* a /* $1 */ $1 */ 'a'", []db.Val{{S: "a"}}}, // PG's comments nest
	} {
		got, err := Inline(c.sql, c.args, sqlkit.PG)
		if err != nil || got != c.want {
			t.Errorf("%q: %q %v, want %q", c.sql, got, err, c.want)
		}
	}
	for _, sql := range []string{"select $2", "select $0"} {
		if _, err := Inline(sql, []db.Val{{S: "a"}}, sqlkit.PG); err == nil {
			t.Errorf("%s: a parameter with no argument", sql)
		}
	}
}

// Each statement is a run of its own, the results in order; the first that fails stops
// the rest and comes back with those before it (F17.7).
func TestExec(t *testing.T) {
	argv, log := fake(t)
	c, _ := New(argv, sqlkit.PG)
	rs, err := c.Exec(context.Background(), "select one; select 'a;b'; select 3", 3)
	if err != nil || len(rs) != 3 || rs[0].Rows[0][0].S != "only" || len(rs[1].Rows) != 3 || !rs[1].Truncated {
		t.Fatalf("%+v %v", rs, err)
	}
	if sent := ran(t, log); len(sent) != 3 || sent[1] != "select 'a;b';" && sent[1] != "select 'a;b'" {
		t.Errorf("runs %q", sent)
	}
	rs, err = c.Exec(context.Background(), "select one; select itemerr; select one", 0)
	var ce Error
	if len(rs) != 1 || !errors.As(err, &ce) || ce.Msg != "relation does not exist" {
		t.Errorf("a failing statement: %d results, %v", len(rs), err)
	}
	if _, err := c.Exec(context.Background(), "select stderr", 0); !errors.As(err, &ce) || ce.Msg != "the panel is not open" {
		t.Errorf("stderr: %v", err)
	}
	if _, err := c.Exec(context.Background(), "select notjson", 0); !errors.As(err, &ce) || !strings.Contains(ce.Msg, "JSON") {
		t.Errorf("not JSON: %v", err)
	}
	if _, err := New([]string{"x"}, sqlkit.PG); err == nil {
		t.Error("no {sql}")
	}
	if _, err := New([]string{"/no/such/command", "{sql}"}, sqlkit.PG); err != nil {
		t.Error("a command that is not there is the first run's error")
	}
}

// A statement that writes has no columns and says how many rows it hit; an item that
// says err is that error on a single query as it is on Exec; what a command says on
// stderr is its last 4 KB (F17.7).
func TestQueryTagErrAndStderrTail(t *testing.T) {
	argv, _ := fake(t)
	c, _ := New(argv, sqlkit.PG)
	ctx := context.Background()
	if r, err := c.Query(ctx, "update tagged"); err != nil || r.Tag != "3" || r.Affected != 3 || len(r.Cols) != 0 {
		t.Errorf("no columns: %+v %v", r, err)
	}
	var ce Error
	if _, err := c.Query(ctx, "select itemerr"); !errors.As(err, &ce) || ce.Msg != "relation does not exist" {
		t.Errorf("an item's err: %v", err)
	}
	if _, err := c.Query(ctx, "select bigerr"); !errors.As(err, &ce) || len(ce.Msg) > 4096 || !strings.HasSuffix(ce.Msg, "aaaEND") {
		t.Errorf("stderr: %d bytes, ends %q", len(ce.Msg), ce.Msg[max(0, len(ce.Msg)-8):])
	}
}

// Cancelling kills the run, as a request's cancel does (F17.7).
func TestCancelKillsTheRun(t *testing.T) {
	argv, _ := fake(t)
	c, _ := New(argv, sqlkit.PG)
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err := c.Query(ctx, "select slow")
	if !errors.Is(err, context.DeadlineExceeded) || time.Since(start) > 3*time.Second {
		t.Errorf("%v after %v", err, time.Since(start))
	}
}

// The engine is PG's over the command: its catalog queries are the ones the tree makes,
// and where the connection landed is what the first run said (F17.7).
func TestEngineOverACommand(t *testing.T) {
	argv, _ := fake(t)
	e := Engine{Engine: postgres.Engine{}, Command: argv}
	ctx := context.Background()
	c, info, err := e.Connect(ctx, "", "", false)
	if err != nil || info.Database != "shop" || info.Addr != "sh" || info.Path != `"$user", public` {
		t.Fatalf("connect: %+v %v", info, err)
	}
	schemas, current, err := e.Schemas(ctx, c)
	if err != nil || strings.Join(schemas, " ") != "public sales" || current != "public" {
		t.Errorf("schemas %v %q %v", schemas, current, err)
	}
	ts, err := e.Tables(ctx, c)
	if err != nil || len(ts) != 2 || ts[0].Name != "orders" || ts[0].Rows != 12 || !ts[1].View() {
		t.Errorf("tables %+v %v", ts, err)
	}
	if dbs, err := e.Databases(ctx, c); err != nil || len(dbs) != 0 { // the command is made for the one it reaches, which another name would not change
		t.Errorf("databases %v %v", dbs, err)
	}
	if err := e.UseSchema(ctx, c, "sales", ""); err != nil {
		t.Errorf("UseSchema: %v", err)
	}
	if e.Broken(errors.New("x")) != db.NotLost {
		t.Error("a run that failed is not a lost connection")
	}
	if got := e.ErrorOf(Error{"boom"}); got.Message != "boom" || got.Severity != "ERROR" {
		t.Errorf("ErrorOf: %+v", got)
	}
	w := db.NewWorker(c)
	r, err := e.Quick(ctx, w, "select 1", "public", 3)
	if err != nil || len(r.Rows) != 3 || !r.Truncated {
		t.Errorf("quick: %d rows, truncated %v, %v", len(r.Rows), r.Truncated, err)
	}
	if _, _, err := (Engine{Engine: postgres.Engine{}, Command: []string{"/bin/sh", "-c", "echo down >&2; exit 1", "{sql}"}}).Connect(ctx, "", "", false); err == nil || err.Error() != "down" {
		t.Errorf("a command that does not work fails the connect: %v", err)
	}
}

// A row has as many cells as the columns, however the command wrote it: the code that
// reads a row by place does not read past its end.
func TestRaggedRows(t *testing.T) {
	argv, _ := fake(t)
	c, _ := New(argv, sqlkit.PG)
	r, err := c.Query(context.Background(), "select ragged")
	if err != nil || len(r.Rows) != 2 || len(r.Rows[0]) != 2 || !r.Rows[0][1].Null || len(r.Rows[1]) != 2 || r.Rows[1][1].S != "2" {
		t.Errorf("%+v %v", r.Rows, err)
	}
}
