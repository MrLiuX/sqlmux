package db_test

import (
	"reflect"
	"testing"

	"github.com/kackerx/sqlmux/internal/db"
	"github.com/kackerx/sqlmux/internal/db/mysql"
	"github.com/kackerx/sqlmux/internal/db/postgres"
	"github.com/kackerx/sqlmux/internal/db/sqlite"
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
