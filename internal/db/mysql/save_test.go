//go:build integration

package mysql

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"sqlmux/internal/db"
)

// saver is a Worker on a database of the test's own, a second session on
// it that reads, and changes rows behind the saver's back, and its DSN.
func saver(t *testing.T) (*db.Worker, func(sql string) string, string) {
	t.Helper()
	ctx := context.Background()
	dsn := OwnDB(t)
	main, _, err := Connect(ctx, dsn, "", false)
	if err != nil {
		t.Fatal(err)
	}
	w := db.NewWorker(main)
	t.Cleanup(func() { w.Close() })
	other, _, err := Connect(ctx, dsn, "", false)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { other.Close() })
	// the first row's values as text, | between, NULL as <null>
	return w, func(sql string) string {
		t.Helper()
		r, err := other.Query(ctx, sql)
		if err != nil {
			t.Fatal(err)
		}
		if len(r.Rows) == 0 { // DML
			return ""
		}
		var vs []string
		for _, v := range r.Rows[0] {
			if v.Null {
				v.S = "<null>"
			}
			vs = append(vs, v.S)
		}
		return strings.Join(vs, "|")
	}, dsn
}

func text(s string) db.Val { return db.Val{S: s} }

func dbName(dsn string) string {
	cfg, _ := Config(dsn, "")
	return cfg.DBName
}

// Rows found by a primary key, a composite one or a unique index take text,
// NULL and DEFAULT; a json column's loaded value compares as its text; one
// set to what it holds is still found (§10.3).
func TestSave(t *testing.T) {
	ctx := context.Background()
	w, q, dsn := saver(t)
	q("update t_user set active = false where id = 1")
	rows := []struct {
		table string
		key   []string
		row   db.Row
		after string
	}{
		{"t_user", []string{"id"}, db.Row{Key: []string{"1"}, Cols: []db.Change{
			{Name: "name", Val: text("renamed"), Old: text("user_1")},
			{Name: "email", Val: db.Val{Null: true}, Old: text("user_1@example.com")},
			{Name: "active", Default: true, Old: text("0")},
		}}, "select name, email, active from t_user where id = 1"},
		{"t_order_item", []string{"order_id", "line_no"}, db.Row{Key: []string{"3", "2"}, Cols: []db.Change{
			{Name: "qty", Val: text("9"), Old: text("1")},
		}}, "select qty from t_order_item where order_id = 3 and line_no = 2"},
		{"t_sku", []string{"code"}, db.Row{Key: []string{"sku_1"}, Cols: []db.Change{
			{Name: "title", Val: text("SKU one"), Old: text("SKU 1")},
		}}, "select title from t_sku where code = 'sku_1'"},
		{"t_order", []string{"id"}, db.Row{Key: []string{"4"}, Cols: []db.Change{
			{Name: "raw", Val: text(`{"n": 40}`), Old: text(`{"n": 4}`)},
		}}, "select raw from t_order where id = 4"},
		{"t_order", []string{"id"}, db.Row{Key: []string{"5"}, Cols: []db.Change{
			{Name: "amount", Val: text("5.99"), Old: text("5.99")},
		}}, "select amount from t_order where id = 5"},
	}
	want := []string{`renamed|<null>|1`, `9`, `SKU one`, `{"n": 40}`, `5.99`}
	for i, c := range rows {
		if failed, _, err := db.Save(ctx, w, my, dbName(dsn), c.table, c.key, db.Changes{Updates: []db.Row{c.row}}, false); err != nil {
			t.Fatalf("%s: row %d: %v", c.table, failed, err)
		}
		if got := q(c.after); got != want[i] {
			t.Errorf("%s: %s, want %s", c.table, got, want[i])
		}
	}
}

