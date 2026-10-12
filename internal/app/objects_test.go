package app

import (
	"slices"
	"strings"
	"testing"

	"github.com/charmbracelet/x/exp/golden"

	"github.com/kackerx/sqlmux/internal/db"
)

// inObjects is the tree with t_order opened, its columns, indexes and keys
// in, and a schema's functions and sequences: the groups of them closed.
func inObjects(t *testing.T) *App {
	t.Helper()
	a := inTree(45)
	c := a.sess.Conns[0]
	order := tableNamed(a, "t_order")
	c.cols[idOf(order)] = db.Columns{PK: []string{"id"}, Cols: []db.Column{{Name: "id", Type: "bigint"}, {Name: "user_id", Type: "bigint"}}}
	c.idx[idOf(order)] = []db.Index{
		{Name: "t_order_pkey", Cols: []string{"id"}, Unique: true, Def: "CREATE UNIQUE INDEX t_order_pkey ON public.t_order USING btree (id);"},
		{Name: "t_order_user", Cols: []string{"user_id", "created_at"}, Def: "CREATE INDEX t_order_user ON public.t_order USING btree (user_id, created_at DESC);"},
	}
	c.keys[idOf(order)] = []db.Key{
		{Name: "t_order_pkey", Cols: []string{"id"}},
		{Name: "t_order_user_id_fkey", Cols: []string{"user_id"}, Ref: &db.Ref{Schema: "public", Table: "t_user", Cols: []string{"id"}}},
		{Name: "t_order_agent_fkey", Cols: []string{"agent_id"}, Ref: &db.Ref{Schema: "agentable", Table: "planner", Cols: []string{"id"}}},
	}
	c.funcs = []db.Function{{Schema: "public", Name: "fn_total", Args: "order_id bigint", ID: "16400"}, {Schema: "public", Name: "fn_greet", Args: "who text, times integer", ID: "16401"}}
	c.seqs = []db.Sequence{{Schema: "public", Name: "invoice_no"}}
	treeTo(t, a, "t_order")
	feed(t, a, "l")
	return a
}

// A table lists its columns, then Indexes (n) and Keys (n), closed; a schema
// its Tables and Views, then Functions (n) and Sequences (n) when it has
// some; a filter keeps only the tables (F14.3).
func TestTreeObjects(t *testing.T) {
	a := inObjects(t)
	got := treeTexts(a)
	for _, want := range []string{
		"          id\n          user_id\n        ▸ Indexes (2)\n        ▸ Keys (3)\n      ▸ t_order_item",
		"      Views (0)\n    ▸ Functions (2)\n    ▸ Sequences (1)\n",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("tree has no %q:\n%s", want, got)
		}
	}
	treeTo(t, a, "Indexes (2)")
	feed(t, a, "l")
	treeTo(t, a, "Keys (3)")
	feed(t, a, "l")
	treeTo(t, a, "Functions (2)")
	feed(t, a, "l")
	treeTo(t, a, "Sequences (1)")
	feed(t, a, "l")
	for _, want := range []string{
		"▾ Indexes (2)\n            t_order_pkey (id) UNIQUE\n            t_order_user (user_id, created_at)\n",
		"▾ Keys (3)\n            t_order_pkey (id)\n            t_order_user_id_fkey (user_id) → public.t_user (id)\n            t_order_agent_fkey (agent_id) → agentable.planner (id)\n",
		"▾ Functions (2)\n        fn_total(order_id bigint)\n        fn_greet(who text, times integer)\n",
		"▾ Sequences (1)\n        invoice_no\n",
	} {
		if !strings.Contains(treeTexts(a), want) {
			t.Fatalf("tree has no %q:\n%s", want, treeTexts(a))
		}
	}
	golden.RequireEqual(t, a.render().String())
	feed(t, a, "/order")
	if got := treeTexts(a); strings.Contains(got, "Indexes") || strings.Contains(got, "Functions") || strings.Contains(got, "fn_total") {
		t.Errorf("a filter keeps tables and views only:\n%s", got)
	}
}

