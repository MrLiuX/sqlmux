package app

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/kackerx/sqlmux/internal/config"
	"github.com/kackerx/sqlmux/internal/db"
	"github.com/kackerx/sqlmux/internal/sqlkit"
)

// sqlLang says of a statement what the run asks, as sqlkit does for its
// dialect (F16.0).
func TestSQLLang(t *testing.T) {
	pg, my := sqlLang{sqlkit.PG}, sqlLang{sqlkit.MySQL}
	if got := pg.Stmts("select 1; select 2"); len(got) != 2 {
		t.Errorf("statements: %v", got)
	}
	for _, c := range []struct {
		name string
		got  bool
		want bool
	}{
		{"insert writes", pg.Writes("insert into t values (1)"), true},
		{"select does not", pg.Writes("select 1"), false},
		{"drop is dangerous", pg.Dangerous("drop table t"), true},
		{"select is not", pg.Dangerous("select 1"), false},
		{"set resets", pg.Resets("set search_path to x"), true},
		{"use resets", my.Resets("use x"), true},
		{"select does not reset", pg.Resets("select 1"), false},
		{"alter is DDL", pg.DDL("alter table t add c int"), true},
		{"rename is DDL", my.DDL("rename table a to b"), true},
		{"select is no DDL", pg.DDL("select 1"), false},
		{"vacuum begins none", pg.NoBegin("vacuum"), true},
		{"select begins one", pg.NoBegin("select 1"), false},
		{"commit ends", pg.EndsTx("commit") == "commit", true},
		{"select does not end", pg.EndsTx("select 1") == "", true},
	} {
		if c.got != c.want {
			t.Errorf("%s: %v", c.name, c.got)
		}
	}
	if text, ok := pg.Page("select * from t", 501, 0); !ok || !strings.Contains(text, "501") {
		t.Errorf("a read is asked a page of: %q %v", text, ok)
	}
	if _, ok := pg.Page("insert into t values (1)", 501, 0); ok {
		t.Error("a write has no pages")
	}
}

// kvDriver is a Driver that is no Engine: a key-value server's.
type kvDriver struct{}

func (kvDriver) Connect(context.Context, string, string, bool) (db.Conn, db.Info, error) {
	return nil, db.Info{}, errors.New("not here")
}
func (kvDriver) Broken(error) db.Lost { return db.NotLost }
func (kvDriver) ErrorOf(err error) db.ServerError {
	return db.ServerError{Severity: "ERR", Code: "KV", Message: err.Error()}
}

// A connection has the driver of its spec and its SQL engine when the driver
// is one; what an error says and what is a lost connection come from the
// driver, which a connection with no engine has (F16.0).
func TestConnDriver(t *testing.T) {
	was := drivers["kv"]
	drivers["kv"] = kvDriver{}
	t.Cleanup(func() {
		if was == nil {
			delete(drivers, "kv")
		}
	})
	c := &Conn{}
	c.setDriver(config.Connection{Engine: "postgres"})
	if c.eng == nil || c.drv == nil || c.driver() != c.drv {
		t.Errorf("postgres: eng %v drv %v", c.eng, c.drv)
	}
	c.setDriver(config.Connection{Engine: "kv", ReadOnly: true})
	if c.eng != nil || c.driver() != (kvDriver{}) || !c.readOnly {
		t.Errorf("kv: eng %v driver %v read-only %v", c.eng, c.driver(), c.readOnly)
	}
	// a page that fails says its error through the driver, with no engine
	a := wide(160, 45)
	if a.openTable(a.sess.Conns[0], db.Table{Schema: "0", Name: "k"}) == nil {
		t.Fatal("opening a table fetches it")
	}
	tab := dataOf(a.focused())
	tab.conn.drv, tab.conn.eng = kvDriver{}, nil
	tab.src = &failing{}
	a.Update(a.fetch(tab, false)())
	if tab.bar == nil || !strings.HasPrefix(tab.bar.First.Head, "[KV]") || tab.bar.First.Mid != "no such key" {
		t.Errorf("the error bar: %+v", tab.bar)
	}
}

type failing struct{ fakeSource }

func (*failing) Page(context.Context, db.PageQuery) (db.Paged, error) {
	return db.Paged{}, errors.New("no such key")
}