// A row changed or gone since it was loaded fails the whole save: nothing
// is written, and failed says which row (§10.3). So does a database error,
// and a change of case, which the column's collation would let by.
func TestSaveRollsBack(t *testing.T) {
	ctx := context.Background()
	w, q, dsn := saver(t)
	name := dbName(dsn)
	ok := db.Row{Key: []string{"1"}, Cols: []db.Change{{Name: "note", Val: text("saved"), Old: db.Val{Null: true}}}}
	stale := db.Row{Key: []string{"2"}, Cols: []db.Change{{Name: "note", Val: text("mine"), Old: db.Val{Null: true}}}}
	q("update t_order set note = 'theirs' where id = 2")
	if failed, _, err := db.Save(ctx, w, my, name, "t_order", []string{"id"}, db.Changes{Updates: []db.Row{ok, stale}}, false); !errors.Is(err, db.ErrStale) || failed != 1 {
		t.Fatalf("changed behind: row %d, %v", failed, err)
	}
	q("update t_user set name = 'USER_3' where id = 3")
	cased := db.Row{Key: []string{"3"}, Cols: []db.Change{{Name: "name", Val: text("mine"), Old: text("user_3")}}}
	if failed, _, err := db.Save(ctx, w, my, name, "t_user", []string{"id"}, db.Changes{Updates: []db.Row{cased}}, false); !errors.Is(err, db.ErrStale) || failed != 0 {
		t.Fatalf("its case changed behind: row %d, %v", failed, err)
	}
	bad := db.Row{Key: []string{"3"}, Cols: []db.Change{{Name: "amount", Val: text("abc"), Old: text("3.99")}}}
	if failed, _, err := db.Save(ctx, w, my, name, "t_order", []string{"id"}, db.Changes{Updates: []db.Row{ok, bad}}, false); errno(err) != 1366 || failed != 1 {
		t.Fatalf("bad input: row %d, %v", failed, err)
	}
	if got := q("select coalesce(note, '<null>') from t_order where id = 1"); got != "<null>" {
		t.Errorf("row 1 was written: %s", got)
	}
}

// Cancelling stops the UPDATEs, rolls back and leaves the connection in
// use (§10.3).
func TestSaveCancel(t *testing.T) {
	ctx := context.Background()
	w, q, dsn := saver(t)
	lock, _, err := Connect(ctx, dsn, "", false)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()
	for _, s := range []string{"begin", "select 1 from t_order where id = 5 for update"} {
		if _, err := lock.Exec(ctx, s, 0); err != nil {
			t.Fatal(err)
		}
	}
	done := make(chan error, 1)
	go func() {
		row := db.Row{Key: []string{"5"}, Cols: []db.Change{{Name: "note", Val: text("x"), Old: db.Val{Null: true}}}}
		_, _, err := db.Save(ctx, w, my, dbName(dsn), "t_order", []string{"id"}, db.Changes{Updates: []db.Row{row}}, false)
		done <- err
	}()
	for q("select count(*) from information_schema.processlist where info like 'update%'") == "0" { // waiting on the lock
		time.Sleep(20 * time.Millisecond)
	}
	w.Cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled: %v", err)
	}
	lock.Exec(ctx, "rollback", 0)
	if got := q("select coalesce(note, '<null>') from t_order where id = 5"); got != "<null>" {
		t.Errorf("written: %s", got)
	}
	if r, err := w.Query(ctx, "select 1"); err != nil || r.Rows[0][0].S != "1" {
		t.Errorf("after the cancel: %v", err)
	}
}

// What a row was loaded as, the text the server sent for each type, is
// what CAST AS BINARY finds in it: floats, decimals, times with their
// fractions, binaries, enums and sets, char without its padding, json,
// booleans, unsigned, latin1 text past ASCII, and NULL (§10.3). A BIT,
// shown as a number, is not edited (§8.1).
func TestSaveChecksEveryType(t *testing.T) {
	ctx := context.Background()
	w, _, dsn := saver(t)
	for _, s := range []string{
		`create table types (id int primary key, f float, d double, n decimal(10, 2), dt datetime(3), ts timestamp(6) null,
			da date, tm time, y year, b bit(8), bn binary(4), vb varbinary(8), e enum('a', 'b'), st set('x', 'y'),
			c char(5), j json, bo boolean, bu bigint unsigned, tx text, nul varchar(8), la varchar(8) character set latin1)`,
		`insert into types values (1, 3.99, 0.1, 1.50, '2026-09-28 10:00:00.5', '2026-09-28 10:00:00.000001',
			'2026-09-28', '12:34:56', 2026, b'101', 'ab', 'xy', 'b', 'x,y', 'ab  ', '{"b": 1, "a": [1, 2]}', true,
			18446744073709551615, 'Abc', null, 'café')`,
	} {
		if _, err := w.Exec(ctx, s, 0); err != nil {
			t.Fatal(err)
		}
	}
	r, err := w.Query(ctx, "select * from types")
	if err != nil {
		t.Fatal(err)
	}
	for i, c := range r.Cols[1:] {
		old := r.Rows[0][i+1]
		if c.Type == "bit" {
			if old.S != "5" {
				t.Errorf("b'101' shown as %q", old.S)
			}
			continue
		}
		row := db.Row{Key: []string{"1"}, Cols: []db.Change{{Name: c.Name, Val: db.Val{Null: true}, Old: old}}}
		if !old.Null {
			row.Cols[0].Val = old // written back as it was: only the check is under test
		}
		if _, _, err := db.Save(ctx, w, my, dbName(dsn), "types", []string{"id"}, db.Changes{Updates: []db.Row{row}}, false); err != nil {
			t.Errorf("%s loaded as %+v: %v", c.Name, old, err)
		}
	}
}

