package db_test

import (
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/kackerx/sqlmux/internal/db"
	"github.com/kackerx/sqlmux/internal/db/mysql"
	"github.com/kackerx/sqlmux/internal/db/postgres"
	"github.com/kackerx/sqlmux/internal/db/sqlite"
	"github.com/kackerx/sqlmux/internal/sqlkit"
)

// One UPDATE a row: DEFAULT as the keyword, the row identity with =, every
// changed column still holding what was loaded as the engine's Same has
// it, or IS NULL (§10.3).
func TestUpdateSQL(t *testing.T) {
	sql, args := db.UpdateSQL(postgres.Engine{}, "public", "t_order_item", []string{"order_id", "line_no"}, db.Row{Key: []string{"3", "2"}, Cols: []db.Change{
		{Name: "qty", Val: db.Val{S: "9"}, Old: db.Val{S: "1"}},
		{Name: "sku code", Default: true, Old: db.Val{Null: true}},
	}})
	want := `update "public"."t_order_item" set "qty" = $1, "sku code" = DEFAULT where "order_id" = $2 and "line_no" = $3` +
		` and format('%s', "qty") = $4 and "sku code" is null`
	if sql != want {
		t.Errorf("sql:\n%s\nwant\n%s", sql, want)
	}
	if !reflect.DeepEqual(args, []db.Val{{S: "9"}, {S: "3"}, {S: "2"}, {S: "1"}}) {
		t.Errorf("args %v", args)
	}
}

// A row added with nothing set takes every column's DEFAULT, as each
// engine writes it (§10.6).
func TestInsertNothingSet(t *testing.T) {
	for e, want := range map[db.Engine]string{
		postgres.Engine{}: `insert into "s"."t" default values`,
		mysql.Engine{}:    "insert into `s`.`t` () values ()",
		sqlite.Engine{}:   `insert into "s"."t" default values`,
	} {
		if sql, args := db.InsertSQL(e, "s", "t", nil); sql != want || args != nil {
			t.Errorf("%s %v, want %s", sql, args, want)
		}
	}
}

// A column left to its DEFAULT is left out of the INSERT, which SQLite has no
// keyword for in a VALUES; all of them so is a row of DEFAULT VALUES (F14.1).
func TestInsertLeavesDefaultsOut(t *testing.T) {
	sql, args := db.InsertSQL(postgres.Engine{}, "s", "t", []db.Change{{Name: "a", Val: db.Val{S: "1"}}, {Name: "b", Default: true}, {Name: "c", Val: db.Val{Null: true}}})
	if want := `insert into "s"."t" ("a", "c") values ($1, $2)`; sql != want || len(args) != 2 {
		t.Errorf("%s %v, want %s", sql, args, want)
	}
	if sql, _ := db.InsertSQL(postgres.Engine{}, "s", "t", []db.Change{{Name: "b", Default: true}}); sql != `insert into "s"."t" default values` {
		t.Errorf("all DEFAULT: %s", sql)
	}
}

// What a save shows before it sends is the statements with their arguments written
// in as literals: the numbered text with each argument put where it goes, in every
// engine, a quote doubled, MySQL's backslash too, a NULL as it is (§13, F17.10).
func TestPreview(t *testing.T) {
	ch := db.Changes{
		Deletes: [][]string{{"7", "it's"}},
		Updates: []db.Row{{Key: []string{"3", `a\b`}, Cols: []db.Change{
			{Name: "qty", Val: db.Val{S: "9"}, Old: db.Val{S: "1"}},
			{Name: "note", Val: db.Val{Null: true}, Old: db.Val{Null: true}},
			{Name: "sku", Default: true, Old: db.Val{S: "x"}},
		}}},
		Inserts: [][]db.Change{{{Name: "a", Val: db.Val{S: "1"}}, {Name: "c", Val: db.Val{Null: true}}}, nil},
	}
	pg := []string{
		`delete from "s"."t" where "id" = '7' and "name" = 'it''s'`,
		`update "s"."t" set "qty" = '9', "note" = NULL, "sku" = DEFAULT where "id" = '3' and "name" = 'a\b' and format('%s', "qty") = '1' and "note" is null and format('%s', "sku") = 'x'`,
		`insert into "s"."t" ("a", "c") values ('1', NULL)`,
		`insert into "s"."t" default values`,
	}
	if got := db.Preview(postgres.Engine{}, "s", "t", []string{"id", "name"}, ch); !reflect.DeepEqual(got, pg) {
		t.Errorf("PG:\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(pg, "\n"))
	}
	if got := db.Preview(mysql.Engine{}, "s", "t", []string{"id", "name"}, ch); !strings.Contains(got[1], `'a\\b'`) || !strings.Contains(got[0], "'it''s'") {
		t.Errorf("MySQL: %q", got)
	}
	for _, e := range []db.Engine{postgres.Engine{}, mysql.Engine{}, sqlite.Engine{}} {
		previewed := db.Preview(e, "s", "t", []string{"id", "name"}, ch)
		for i, s := range db.Statements(e, "s", "t", []string{"id", "name"}, ch) {
			lits := make([]string, len(s.Args))
			for j, a := range s.Args {
				lits[j] = "NULL"
				if !a.Null {
					lits[j] = sqlkit.Literal(a.S, e.Dialect())
				}
			}
			text := s.SQL
			if e.Dialect() == sqlkit.MySQL { // ?, in order
				for _, l := range lits {
					text = strings.Replace(text, "?", l, 1)
				}
			} else { // $1, $2… or ?1, ?2…: every one, from the last, so $1 is not in $10
				mark := map[sqlkit.Dialect]string{sqlkit.PG: "$", sqlkit.SQLite: "?"}[e.Dialect()]
				for j := len(lits); j >= 1; j-- {
					text = strings.ReplaceAll(text, mark+strconv.Itoa(j), lits[j-1])
				}
			}
			if text != previewed[i] {
				t.Errorf("%T statement %d: numbered with its arguments in %q, previewed %q", e, i, text, previewed[i])
			}
		}
	}
}
