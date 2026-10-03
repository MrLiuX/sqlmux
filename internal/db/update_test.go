package db

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
)

// pgSQL writes SQL as PG's engine does; the rest of Engine it leaves out.
type pgSQL struct{ Engine }

func (pgSQL) Quote(names ...string) string { return `"` + strings.Join(names, `"."`) + `"` }
func (pgSQL) Arg(n int) string             { return fmt.Sprintf("$%d", n) }
func (pgSQL) Same(col, arg string) string  { return "format('%s', " + col + ") = " + arg }

// One UPDATE a row: DEFAULT as the keyword, the row identity with =, every
// changed column still holding what was loaded as the engine's Same has
// it, or IS NULL (§10.3).
func TestUpdateSQL(t *testing.T) {
	sql, args := update(pgSQL{}, "public", "t_order_item", []string{"order_id", "line_no"}, Row{Key: []string{"3", "2"}, Cols: []Change{
		{Name: "qty", Val: Val{S: "9"}, Old: Val{S: "1"}},
		{Name: "sku code", Default: true, Old: Val{Null: true}},
	}})
	want := `update "public"."t_order_item" set "qty" = $1, "sku code" = DEFAULT where "order_id" = $2 and "line_no" = $3` +
		` and format('%s', "qty") = $4 and "sku code" is null`
	if sql != want {
		t.Errorf("sql:\n%s\nwant\n%s", sql, want)
	}
	if !reflect.DeepEqual(args, []Val{{S: "9"}, {S: "3"}, {S: "2"}, {S: "1"}}) {
		t.Errorf("args %v", args)
	}
}