// Deletes, updates and inserts in one save, in that order (§10.6); a row
// with nothing set goes in as one column DEFAULT.
func TestSaveInsertsAndDeletes(t *testing.T) {
	ctx := context.Background()
	w, q, dsn := saver(t)
	name := dbName(dsn)
	if _, err := w.Exec(ctx, "create table all_default (id bigint auto_increment primary key, n int default 7)", 0); err != nil {
		t.Fatal(err)
	}
	ch := db.Changes{
		Deletes: [][]string{{"3", "1"}},
		Updates: []db.Row{{Key: []string{"3", "2"}, Cols: []db.Change{{Name: "qty", Val: text("9"), Old: text("1")}}}},
		Inserts: [][]db.Change{{{Name: "order_id", Val: text("3")}, {Name: "line_no", Val: text("99")}, {Name: "sku_code", Val: text("sku_1")}, {Name: "qty", Default: true}}},
	}
	if failed, _, err := db.Save(ctx, w, my, name, "t_order_item", []string{"order_id", "line_no"}, ch, false); err != nil {
		t.Fatalf("statement %d: %v", failed, err)
	}
	if got := q("select (select count(*) from t_order_item where order_id = 3 and line_no = 1), (select qty from t_order_item where order_id = 3 and line_no = 2), (select qty from t_order_item where order_id = 3 and line_no = 99)"); got != "0|9|1" {
		t.Errorf("deleted, updated, inserted with qty's default: %s", got)
	}
	if _, _, err := db.Save(ctx, w, my, name, "all_default", []string{"id"}, db.Changes{Inserts: [][]db.Change{nil}}, false); err != nil {
		t.Fatal(err)
	}
	if got := q("select n from all_default"); got != "7" {
		t.Errorf("default values: %s", got)
	}
	gone := db.Changes{Deletes: [][]string{{"4", "1"}, {"999999", "1"}}}
	if failed, _, err := db.Save(ctx, w, my, name, "t_order_item", []string{"order_id", "line_no"}, gone, false); !errors.Is(err, db.ErrGone) || failed != 1 {
		t.Fatalf("gone: statement %d, %v", failed, err)
	}
	if got := q("select count(*) from t_order_item where order_id = 4 and line_no = 1"); got != "1" {
		t.Errorf("rolled back: %s", got)
	}
}

// In the manual mode a save begins a transaction and leaves it open: others
// see nothing till a commit; a save that fails takes back itself alone. In
// a transaction the user began, an auto save goes in it too (§10.3).
func TestSaveManual(t *testing.T) {
	ctx := context.Background()
	w, q, dsn := saver(t)
	name := dbName(dsn)
	note := func(id, v string) db.Changes {
		return db.Changes{Updates: []db.Row{{Key: []string{id}, Cols: []db.Change{{Name: "note", Val: text(v), Old: db.Val{Null: true}}}}}}
	}
	end := func(sql string) {
		t.Helper()
		if err := w.Run(ctx, func(ctx context.Context, c db.Conn) error { _, err := db.End(ctx, c, sql); return err }); err != nil {
			t.Fatal(err)
		}
	}
	if _, tx, err := db.Save(ctx, w, my, name, "t_order", []string{"id"}, note("1", "first"), true); err != nil || tx != 'T' {
		t.Fatalf("first: %c %v", tx, err)
	}
	if got := q("select coalesce(note, '<null>') from t_order where id = 1"); got != "<null>" {
		t.Errorf("committed: %s", got)
	}
	bad := db.Changes{Updates: []db.Row{{Key: []string{"2"}, Cols: []db.Change{{Name: "amount", Val: text("abc"), Old: text("2.99")}}}}}
	if _, tx, err := db.Save(ctx, w, my, name, "t_order", []string{"id"}, bad, true); errno(err) != 1366 || tx != 'T' {
		t.Fatalf("bad: %c %v", tx, err)
	}
	end("commit")
	if got := q("select note from t_order where id = 1"); got != "first" {
		t.Errorf("the failed save took the first with it: %s", got)
	}
	end("begin")
	if _, tx, err := db.Save(ctx, w, my, name, "t_order", []string{"id"}, note("3", "mine"), false); err != nil || tx != 'T' {
		t.Fatalf("in the user's: %c %v", tx, err)
	}
	end("rollback")
	if got := q("select coalesce(note, '<null>') from t_order where id = 3"); got != "<null>" {
		t.Errorf("committed with the user's: %s", got)
	}
}
