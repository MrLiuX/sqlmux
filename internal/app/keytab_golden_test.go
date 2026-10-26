package app

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/exp/golden"

	"github.com/kackerx/sqlmux/internal/db"
)

// A key tab of each type: the Info row over MATCH, the chips, and the rows by the
// columns of the type (F16.3). The pages are made here, not read: what a server
// says of memory differs from version to version.
func TestGoldenKeyTabs(t *testing.T) {
	text := func(name string) db.Column { return db.Column{Name: name, Type: "text", NotNull: true} }
	str := func(ss ...string) []db.Val {
		var out []db.Val
		for _, s := range ss {
			out = append(out, db.Val{S: s})
		}
		return out
	}
	info := func(typ, n, enc string) []db.Attr {
		as := []db.Attr{{Name: "type", Value: typ}, {Name: "length", Value: n}, {Name: "ttl", Value: "12m03s"}}
		if enc != "" {
			as = append(as, db.Attr{Name: "encoding", Value: enc}, db.Attr{Name: "memory", Value: "112B"})
		}
		return as
	}
	for _, k := range []struct {
		typ, name string
		cols      db.Columns
		rows      [][]db.Val
		info      []db.Attr
		next      bool
	}{
		{"string", "str:plain", db.Columns{Cols: []db.Column{text("value")}, PK: []string{}, Sortable: []string{}}, [][]db.Val{str("hello")}, info("string", "5", "embstr"), false},
		{"hash", "hash:user", db.Columns{Cols: []db.Column{text("field"), text("value")}, PK: []string{"field"}}, [][]db.Val{str("age", "30"), str("email", "ann@example.com"), str("name", "Ann")}, info("hash", "3", "listpack"), false},
		{"list", "list:queue", db.Columns{Cols: []db.Column{{Name: "index", Type: "bigint", Generated: true, ReadOnly: "index"}, text("value")}, PK: []string{"index"}, Sortable: []string{}}, [][]db.Val{str("0", "first"), str("1", "second")}, info("list", "2", "listpack"), false},
		{"set", "set:tags", db.Columns{Cols: []db.Column{text("member")}, PK: []string{"member"}}, [][]db.Val{str("blue"), str("green"), str("red")}, info("set", "3", "listpack"), false},
		{"zset", "zset:rank", db.Columns{Cols: []db.Column{text("member"), {Name: "score", Type: "double precision"}}, PK: []string{"member"}}, [][]db.Val{str("alice", "1"), str("bob", "2.5")}, info("zset", "2", "listpack"), false},
		{"stream", "stream:events", db.Columns{Cols: []db.Column{text("id")}, PK: []string{"id"}, Sortable: []string{}}, [][]db.Val{{{S: "1700000000000-1"}, {S: "login"}}, {{S: "1700000000000-2"}, {Null: true}}}, info("stream", "2", "stream"), true},
		{"json", "json:doc", db.Columns{Cols: []db.Column{{Name: "value", Type: "json"}}, PK: []string{}, Sortable: []string{}}, [][]db.Val{str(`{"a":1}`)}, []db.Attr{{Name: "type", Value: "json"}, {Name: "ttl", Value: "不过期"}}, false},
	} {
		t.Run(k.typ, func(t *testing.T) {
			a, c := redisTree(t, someKeys())
			c.State = connReady
			a.openTable(c, db.Table{Name: k.name, Kind: "k"})
			tab := dataOf(a.focused())
			res := db.Result{Rows: k.rows}
			for _, col := range k.cols.Cols {
				res.Cols = append(res.Cols, db.Col{Name: col.Name})
			}
			if k.typ == "stream" {
				res.Cols = append(res.Cols, db.Col{Name: "type"})
			}
			a.Update(pageMsg{tab: tab, seq: tab.seq, cols: k.cols, page: res, next: k.next, info: k.info, cursor: map[bool]string{true: "1700000000000-2"}[k.next]})
			r := a.layout()[a.focused().ID]
			var lines []string
			for _, l := range strings.Split(a.render().String(), "\n")[r.Min.Y : r.Min.Y+9] {
				lines = append(lines, ansi.Cut(l, r.Min.X, r.Max.X))
			}
			golden.RequireEqual(t, strings.Join(lines, "\n"))
			if k.typ != "hash" {
				return
			}
			t.Run("typed", func(t *testing.T) {
				// MATCH typed into: the text and the cursor are on its own row, under the Info row, over the chips (F18.10)
				feed(t, a, "/e*")
				f := a.render()
				row := strings.Split(f.String(), "\n")[r.Min.Y+2]
				if i := strings.Index(row, "e*"); !strings.Contains(row, "MATCH e*") || f.Cursor == nil || f.Cursor.Y != r.Min.Y+2 || f.Cursor.X != ansi.StringWidth(row[:i])+2 {
					t.Errorf("MATCH row %q, cursor %v", row, f.Cursor)
				}
				var typed []string
				for _, l := range strings.Split(f.String(), "\n")[r.Min.Y : r.Min.Y+5] {
					typed = append(typed, ansi.Cut(l, r.Min.X, r.Max.X))
				}
				golden.RequireEqual(t, strings.Join(typed, "\n"))
			})
		})
	}
}
