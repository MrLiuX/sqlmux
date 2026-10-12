//go:build integration

package mysql

import (
	"context"
	"strings"
	"testing"

	"github.com/kackerx/sqlmux/internal/db"
)

// keyTexts is a function of what Keys gives, the keys as text: name(cols)->schema.table(cols).
func keyTexts(t *testing.T) func([]db.Key, error) []string {
	return func(ks []db.Key, err error) (out []string) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
		for _, k := range ks {
			s := k.Name + "(" + strings.Join(k.Cols, ",") + ")"
			if k.Ref != nil {
				s += "->" + k.Ref.Schema + "." + k.Ref.Table + "(" + strings.Join(k.Ref.Cols, ",") + ")"
			}
			out = append(out, s)
		}
		return out
	}
}

// A table's indexes, PRIMARY first, with a statement made of their columns, an
// expression in parentheses; its keys, the primary key and the foreign keys
// with what they point at (F14.3).
func TestIndexesAndKeys(t *testing.T) {
	dsn := OwnDB(t)
	c, _, err := Connect(context.Background(), dsn, "", true)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	cfg, _ := Config(dsn, "")
	ctx, name := context.Background(), cfg.DBName
	ixs, err := my.Indexes(ctx, c, name, "t_order")
	if err != nil || len(ixs) != 2 || ixs[0].Name != "PRIMARY" || !ixs[0].Unique || ixs[1].Name != "t_order_user_created" || ixs[1].Unique { // the foreign key on user_id has an index of this one's
		t.Fatalf("t_order: %+v %v", ixs, err)
	}
	for i, want := range []string{
		"ALTER TABLE `" + name + "`.`t_order` ADD PRIMARY KEY (`id`);",
		"CREATE INDEX `t_order_user_created` ON `" + name + "`.`t_order` (`user_id`, `created_at`);",
	} {
		if ixs[i].Def != want {
			t.Errorf("def %d: %q, want %q", i, ixs[i].Def, want)
		}
	}
	sku, _ := my.Indexes(ctx, c, name, "t_sku")
	var got []string
	for _, ix := range sku {
		got = append(got, ix.Name+"("+strings.Join(ix.Cols, ",")+")")
	}
	if want := "t_sku_barcode(barcode) t_sku_code(code) t_sku_lower_title((lower(`title`))) t_sku_title_code(title,code)"; strings.Join(got, " ") != want {
		t.Errorf("t_sku: %q, want %q", got, want)
	}
	for _, ix := range sku {
		if ix.Name == "t_sku_lower_title" && !strings.Contains(ix.Def, "((lower(`title`)))") {
			t.Errorf("an expression in a statement: %s", ix.Def)
		}
	}
	if got, want := keyTexts(t)(my.Keys(ctx, c, name, "t_order_item")), "PRIMARY(order_id,line_no) t_order_item_ibfk_1(order_id)->"+name+".t_order(id)"; strings.Join(got, " ") != want {
		t.Errorf("keys %q, want %q", got, want)
	}
	if got := keyTexts(t)(my.Keys(ctx, c, name, "t_log")); len(got) != 0 {
		t.Errorf("no keys: %q", got)
	}
}

// Functions with their parameters, by name; a definition by SHOW CREATE
// FUNCTION; no sequences (F14.3).
func TestFunctions(t *testing.T) {
	c := connect(t, true) // the shared database: the seed made the functions as root
	cfg, _ := Config(IntegrationDSN(t), "")
	ctx := context.Background()
	fs, err := my.Functions(ctx, c)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, f := range fs {
		got = append(got, f.Schema+"."+f.Name+"("+f.Args+")")
	}
	if want := "agentable.fn_agent_count() " + cfg.DBName + ".fn_greet(who varchar(64)) " + cfg.DBName + ".fn_order_total(order_id bigint)"; strings.Join(got, " ") != want {
		t.Fatalf("functions %q, want %q", got, want)
	}
	def, err := my.FunctionDef(ctx, c, db.Function{Schema: cfg.DBName, Name: "fn_greet", ID: "fn_greet"})
	if err != nil || !strings.Contains(def, "CREATE DEFINER=") || !strings.Contains(def, "FUNCTION `fn_greet`") || !strings.HasSuffix(def, ";") {
		t.Errorf("def %q %v", def, err)
	}
	if ss, err := my.Sequences(ctx, c); err != nil || ss != nil {
		t.Errorf("sequences %v %v", ss, err)
	}
}
