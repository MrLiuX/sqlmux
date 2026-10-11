//go:build integration

package app

import (
	"context"
	"encoding/csv"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/kackerx/sqlmux/internal/config"
	"github.com/kackerx/sqlmux/internal/db"
	"github.com/kackerx/sqlmux/internal/db/mysql"
	"github.com/kackerx/sqlmux/internal/db/postgres"
	"github.com/kackerx/sqlmux/internal/sqlkit"
)

// The condition of each kind of cell, written by cellCondition from what the
// server sent, keeps that row on the server and no other: the quotes, the
// backslash of MySQL, a number, a boolean, a time, a uuid, jsonb and NULL
// (F12.3). The table is temporary, in this connection only.
func TestFilterCellOnServer(t *testing.T) {
	ctx := context.Background()
	dsns := engineDSNs(t)
	type col struct{ name, typ string }
	run := func(t *testing.T, c db.Conn, d sqlkit.Dialect, quote func(string) string, cols []col, ddl string, inserts []string) {
		t.Helper()
		if _, err := c.Query(ctx, ddl); err != nil {
			t.Fatal(err)
		}
		for _, s := range inserts {
			if _, err := c.Query(ctx, s); err != nil {
				t.Fatalf("%s: %v", s, err)
			}
		}
		for k := 1; k <= len(inserts); k++ {
			row, err := c.Query(ctx, "select * from f where k = "+string(rune('0'+k)))
			if err != nil || len(row.Rows) != 1 {
				t.Fatalf("row %d: %v %v", k, row.Rows, err)
			}
			for i, cl := range cols {
				cond, ok := cellCondition(d, quote(cl.name), cl.typ, false, row.Rows[0][i])
				if !ok {
					t.Errorf("row %d %s (%s): no condition", k, cl.name, cl.typ)
					continue
				}
				res, err := c.Query(ctx, "select count(*) from f where k = "+string(rune('0'+k))+" and "+cond)
				if err != nil || len(res.Rows) != 1 || res.Rows[0][0].S != "1" {
					t.Errorf("row %d %s (%s) %q: %v %v", k, cl.name, cl.typ, cond, res.Rows, err)
				}
			}
		}
	}
	if dsn, ok := dsns["postgres"]; ok {
		t.Run("postgres", func(t *testing.T) {
			c, _, err := postgres.Connect(ctx, dsn, "", false)
			if err != nil {
				t.Fatal(err)
			}
			defer c.Close()
			run(t, c, sqlkit.PG, func(s string) string { return `"` + s + `"` },
				[]col{{"k", "integer"}, {"t", "text"}, {"n", "numeric(10,2)"}, {"x", "double precision"}, {"b", "boolean"}, {"ts", "timestamp with time zone"},
					{"d", "date"}, {"u", "uuid"}, {"j", "jsonb"}, {"ip", "inet"}, {"v", "character varying(20)"}},
				`create temp table f (k int, t text, n numeric(10,2), x float8, b bool, ts timestamptz, d date, u uuid, j jsonb, ip inet, v varchar(20))`,
				[]string{
					`insert into f values (1, E'it''s a \\ back"slash', -1.50, 2.5e10, true, '2026-09-01 10:00:00.5+00', '2026-09-01', 'a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11', '{"a": [1, "it''s"]}', '10.0.0.1', 'x')`,
					`insert into f values (2, NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL)`,
					`insert into f values (3, 'plain', 'NaN', 'Infinity', false, 'infinity', 'infinity', null, null, null, '')`,
				})
		})
	}
	if dsn, ok := dsns["mysql"]; ok {
		t.Run("mysql", func(t *testing.T) {
			c, _, err := mysql.Connect(ctx, dsn, "", false)
			if err != nil {
				t.Fatal(err)
			}
			defer c.Close()
			run(t, c, sqlkit.MySQL, func(s string) string { return "`" + s + "`" },
				[]col{{"k", "int"}, {"t", "text"}, {"n", "decimal(10,2)"}, {"x", "double"}, {"b", "tinyint(1)"}, {"ts", "datetime"}, {"d", "date"}, {"e", "enum('a','b')"}, {"v", "varchar(20)"}, {"u", "int unsigned"}},
				"create temporary table f (k int, t text, n decimal(10,2), x double, b tinyint(1), ts datetime, d date, e enum('a','b'), v varchar(20), u int unsigned)",
				[]string{
					`insert into f values (1, 'it''s a \\ back"slash', -1.50, 2.5e10, 1, '2026-09-01 10:00:00', '2026-09-01', 'b', 'x', 4000000000)`,
					`insert into f values (2, NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL)`,
					`insert into f values (3, 'plain', 0, 0, 0, '2000-01-01 00:00:00', '2000-01-01', 'a', '', 0)`,
				})
		})
	}
}

// "All" of a table written to a file has every row, in the order the grid
// shows them (F12.4): t_order, 6000 rows, by status descending; the first page
// of it is the first rows of the file.
func TestExportAllOnServer(t *testing.T) { eachEngine(t, exportAllOn) }

func exportAllOn(t *testing.T, eng, dsn string) {
	dir := t.TempDir()
	t.Chdir(dir)
	a := opened(t, config.Connection{Name: "play", Engine: eng, DSN: dsn})
	play := a.sess.Conns[0]
	i := slices.IndexFunc(play.Tables, func(tb db.Table) bool { return tb.Schema == play.Schema && tb.Name == "t_order" })
	settle(a, a.openTable(play, play.Tables[i]))
	tab := dataOf(a.focused())
	tab.order, tab.pageNo = orderOf("status-"), 0
	settle(a, a.fetch(tab, false))
	id := slices.IndexFunc(tab.page.Cols, func(c db.Col) bool { return c.Name == "id" })
	a.run("grid.export", 0)
	a.Update(teaKey("<C-n>")) // CSV · 全部
	_, cmd := a.Update(teaKey("<CR>"))
	a.Update(cmd())
	data, err := os.ReadFile(filepath.Join(dir, "t_order.csv"))
	if err != nil {
		t.Fatal(err)
	}
	recs, err := csv.NewReader(strings.NewReader(string(data))).ReadAll()
	if err != nil || len(recs) != 6001 || tab.conn.busy != 0 {
		t.Fatalf("%d records, %v, busy %d, toast %q", len(recs), err, tab.conn.busy, a.toast)
	}
	for r, row := range tab.page.Rows {
		if recs[r+1][id] != row[id].S {
			t.Fatalf("row %d: id %s in the file, %s in the grid", r+1, recs[r+1][id], row[id].S)
		}
	}
	seen := map[string]bool{}
	for _, rec := range recs[1:] {
		seen[rec[id]] = true
	}
	if len(seen) != 6000 {
		t.Errorf("%d different ids", len(seen))
	}
}
