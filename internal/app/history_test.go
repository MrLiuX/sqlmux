package app

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/kackerx/sqlmux/internal/config"
	"github.com/kackerx/sqlmux/internal/db"
	"github.com/kackerx/sqlmux/internal/editor"
)

// Each statement a run reached is a line of history.jsonl, a failure with its
// message's first line, a cancel as such; those after the one that failed
// never ran, and are not in it (F14.2).
func TestRunRecordsHistory(t *testing.T) {
	fail := &pgconn.PgError{Severity: "ERROR", Message: "relation \"nope\" does not exist\nsecond line"}
	d := &execDB{res: map[string]db.Result{"select 1": rows(1), "update t set a = 1": {Tag: "UPDATE 3"}},
		fail: map[string]error{"select * from nope": fail, "select pg_sleep(9)": context.Canceled}}
	a, c := inRun(t, d, "select 1;\nupdate t set a = 1;\nselect * from nope;\nselect 3;")
	c.schema = "m0"
	conn := a.sess.Conns[0].Spec.Name
	press(t, a, "ggVG<CR>")
	c.ed.Load("select pg_sleep(9)")
	press(t, a, "<CR>")
	hs, err := config.LoadHistory()
	if err != nil || len(hs) != 4 {
		t.Fatalf("%d entries: %v %v", len(hs), hs, err)
	}
	for i, want := range []config.History{
		{Conn: conn, Schema: "m0", SQL: "select 1"},
		{Conn: conn, Schema: "m0", SQL: "update t set a = 1", Tag: "UPDATE 3"},
		{Conn: conn, Schema: "m0", SQL: "select * from nope", Error: `relation "nope" does not exist`},
		{Conn: conn, Schema: "m0", SQL: "select pg_sleep(9)", Error: "canceled"},
	} {
		if got := hs[i]; got.Conn != want.Conn || got.Schema != want.Schema || got.SQL != want.SQL || got.Tag != want.Tag || got.Error != want.Error || got.At.IsZero() {
			t.Errorf("entry %d: %+v, want %+v", i, got, want)
		}
	}
}

// C-r in INSERT opens the history of the query's connection, the newest first;
// what is typed filters it, ↵ puts the statement as it was at the cursor.
func TestHistoryList(t *testing.T) {
	a, c := inQuery(t, "", "")
	conn := a.sess.Conns[0].Spec.Name
	for _, h := range []config.History{
		{Conn: conn, SQL: "select 1", Tag: "SELECT 1", TookMS: 3},
		{Conn: "other", SQL: "select from_other"},
		{Conn: conn, SQL: "select a,\n       b\n  from two", Error: "boom"},
		{Conn: conn, SQL: "update three set x = 1", Tag: "UPDATE 1"},
	} {
		h.At = time.Now()
		if err := config.AppendHistory(h); err != nil {
			t.Fatal(err)
		}
	}
	feed(t, a, "<C-r>")
	if a.drop != nil {
		t.Fatal("C-r in NORMAL is vim's redo")
	}
	feed(t, a, "ifoo ")
	feed(t, a, "<C-r>")
	if a.drop == nil || a.drop.kind != dropHistory || c.ed.Mode() != editor.Insert {
		t.Fatalf("C-r in INSERT: %v", a.drop)
	}
	items, _ := a.dropItems()
	if strings.Join(items, "|") != "update three set x = 1|select a, b from two|select 1" {
		t.Fatalf("items %q: this connection's, the newest first", items)
	}
	if note := a.dropNote(1); !strings.Contains(note, "失败: boom") {
		t.Errorf("note %q", note)
	}
	if !strings.Contains(a.render().String(), "update three set x = 1") {
		t.Error("the list is not drawn")
	}
	feed(t, a, "two<CR>")
	if a.drop != nil || text(c) != "foo select a,\n       b\n  from two" || c.ed.Mode() != editor.Insert {
		t.Fatalf("↵: %q, drop %v", text(c), a.drop)
	}
	feed(t, a, "<C-r><Esc>")
	if a.drop != nil || text(c) != "foo select a,\n       b\n  from two" {
		t.Fatalf("esc closes it: %q", text(c))
	}
}

// vim = false has C-r for the history as well, and the palette has it for a
// query in NORMAL.
func TestHistoryPlainAndPalette(t *testing.T) {
	a, _ := inQuery(t, "vim = false", "")
	_ = config.AppendHistory(config.History{Conn: a.sess.Conns[0].Spec.Name, SQL: "select 1", At: time.Now()})
	feed(t, a, "<C-r>")
	if a.drop == nil || a.drop.kind != dropHistory || len(a.drop.hist) != 1 {
		t.Fatalf("vim = false: %v", a.drop)
	}
	b, _ := inQuery(t, "", "")
	feed(t, b, "<C-p>查询历史")
	feed(t, b, "<CR>")
	if b.drop == nil || b.drop.kind != dropHistory {
		t.Fatalf("palette: %v", b.drop)
	}
}
