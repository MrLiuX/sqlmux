//go:build integration

package redis

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/kackerx/sqlmux/internal/db"
)

func val(s string) db.Val { return db.Val{S: s} }

func upd(key string, cols ...db.Change) db.Row { return db.Row{Key: []string{key}, Cols: cols} }

func chg(name, old, now string) db.Change {
	return db.Change{Name: name, Old: val(old), Val: val(now)}
}

func add(pairs ...string) (row []db.Change) {
	for i := 0; i < len(pairs); i += 2 {
		row = append(row, db.Change{Name: pairs[i], Val: val(pairs[i+1])})
	}
	return row
}

// dump is the key as a table tab reads it, all of it.
func dump(t *testing.T, k Key) []string {
	t.Helper()
	cols, err := k.Columns(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	p, err := k.Page(context.Background(), db.PageQuery{Cols: cols, Limit: 1000})
	if err != nil {
		t.Fatal(err)
	}
	return rowsOf(p)
}

// A save writes each type as §8.7 says: a hash's value and name (a rename is a
// HDEL and a HSET), a list's value, its deletes by one LREM of a sentinel after
// the LSETs so that no place moves, a set's member, a zset's score and member, a
// stream's delete, a string's value with its TTL kept; each after a check that
// the cell is as it was loaded, the row there, the name free (F16.3).
func TestKeySave(t *testing.T) {
	ctx := context.Background()
	dsn := OwnDB(t)
	c := connectTo(t, dsn)
	run := func(cmd string) {
		t.Helper()
		if _, err := c.Exec(ctx, cmd, 0); err != nil {
			t.Fatal(cmd, err)
		}
	}
	save := func(k Key, ch db.Changes) (int, error) {
		t.Helper()
		failed, tx, err := k.Save(ctx, ch, false)
		if tx != 'I' && err == nil {
			t.Errorf("tx %c", tx)
		}
		return failed, err
	}
	// hash
	run("HSET h a 1 b 2 c 3 d 4")
	k := keyOf(t, dsn, "h", 50, 100)
	if failed, err := save(k, db.Changes{Updates: []db.Row{upd("a", chg("value", "1", "ONE")), upd("b", chg("field", "b", "bee"), chg("value", "2", "BEE"))}, Deletes: [][]string{{"c"}}, Inserts: [][]db.Change{add("field", "e", "value", "5")}}); err != nil || failed != -1 {
		t.Fatalf("hash: %d %v", failed, err)
	}
	if got := dump(t, k); !slices.Equal(got, []string{"a=ONE", "bee=BEE", "d=4", "e=5"}) {
		t.Errorf("hash after: %v", got)
	}
	// a cell that is no more what it was, a row that is gone, a name that is taken: nothing is written, the one at fault named
	for name, c := range map[string]struct {
		ch     db.Changes
		want   error
		failed int
	}{
		"stale":    {db.Changes{Updates: []db.Row{upd("d", chg("value", "4", "four")), upd("a", chg("value", "was", "x"))}}, db.ErrStale, 1},
		"gone":     {db.Changes{Updates: []db.Row{upd("zz", chg("value", "1", "x"))}}, db.ErrGone, 0},
		"deleted":  {db.Changes{Deletes: [][]string{{"d"}, {"zz"}}}, db.ErrGone, 1},
		"taken":    {db.Changes{Updates: []db.Row{upd("a", chg("field", "a", "d"))}}, ErrExists, 0},
		"insert":   {db.Changes{Inserts: [][]db.Change{add("field", "e", "value", "x")}}, ErrExists, 0},
		"no name":  {db.Changes{Inserts: [][]db.Change{add("field", " ", "value", "x")}}, nil, 0},
		"no value": {db.Changes{Inserts: [][]db.Change{add("field", "q")}}, nil, 0},
	} {
		failed, err := save(k, c.ch)
		if err == nil || c.want != nil && !errors.Is(err, c.want) || failed != c.failed {
			t.Errorf("%s: %d %v, want %d %v", name, failed, err, c.failed, c.want)
		}
	}
	if got := dump(t, k); !slices.Equal(got, []string{"a=ONE", "bee=BEE", "d=4", "e=5"}) {
		t.Errorf("hash after failures: %v", got)
	}
	// NULL and bytes
	if _, err := save(k, db.Changes{Updates: []db.Row{upd("a", db.Change{Name: "value", Old: val("ONE"), Val: db.Val{Null: true}})}}); !errors.Is(err, ErrNoNull) {
		t.Errorf("NULL: %v", err)
	}
	if _, err := save(k, db.Changes{Updates: []db.Row{upd("a", chg("value", "ONE", db.BinaryShown(3)))}}); !errors.Is(err, ErrBinary) {
		t.Errorf("bytes: %v", err)
	}
	// list: updates by place, several deletes at once without the places moving, an insert at the end
	run("RPUSH l a b c d e f")
	k = keyOf(t, dsn, "l", 50, 100)
	if failed, err := save(k, db.Changes{Deletes: [][]string{{"1"}, {"3"}}, Updates: []db.Row{upd("0", chg("value", "a", "A")), upd("4", chg("value", "e", "E"))}, Inserts: [][]db.Change{add("value", "g")}}); err != nil || failed != -1 {
		t.Fatalf("list: %d %v", failed, err)
	}
	if got := dump(t, k); !slices.Equal(got, []string{"0=A", "1=c", "2=E", "3=f", "4=g"}) {
		t.Errorf("list after: %v", got)
	}
	if _, err := save(k, db.Changes{Updates: []db.Row{upd("1", chg("value", "was", "x"))}}); !errors.Is(err, db.ErrStale) {
		t.Errorf("list stale: %v", err)
	}
	if _, err := save(k, db.Changes{Deletes: [][]string{{"9"}}}); !errors.Is(err, db.ErrGone) {
		t.Errorf("list gone: %v", err)
	}
	run("RPUSH l2 x x y x") // a value that is in the list more than once: only the places named go
	k2 := keyOf(t, dsn, "l2", 50, 100)
	if _, err := save(k2, db.Changes{Deletes: [][]string{{"2"}}}); err != nil || !slices.Equal(dump(t, k2), []string{"0=x", "1=x", "2=x"}) {
		t.Errorf("list one of equal values gone: %v %v", dump(t, k2), err)
	}
	// set
	run("SADD s a b c")
	k = keyOf(t, dsn, "s", 50, 100)
	if _, err := save(k, db.Changes{Updates: []db.Row{upd("a", chg("member", "a", "A"))}, Deletes: [][]string{{"b"}}, Inserts: [][]db.Change{add("member", "d")}}); err != nil || !slices.Equal(dump(t, k), []string{"A", "c", "d"}) {
		t.Errorf("set: %v %v", dump(t, k), err)
	}
	if _, err := save(k, db.Changes{Updates: []db.Row{upd("A", chg("member", "A", "c"))}}); !errors.Is(err, ErrExists) {
		t.Errorf("set taken: %v", err)
	}
	// zset
	run("ZADD z 1 a 2 b 3 c")
	k = keyOf(t, dsn, "z", 50, 100)
	if _, err := save(k, db.Changes{Updates: []db.Row{upd("a", chg("score", "1", "10")), upd("b", chg("member", "b", "bee")), upd("c", chg("member", "c", "see"), chg("score", "3", "30"))}, Inserts: [][]db.Change{add("member", "d", "score", "4")}}); err != nil {
		t.Fatal(err)
	}
	if got := dump(t, k); !slices.Equal(got, []string{"bee=2", "d=4", "a=10", "see=30"}) {
		t.Errorf("zset: %v", got)
	}
	if _, err := save(k, db.Changes{Updates: []db.Row{upd("a", chg("score", "1.0", "11"))}}); !errors.Is(err, db.ErrStale) {
		t.Errorf("zset stale: %v", err)
	}
	if _, err := save(k, db.Changes{Updates: []db.Row{upd("a", chg("score", "10.0", "11"))}}); err != nil { // the score as a number, not as text
		t.Errorf("zset score as a number: %v", err)
	}
	run("ZADD z 11.5 a")
	if _, err := save(k, db.Changes{Updates: []db.Row{upd("a", chg("score", "11", "12"))}}); !errors.Is(err, db.ErrStale) { // a fraction is a change too
		t.Errorf("zset stale by a fraction: %v", err)
	}
	run("ZADD z 11 a")
	if _, err := save(k, db.Changes{Deletes: [][]string{{"d"}}}); err != nil || !slices.Equal(dump(t, k), []string{"bee=2", "a=11", "see=30"}) {
		t.Errorf("zset delete: %v %v", dump(t, k), err)
	}
	// stream: delete alone
	run("XADD x 1-1 f v")
	run("XADD x 2-1 f w")
	k = keyOf(t, dsn, "x", 50, 100)
	if _, err := save(k, db.Changes{Deletes: [][]string{{"1-1"}}}); err != nil || len(dump(t, k)) != 1 {
		t.Errorf("stream delete: %v %v", dump(t, k), err)
	}
	if _, err := save(k, db.Changes{Updates: []db.Row{upd("2-1", chg("f", "w", "x"))}}); err == nil {
		t.Error("a stream entry changed")
	}
	if _, err := save(k, db.Changes{Deletes: [][]string{{"9-9"}}}); !errors.Is(err, db.ErrGone) {
		t.Errorf("stream gone: %v", err)
	}
	// string: the TTL stays
	run("SET str old EX 1000")
	k = keyOf(t, dsn, "str", 50, 100)
	if _, err := save(k, db.Changes{Updates: []db.Row{{Cols: []db.Change{chg("value", "old", "new")}}}}); err != nil {
		t.Fatal(err)
	}
	if r, _ := c.Query(ctx, "TTL", val("str")); r.Rows[0][0].S == "-1" {
		t.Error("a SET took the TTL")
	}
	if got := dump(t, k); !slices.Equal(got, []string{"new"}) {
		t.Errorf("string: %v", got)
	}
	if _, err := save(k, db.Changes{Updates: []db.Row{{Cols: []db.Change{chg("value", "old", "again")}}}}); !errors.Is(err, db.ErrStale) {
		t.Errorf("string stale: %v", err)
	}
	if _, err := save(k, db.Changes{Inserts: [][]db.Change{add("value", "x")}}); err == nil {
		t.Error("a second value of a string")
	}
	if _, err := save(k, db.Changes{Deletes: [][]string{nil}}); err != nil {
		t.Errorf("string delete: %v", err)
	}
	if r, _ := c.Query(ctx, "EXISTS", val("str")); r.Rows[0][0].S != "0" {
		t.Error("the string is there")
	}
	// the key gone, or another type than it was
	if _, err := save(k, db.Changes{Deletes: [][]string{nil}}); !errors.Is(err, ErrNoKey) {
		t.Errorf("a key that is gone: %v", err)
	}
	// read-only: nothing is sent
	ro := keyOf(t, dsn, "h", 50, 100)
	ro.ReadOnly = true
	if failed, tx, err := ro.Save(ctx, db.Changes{Deletes: [][]string{{"a"}}}, false); !errors.Is(err, db.ErrReadOnly) || tx != 0 || failed != -1 {
		t.Errorf("read-only: %d %c %v", failed, tx, err)
	}
	if got := dump(t, keyOf(t, dsn, "h", 50, 100)); len(got) != 4 {
		t.Errorf("read-only wrote: %v", got)
	}
}

// A key that another client changes after the save has read it is not written over:
// EXEC finds the WATCH broken and the save says the row changed (§8.7).
func TestKeySaveWatch(t *testing.T) {
	ctx := context.Background()
	dsn := OwnDB(t)
	c := connectTo(t, dsn)
	if _, err := c.Exec(ctx, "HSET h a 1 b 2", 0); err != nil {
		t.Fatal(err)
	}
	other := connectTo(t, dsn)
	k := keyOf(t, dsn, "h", 50, 100)
	onPlanned = func() { _, _ = other.Query(ctx, "HSET", val("h"), val("b"), val("changed")) }
	defer func() { onPlanned = nil }()
	failed, _, err := k.Save(ctx, db.Changes{Updates: []db.Row{upd("a", chg("value", "1", "x"))}}, false)
	if !errors.Is(err, db.ErrStale) || failed != -1 {
		t.Errorf("a key changed under the save: %d %v", failed, err)
	}
	if got := dump(t, k); !slices.Equal(got, []string{"a=1", "b=changed"}) {
		t.Errorf("nothing of the save: %v", got)
	}
}

// An EXEC that goes out and whose answer is lost is not a failure to be sure of:
// the connection is gone, the commit unknown (F13.1, §8.7).
func TestKeySaveCommitUnknown(t *testing.T) {
	ctx := context.Background()
	dsn := OwnDB(t)
	c := connectTo(t, dsn)
	if _, err := c.Exec(ctx, "HSET h a 1", 0); err != nil {
		t.Fatal(err)
	}
	k := keyOf(t, dsn, "h", 50, 100)
	killer := connectTo(t, dsn)
	onPlanned = func() { // the connection of Main goes before the EXEC does
		_, _ = killer.Query(ctx, "CLIENT", val("KILL"), val("USER"), val("default"), val("SKIPME"), val("yes"))
		time.Sleep(50 * time.Millisecond)
	}
	defer func() { onPlanned = nil }()
	_, _, err := k.Save(ctx, db.Changes{Updates: []db.Row{upd("a", chg("value", "1", "x"))}}, false)
	if err == nil {
		t.Fatalf("a save on a connection that is gone: %v", err)
	}
	if !errors.Is(err, db.ErrCommitUnknown) {
		t.Errorf("not said to be unknown: %v", err)
	}
}

// A check of the key that the server refuses, or a connection that goes, is the
// error of the save, as it is: not "the row is gone", which is what a nil says (F16.3).
func TestKeySaveReadFailsIsNotGone(t *testing.T) {
	ctx := context.Background()
	own := OwnDB(t)
	admin := connectTo(t, own)
	for _, cmd := range [][]string{
		{"HSET", "h", "a", "1"}, {"SADD", "s", "a"}, {"ZADD", "z", "1", "a"}, {"RPUSH", "l", "a"}, {"SET", "str", "a"}, {"XADD", "x", "1-1", "f", "v"},
	} {
		var vals []db.Val
		for _, a := range cmd[1:] {
			vals = append(vals, val(a))
		}
		if _, err := admin.Query(ctx, cmd[0], vals...); err != nil {
			t.Fatal(cmd, err)
		}
	}
	for _, args := range [][]string{{"SETUSER", "sqlmux_noread", "on", ">p", "~*", "+@all", "-@read", "+type"}} {
		var vals []db.Val
		for _, a := range args {
			vals = append(vals, val(a))
		}
		if _, err := admin.Query(ctx, "ACL", vals...); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() { _, _ = admin.Query(ctx, "ACL", val("DELUSER"), val("sqlmux_noread")) })
	dsn := strings.Replace(own, "redis://", "redis://sqlmux_noread:p@", 1)
	for _, c := range []struct {
		key string
		ch  db.Changes
	}{
		{"h", db.Changes{Deletes: [][]string{{"a"}}}}, {"h", db.Changes{Updates: []db.Row{upd("a", chg("value", "1", "2"))}}},
		{"h", db.Changes{Updates: []db.Row{upd("a", chg("field", "a", "b"))}}}, {"h", db.Changes{Inserts: [][]db.Change{add("field", "b", "value", "2")}}},
		{"s", db.Changes{Deletes: [][]string{{"a"}}}}, {"s", db.Changes{Updates: []db.Row{upd("a", chg("member", "a", "b"))}}}, {"s", db.Changes{Inserts: [][]db.Change{add("member", "b")}}},
		{"z", db.Changes{Deletes: [][]string{{"a"}}}}, {"z", db.Changes{Updates: []db.Row{upd("a", chg("score", "1", "2"))}}},
		{"z", db.Changes{Updates: []db.Row{upd("a", chg("member", "a", "b"))}}}, {"z", db.Changes{Inserts: [][]db.Change{add("member", "b", "score", "2")}}},
		{"l", db.Changes{Updates: []db.Row{upd("0", chg("value", "a", "b"))}}},
		{"str", db.Changes{Updates: []db.Row{upd("", chg("value", "a", "b"))}}}, {"str", db.Changes{Deletes: [][]string{{}}}},
		{"x", db.Changes{Deletes: [][]string{{"1-1"}}}},
	} {
		k := keyOf(t, dsn, c.key, 50, 100)
		_, _, err := k.Save(ctx, c.ch, false)
		if err == nil || errors.Is(err, db.ErrGone) || errors.Is(err, db.ErrStale) || errors.Is(err, ErrExists) || !strings.Contains(err.Error(), "NOPERM") || strings.Contains(err.Error(), "'type'") { // the check that is refused, not the TYPE before them
			t.Errorf("%s %+v: %v", c.key, c.ch, err)
		}
	}
}

// The command that fails when the batch runs is told by its change: a score that is
// no number is the added row's, after the updated one which went (F16.3).
func TestKeySaveNamesTheChangeThatFailed(t *testing.T) {
	ctx := context.Background()
	dsn := OwnDB(t)
	c := connectTo(t, dsn)
	if _, err := c.Query(ctx, "ZADD", val("z"), val("1"), val("a")); err != nil {
		t.Fatal(err)
	}
	k := keyOf(t, dsn, "z", 50, 100)
	failed, _, err := k.Save(ctx, db.Changes{Updates: []db.Row{upd("a", chg("score", "1", "2"))}, Inserts: [][]db.Change{add("member", "b", "score", "not a number")}}, false)
	if failed != 1 || err == nil || !strings.Contains(err.Error(), "float") {
		t.Errorf("failed %d: %v", failed, err)
	}
}
