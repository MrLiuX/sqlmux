//go:build integration

package mssql

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/kackerx/sqlmux/internal/db"
	"github.com/kackerx/sqlmux/internal/sqlkit"
)

// The statements an AutoLimit does not append to run as they are, and the
// ones it does run with the OFFSET/FETCH: none of them errors on the
// server (§9.4, F18.6).
func TestAutoLimitRuns(t *testing.T) {
	c := connect(t)
	ctx := context.Background()
	for _, s := range []string{
		"select top 3 id from t_order",
		"select top 2 id from t_order order by id",
		"select distinct status from t_order",
		"select status from t_order union select status from t_order",
		"select id from t_order intersect select id from t_order",
		"select id from t_order for json path",
		"select count(*) from t_order",
		"with x as (select top 2 id from t_order) select * from x",
		"select id from t_order order by id",
	} {
		text, _ := sqlkit.AutoLimit(s, sqlkit.MSSQL, 101, 0)
		if _, err := c.Query(ctx, text); err != nil {
			t.Errorf("%s: %v\n    as %q", s, err, text)
		}
	}
}

// A procedure that goes without its EXEC runs, and one that answers with
// rows has them (§9.2, F18.6).
func TestBareProc(t *testing.T) {
	c := connect(t)
	ctx := context.Background()
	r, err := c.Query(ctx, "sp_help 'dbo.t_order'")
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Rows) == 0 {
		t.Errorf("sp_help: %d rows", len(r.Rows))
	}
}

// Each type: read it as the grid does, then save the same column with Old =
// what was read — a stale answer means the value the grid shows can never
// be saved (§10.3, F18.6 g).
func TestSaveEachType(t *testing.T) {
	dsn := OwnDB(t)
	w, e := workerOf(t, dsn), Engine{}
	ctx := context.Background()
	cols := []struct{ typ, val, set, back string }{
		{"real", "0.1", "0.25", ""},
		{"float", "0.1", "0.25", ""},
		{"datetime", "'2026-01-02T03:04:05.123'", "2026-01-02T03:04:06.000", ""},
		{"smalldatetime", "'2026-01-02 03:04'", "2026-01-02T03:05:00", ""},
		{"datetime2(0)", "'2026-01-02T03:04:05'", "2026-01-02T03:04:06", ""},
		{"datetime2(3)", "'2026-01-02T03:04:05.123'", "2026-01-02T03:04:06.000", ""},
		{"datetime2", "'2026-01-02T03:04:05.1234567'", "2026-01-02T03:04:06.0000000", ""},
		{"time(0)", "'03:04:05'", "03:04:06", ""},
		{"time", "'03:04:05.1234567'", "03:04:06.0000000", ""},
		{"date", "'2026-01-02'", "2026-01-03", ""},
		{"datetimeoffset(2)", "'2026-01-02T03:04:05.12+08:00'", "2026-01-02T03:04:06.00+08:00", ""},
		{"money", "12.3456", "1.5", "1.5000"},       // the scale is the type's
		{"smallmoney", "12.3456", "1.5", "1.5000"},  //
		{"decimal(10,3)", "12.345", "1.5", "1.500"}, //
		{"numeric(5,0)", "123", "124", ""},
		{"bigint", "9007199254740993", "1", ""},
		{"tinyint", "7", "8", ""},
		{"bit", "1", "false", ""},
		{"bit", "0", "true", ""},
		{"uniqueidentifier", "'6F9619FF-8B86-D011-B42D-00C04FC964FF'", "00000000-0000-0000-0000-000000000001", ""},
		{"xml", "'<a>1</a>'", "<b>2</b>", ""},
		{"nvarchar(max)", "N'中文 text'", "x", ""},
		{"varchar(10)", "'abc'", "abd", ""},
		{"char(5)", "'ab'", "cd", "cd   "}, // the padding is the type's
		{"nchar(3)", "N'ab'", "cd", "cd "}, //
		{"sql_variant", "cast(5 as int)", "6", ""},
	}
	for i, c := range cols {
		tab := fmt.Sprintf("zz_t%d", i)
		if _, err := w.Query(ctx, fmt.Sprintf("create table dbo.%s (id int primary key, v %s null)", tab, c.typ)); err != nil {
			t.Errorf("%s: create: %v", c.typ, err)
			continue
		}
		if _, err := w.Query(ctx, fmt.Sprintf("insert into dbo.%s values (1, %s)", tab, c.val)); err != nil {
			t.Errorf("%s: insert: %v", c.typ, err)
			continue
		}
		r, err := w.Query(ctx, fmt.Sprintf("select v from dbo.%s where id = 1", tab))
		if err != nil {
			t.Errorf("%s: read: %v", c.typ, err)
			continue
		}
		old := r.Rows[0][0]
		ch := db.Changes{Updates: []db.Row{{Key: []string{"1"}, Cols: []db.Change{
			{Name: "v", Val: db.Val{S: c.set}, Old: old},
		}}}}
		if failed, _, err := db.Save(ctx, w, e, "dbo", tab, []string{"id"}, ch, false); failed != -1 || err != nil {
			t.Errorf("%s: shown %q: failed %d, err %v", c.typ, old.S, failed, err)
			continue
		}
		r2, err := w.Query(ctx, fmt.Sprintf("select v from dbo.%s where id = 1", tab))
		if err != nil {
			t.Errorf("%s: re-read: %v", c.typ, err)
			continue
		}
		want := c.set
		if c.back != "" {
			want = c.back // what the type's scale or padding makes of it
		}
		if after := r2.Rows[0][0].S; after != want {
			t.Errorf("%s: saved %q, read back %q", c.typ, c.set, after)
		}
	}
}

