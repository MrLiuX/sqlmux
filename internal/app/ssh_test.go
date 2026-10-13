package app

import (
	"context"
	"testing"

	"github.com/kackerx/sqlmux/internal/config"
	"github.com/kackerx/sqlmux/internal/db"
	"github.com/kackerx/sqlmux/internal/db/postgres"
	"github.com/kackerx/sqlmux/internal/ui"
)

// dialerSeen is PG's engine that connects to nothing and notes the dialer each
// connect was asked to go by, "" for none.
type dialerSeen struct {
	db.Engine
	got []string
}

func (d *dialerSeen) Connect(ctx context.Context, _, _ string, _ bool) (db.Conn, db.Info, error) {
	name := ""
	if dl, ok := db.DialerOf(ctx); ok {
		name = dl.Name
	}
	d.got = append(d.got, name)
	return versionDB{}, db.Info{Addr: "u@h:1", Database: "d"}, nil
}

// versionDB answers the form's test its version.
type versionDB struct{ noDB }

func (versionDB) Query(context.Context, string, ...db.Val) (db.Result, error) {
	return db.Result{Rows: [][]db.Val{{{S: "PostgreSQL 17.0"}}}}, nil
}

func seeing(t *testing.T, engine string) *dialerSeen {
	t.Helper()
	d := &dialerSeen{Engine: postgres.Engine{}}
	was := engines[engine]
	engines[engine] = d
	t.Cleanup(func() { engines[engine] = was })
	return d
}

// A connection with an ssh is dialed through it, in a session and in the
// form's test; one without is not, and neither is a SQLite file's (F15.2).
func TestConnectThroughSSH(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	for _, c := range []struct {
		spec config.Connection
		want string
	}{
		{config.Connection{Name: "a", Engine: "postgres", DSN: "postgres://h/d", SSH: "me@jump"}, "ssh:me@jump"},
		{config.Connection{Name: "b", Engine: "postgres", DSN: "postgres://h/d"}, ""},
		{config.Connection{Name: "c", Engine: "sqlite", DSN: "/tmp/x.db", SSH: "me@jump"}, ""},
	} {
		d := seeing(t, c.spec.Engine)
		if _, _, err := connectTo(context.Background(), c.spec, false); err != nil {
			t.Fatal(err)
		}
		if _, err := testConn(c.spec, "pw"); err != nil {
			t.Fatal(err)
		}
		if len(d.got) != 2 || d.got[0] != c.want || d.got[1] != c.want {
			t.Errorf("%s: dialed by %q, want %q", c.spec.Name, d.got, c.want)
		}
	}
}

// The form has the ssh: read from the file, saved to it, checked, new or not,
// and not there for a file (F15.2).
func TestConnFormSSH(t *testing.T) {
	a := managed(t)
	a.run("conn.manage", 0)
	feed(t, a, "j<Tab>")
	if got := a.form.fields[fieldSSH].Input.Text; got != "" {
		t.Fatalf("none yet: %q", got)
	}
	a.form.fields[fieldSSH].Input = ui.Input{Text: "-oProxyCommand=x", Pos: 16}
	feed(t, a, "<C-s>")
	if !a.form.fields[fieldSSH].Bad || a.form.err == "" {
		t.Fatalf("an option is no destination: %q", a.form.err)
	}
	a.form.fields[fieldSSH].Input = ui.Input{Text: "me@jump", Pos: 7}
	feed(t, a, "<C-s>")
	if got, err := config.ReadConnection("second"); err != nil || got.SSH != "me@jump" {
		t.Fatalf("saved: %+v %v", got, err)
	}
	a.run("conn.manage", 0)
	feed(t, a, "j<Tab>")
	if got := a.form.fields[fieldSSH].Input.Text; got != "me@jump" {
		t.Fatalf("read back: %q", got)
	}
	a.form.fields[fieldSSH].Input = ui.Input{}
	feed(t, a, "<C-s>")
	if got, _ := config.ReadConnection("second"); got.SSH != "" {
		t.Fatalf("cleared: %+v", got)
	}

	a.run("conn.new", 0)
	if !a.form.skipped(fieldSSH) || !a.managerView().Form.Fields[fieldSSH].Hidden {
		t.Error("in the fold, closed")
	}
	a.form.fields[fieldAdvanced].On = 1
	if a.form.skipped(fieldSSH) {
		t.Error("the fold is open")
	}
	c := a.form.fields
	for field, text := range map[int]string{fieldName: "tunneled", fieldHost: "db.internal", fieldPort: "5432", fieldUser: "u", fieldDatabase: "d", fieldSSH: "me@jump"} {
		c[field].Input = ui.Input{Text: text, Pos: len(text)}
	}
	if got, ok := a.formConn(); !ok || got.SSH != "me@jump" {
		t.Fatalf("new: %+v %v", got, ok)
	}
	a.form.fields[fieldEngine].On = 2 // sqlite
	if !a.form.skipped(fieldSSH) {
		t.Error("a file has no ssh")
	}
	fields := a.managerView().Form.Fields
	if !fields[fieldSSH].Hidden {
		t.Error("and is not shown")
	}
}