// ↵ on an index shows its statement in the DDL popup, a foreign key opens its
// table, another schema's with its schema opened in the tree, the primary key
// and a sequence do nothing, a function reads its definition (F14.3).
func TestTreeObjectsOpen(t *testing.T) {
	a := inObjects(t)
	for _, g := range []string{"Indexes (2)", "Keys (3)"} {
		treeTo(t, a, g)
		feed(t, a, "l")
	}
	treeTo(t, a, "t_order_user (user_id, created_at)")
	feed(t, a, "<CR>")
	if a.ddl == nil || a.ddl.label != "t_order_user" || !strings.Contains(a.render().String(), "CREATE INDEX t_order_user") {
		t.Fatalf("index popup: %+v", a.ddl)
	}
	feed(t, a, "<Esc>")
	if a.ddl != nil {
		t.Fatal("esc closes it")
	}
	// the primary key: the key leaf, not the index of it
	ns, _ := a.treeNodes()
	a.treeGo(slices.IndexFunc(ns, func(x node) bool { return x.kind == nodeKey && x.key.Ref == nil }))
	if cmd := a.treeOpen(false); cmd != nil || a.ddl != nil {
		t.Errorf("↵ on PRIMARY does nothing: %v", cmd)
	}
	treeTo(t, a, "t_order_agent_fkey (agent_id) → agentable.planner (id)")
	delete(a.sess.open, schemaNode(a.sess.Conns[0], "agentable"))
	feed(t, a, "<CR>")
	if d := dataOf(a.focused()); d == nil || d.table.Name != "planner" || !a.sess.opened(schemaNode(a.sess.Conns[0], "agentable"), false) {
		t.Fatalf("↵ on a foreign key: %v", dataOf(a.focused()))
	}
	a.win().focus(0)
	treeTo(t, a, "Functions (2)")
	feed(t, a, "l")
	treeTo(t, a, "fn_greet(who text, times integer)")
	fn, _ := a.treeNode()
	if fn.kind != nodeFunc || fn.fn.ID != "16401" {
		t.Fatalf("function leaf: %+v", fn.fn)
	}
	a.Update(funcMsg{a.sess.Conns[0], fn.fn, "CREATE FUNCTION public.fn_greet(who text) ...;", nil})
	q := a.focusedQuery()
	if q == nil || text(q) != "CREATE FUNCTION public.fn_greet(who text) ...;" || q.schema != "public" {
		t.Fatalf("function definition in a new query: %v", q)
	}
	a.win().focus(0)
	treeTo(t, a, "Sequences (1)")
	feed(t, a, "l")
	treeTo(t, a, "invoice_no")
	if cmd := a.treeOpen(false); cmd != nil {
		t.Errorf("↵ on a sequence does nothing")
	}
}

// A table opened in the tree asks for its indexes and keys with its columns;
// R drops them (F14.3).
func TestObjectsFetchedWithTheTable(t *testing.T) {
	a := inTree(45)
	c := a.sess.Conns[0]
	treeTo(t, a, "t_user")
	_, cmd := a.Update(teaKey("l"))
	var asked []string
	for _, m := range batchMsgs(cmd) {
		switch m := m.(type) {
		case colsMsg:
			asked = append(asked, "cols")
		case objectsMsg:
			asked = append(asked, "objects "+m.table.Name)
		}
	}
	if strings.Join(asked, ",") != "cols,objects t_user" {
		t.Fatalf("opening t_user asked for %v", asked)
	}
	user := tableNamed(a, "t_user")
	a.Update(objectsMsg{conn: c, table: user, idx: []db.Index{{Name: "i", Cols: []string{"a"}}}})
	if len(c.idx[idOf(user)]) != 1 {
		t.Fatal("the indexes are kept")
	}
	a.Update(teaKey("R"))
	if len(c.idx) != 0 || len(c.keys) != 0 {
		t.Errorf("R drops them: %v %v", c.idx, c.keys)
	}
}