// The optimistic check catches a row changed under the grid (§10.3,
// F18.6): Old as the grid read it, another session writes another value,
// the save says so — and one that did not change goes in.
// ponytail: a text column whose value is all of 'true' or 'false' slips it
// when another session made it '1' or '0' — the bit's third arm reads it
// as the bit's, the last case here
func TestStaleCaught(t *testing.T) {
	dsn := OwnDB(t)
	w, e := workerOf(t, dsn), Engine{}
	ctx := context.Background()
	for i, c := range []struct {
		typ, was, other, set string
		goes                 bool // the save goes in, the row having changed all the same
	}{
		{"int", "5", "1", "7", false},
		{"int", "0", "0", "7", true},
		{"bigint", "-3", "1", "7", false},
		{"decimal(5,2)", "2.50", "1.00", "7", false},
		{"varchar(10)", "'true'", "'1'", "x", true},
		{"varchar(10)", "'yes'", "'1'", "x", false},
		{"bit", "1", "0", "false", false},
		{"bit", "1", "1", "false", true},
	} {
		tab := fmt.Sprintf("zz_s%d", i)
		if _, err := w.Query(ctx, fmt.Sprintf("create table dbo.%s (id int primary key, v %s)", tab, c.typ)); err != nil {
			t.Fatal(err)
		}
		if _, err := w.Query(ctx, fmt.Sprintf("insert into dbo.%s values (1, %s)", tab, c.was)); err != nil {
			t.Fatal(err)
		}
		r, err := w.Query(ctx, "select v from dbo."+tab)
		if err != nil {
			t.Fatal(err)
		}
		old := r.Rows[0][0]
		if _, err := w.Query(ctx, fmt.Sprintf("update dbo.%s set v = %s", tab, c.other)); err != nil { // someone else
			t.Fatal(err)
		}
		ch := db.Changes{Updates: []db.Row{{Key: []string{"1"}, Cols: []db.Change{
			{Name: "v", Val: db.Val{S: c.set}, Old: old},
		}}}}
		failed, _, err := db.Save(ctx, w, e, "dbo", tab, []string{"id"}, ch, false)
		if c.goes {
			if failed != -1 || err != nil {
				t.Errorf("%s read %q then %s: failed %d, err %v, want the save in", c.typ, old.S, c.other, failed, err)
			}
			continue
		}
		if failed != 0 || !errors.Is(err, db.ErrStale) {
			t.Errorf("%s read %q then %s: failed %d, err %v, want the row changed", c.typ, old.S, c.other, failed, err)
		}
	}
}

// The catalog's types are the column's, not the type's defaults, and the
// DDL is what the columns say (§8.4, F18.6).
func TestColumnTypes(t *testing.T) {
	dsn := OwnDB(t)
	w := workerOf(t, dsn)
	ctx := context.Background()
	if _, err := w.Query(ctx, "create table dbo.zz_types (id int primary key, d decimal(10,3), n numeric(5,0), t2 datetime2(0), tm time(3), dto datetimeoffset(2), vc varchar(20), nv nvarchar(30))"); err != nil {
		t.Fatal(err)
	}
	cols, err := (Engine{}).TableColumns(ctx, w, "dbo", "zz_types")
	if err != nil {
		t.Fatal(err)
	}
	types := map[string]string{}
	for _, col := range cols.Cols {
		types[col.Name] = col.Type
	}
	for name, want := range map[string]string{
		"d": "decimal(10, 3)", "n": "numeric(5, 0)", "t2": "datetime2(0)",
		"tm": "time(3)", "dto": "datetimeoffset(2)", "vc": "varchar(20)", "nv": "nvarchar(30)",
	} {
		if types[name] != want {
			t.Errorf("%s = %q, want %q", name, types[name], want)
		}
	}
	d, err := (Engine{}).DDL(ctx, w, "dbo", "zz_types")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"[d] decimal(10, 3)", "[t2] datetime2(0)", "[n] numeric(5, 0)", "[tm] time(3)", "[dto] datetimeoffset(2)"} {
		if !strings.Contains(d, want) {
			t.Errorf("the ddl misses %q:\n%s", want, d)
		}
	}
}

// Quick on a big result stops at the AutoLimit's row instead of reading it
// all (§9.4, §12, F18.6).
func TestQuickBound(t *testing.T) {
	c := connect(t)
	w := db.NewWorker(c)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	start := time.Now()
	r, err := (Engine{}).Quick(ctx, w, "select a.object_id, b.name from sys.all_objects a cross join sys.all_objects b", "", 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Rows) != 100 || !r.Truncated {
		t.Fatalf("rows %d, truncated %v", len(r.Rows), r.Truncated)
	}
	if took := time.Since(start); took > 10*time.Second {
		t.Fatalf("the quick took %v", took)
	}
}

// A save in a transaction of the user's goes to its savepoint: one that
// works writes and stays in the transaction, one that fails takes itself
// back and leaves what came before (§10.3, F18.6).
func TestSaveInATransaction(t *testing.T) {
	dsn := OwnDB(t)
	w, e := workerOf(t, dsn), Engine{}
	ctx := context.Background()
	r, err := w.Query(ctx, "select email from dbo.t_user where id = 3")
	if err != nil {
		t.Fatal(err)
	}
	first := r.Rows[0][0]
	if _, err := w.Query(ctx, "begin transaction"); err != nil {
		t.Fatal(err)
	}
	// the save that works, in the transaction's savepoint
	ch := db.Changes{Updates: []db.Row{{Key: []string{"3"}, Cols: []db.Change{
		{Name: "email", Val: db.Val{S: "user_3@saved.example"}, Old: first},
	}}}}
	failed, tx, err := db.Save(ctx, w, e, "dbo", "t_user", []string{"id"}, ch, false)
	if failed != -1 || err != nil {
		t.Fatalf("the save: failed %d, err %v", failed, err)
	}
	if tx != 'T' {
		t.Fatalf("after the save: tx %c", tx)
	}
	// the save that fails, its own alone taken back
	failed, tx, err = db.Save(ctx, w, e, "dbo", "t_user", []string{"id"}, ch, false)
	if failed != 0 || !errors.Is(err, db.ErrStale) {
		t.Fatalf("the stale save: failed %d, err %v", failed, err)
	}
	if tx != 'T' {
		t.Fatalf("after the stale save: tx %c", tx)
	}
	r, err = w.Query(ctx, "select email from dbo.t_user where id = 3")
	if err != nil || r.Rows[0][0].S != "user_3@saved.example" {
		t.Fatalf("what the first save wrote is gone: %v, %q", err, r.Rows[0][0].S)
	}
	if _, err := w.Query(ctx, "rollback transaction"); err != nil {
		t.Fatal(err)
	}
	r, err = w.Query(ctx, "select email from dbo.t_user where id = 3")
	if err != nil || r.Rows[0][0].S != first.S {
		t.Fatalf("the rollback: %v, %q", err, r.Rows[0][0].S)
	}
}
