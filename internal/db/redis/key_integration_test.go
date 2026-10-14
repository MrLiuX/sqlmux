//go:build integration

package redis

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/kackerx/sqlmux/internal/db"
)

// keyOf is the key name of the db dsn names, with the length a hash is read whole up
// to and the budget of an export as given.
func keyOf(t *testing.T, dsn, name string, loadAll, budget int) Key {
	t.Helper()
	return Key{Meta: worker(t, dsn), Main: worker(t, dsn), Name: name, LoadAll: loadAll, ScanKeys: budget}
}

// rowsOf is the first column of each row, joined by the others with =.
func rowsOf(p db.Paged) (out []string) {
	for _, r := range p.Rows {
		var cells []string
		for _, v := range r {
			if v.Null {
				cells = append(cells, "NULL")
			} else {
				cells = append(cells, v.S)
			}
		}
		out = append(out, strings.Join(cells, "="))
	}
	return out
}

func colNames(cols db.Columns) (out []string) {
	for _, c := range cols.Cols {
		out = append(out, c.Name)
	}
	return out
}

func attr(p db.Paged, name string) string {
	for _, a := range p.Info {
		if a.Name == name {
			return a.Value
		}
	}
	return ""
}

// What each type of the seed gives as columns, rows and the line under the title (F16.3, §8.7).
func TestKeyReads(t *testing.T) {
	ctx := context.Background()
	dsn := IntegrationDSN(t)
	page := func(k Key, q db.PageQuery) db.Paged {
		t.Helper()
		cols, err := k.Columns(ctx)
		if err != nil {
			t.Fatal(k.Name, err)
		}
		q.Cols = cols
		p, err := k.Page(ctx, q)
		if err != nil {
			t.Fatal(k.Name, err)
		}
		return p
	}
	// string
	k := keyOf(t, dsn, "str:plain", 50, 100)
	cols, _ := k.Columns(ctx)
	if !slices.Equal(colNames(cols), []string{"value"}) || cols.PK == nil || len(cols.PK) != 0 || cols.NoInsert == "" || len(cols.Sortable) != 0 || cols.Sortable == nil {
		t.Errorf("string columns: %+v", cols)
	}
	p := page(k, db.PageQuery{Limit: 100})
	if !slices.Equal(rowsOf(p), []string{"hello"}) || attr(p, "type") != "string" || attr(p, "length") != "5" || attr(p, "ttl") == "" || attr(p, "memory") == "" || attr(p, "encoding") == "" {
		t.Errorf("string: %v %+v", rowsOf(p), p.Info)
	}
	if got := page(keyOf(t, dsn, "str:ttl", 50, 100), db.PageQuery{Limit: 100}); !strings.HasSuffix(attr(got, "ttl"), "m") || !strings.Contains(attr(got, "ttl"), "d") {
		t.Errorf("a TTL of ten years: %q", attr(got, "ttl"))
	}
	if got := page(keyOf(t, dsn, "str:binary", 50, 100), db.PageQuery{Limit: 100}); !db.IsBinaryShown(got.Rows[0][0].S) {
		t.Errorf("bytes: %v", rowsOf(got))
	}
	// hash, read whole: sorted by field, filtered by a glob, paged
	k = keyOf(t, dsn, "hash:user", 50, 100)
	cols, _ = k.Columns(ctx)
	if !slices.Equal(colNames(cols), []string{"field", "value"}) || !slices.Equal(cols.PK, []string{"field"}) || cols.Sortable != nil {
		t.Errorf("hash columns: %+v", cols)
	}
	if got := rowsOf(page(k, db.PageQuery{Limit: 100})); !slices.Equal(got, []string{"age=30", "email=ann@example.com", `meta={"vip":true}`, "name=Ann"}) {
		t.Errorf("hash: %v", got)
	}
	if got := rowsOf(page(k, db.PageQuery{Limit: 100, Filter: "*e"})); !slices.Equal(got, []string{"age=30", "name=Ann"}) {
		t.Errorf("hash filtered: %v", got)
	}
	if got := rowsOf(page(k, db.PageQuery{Limit: 100, Order: []db.OrderCol{{Col: "value", Desc: true}}})); got[0] != `meta={"vip":true}` {
		t.Errorf("hash by value desc: %v", got)
	}
	if got := page(k, db.PageQuery{Limit: 2, Offset: 1}); !slices.Equal(rowsOf(got), []string{"email=ann@example.com", `meta={"vip":true}`}) || !got.Next || got.Cursor != "" {
		t.Errorf("hash page: %v next %v", rowsOf(got), got.Next)
	}
	if n, err := k.Count(ctx, ""); err != nil || n != 4 {
		t.Errorf("hash count %d %v", n, err)
	}
	if n, err := k.Count(ctx, "*e"); err != nil || n != 2 {
		t.Errorf("hash count of a filter %d %v", n, err)
	}
	// a hash too long to read whole: by HSCAN, a next page by its cursor, no sorting, a filter that is MATCH
	k = keyOf(t, dsn, "hash:big", 50, 100)
	cols, _ = k.Columns(ctx)
	if cols.Sortable == nil || len(cols.Sortable) != 0 {
		t.Errorf("a long hash sorts: %+v", cols.Sortable)
	}
	seen, cursor, pages := map[string]bool{}, "", 0
	for {
		got := page(k, db.PageQuery{Limit: 500, Cursor: cursor})
		for _, r := range got.Rows {
			seen[r[0].S] = true
		}
		if pages++; !got.Next {
			break
		}
		if got.Cursor == "" {
			t.Fatal("a next page with no cursor")
		}
		cursor = got.Cursor
	}
	if len(seen) != 6000 || pages < 3 {
		t.Errorf("HSCAN gave %d fields in %d pages", len(seen), pages)
	}
	if got := page(k, db.PageQuery{Limit: 500, Filter: "field59*"}); len(got.Rows) == 0 || !strings.HasPrefix(got.Rows[0][0].S, "field59") {
		t.Errorf("MATCH: %v", rowsOf(got))
	}
	if n, err := k.Count(ctx, ""); err != nil || n != 6000 {
		t.Errorf("long hash count %d %v", n, err)
	}
	if n, err := k.Count(ctx, "field1*"); err != nil || n != -1 {
		t.Errorf("a count that is not known: %d %v", n, err)
	}
	// list by offset
	k = keyOf(t, dsn, "list:queue", 50, 100)
	cols, _ = k.Columns(ctx)
	if !cols.Cols[0].Generated || cols.Cols[0].ReadOnly == "" || len(cols.Sortable) != 0 || cols.Sortable == nil {
		t.Errorf("list columns: %+v", cols)
	}
	if got := page(k, db.PageQuery{Limit: 2}); !slices.Equal(rowsOf(got), []string{"0=first", "1=second"}) || !got.Next {
		t.Errorf("list: %v %v", rowsOf(got), got.Next)
	}
	if got := page(k, db.PageQuery{Limit: 2, Offset: 4}); !slices.Equal(rowsOf(got), []string{"4=fifth"}) || got.Next {
		t.Errorf("list end: %v %v", rowsOf(got), got.Next)
	}
	// set
	k = keyOf(t, dsn, "set:tags", 50, 100)
	if got := rowsOf(page(k, db.PageQuery{Limit: 100})); !slices.Equal(got, []string{"blue", "green", "red"}) {
		t.Errorf("set: %v", got)
	}
	if got := rowsOf(page(k, db.PageQuery{Limit: 100, Filter: "?e*"})); !slices.Equal(got, []string{"red"}) {
		t.Errorf("set filtered: %v", got)
	}
	// zset: whole, by score; by offset beyond LoadAll, in the score's order or the reverse
	k = keyOf(t, dsn, "zset:rank", 50, 100)
	if got := rowsOf(page(k, db.PageQuery{Limit: 100})); !slices.Equal(got, []string{"alice=1", "bob=2.5", "carol=3", "dave=3", "erin=10"}) {
		t.Errorf("zset: %v", got)
	}
	if got := rowsOf(page(k, db.PageQuery{Limit: 100, Order: []db.OrderCol{{Col: "score", Desc: true}}})); got[0] != "erin=10" {
		t.Errorf("zset by score desc: %v", got)
	}
	k = keyOf(t, dsn, "zset:rank", 2, 100)
	cols, _ = k.Columns(ctx)
	if !slices.Equal(cols.Sortable, []string{"score"}) {
		t.Errorf("a long zset sorts by %v", cols.Sortable)
	}
	if got := page(k, db.PageQuery{Limit: 2, Offset: 1}); !slices.Equal(rowsOf(got), []string{"bob=2.5", "carol=3"}) || !got.Next {
		t.Errorf("zset by offset: %v %v", rowsOf(got), got.Next)
	}
	if got := page(k, db.PageQuery{Limit: 2, Order: []db.OrderCol{{Col: "score", Desc: true}}}); !slices.Equal(rowsOf(got), []string{"erin=10", "dave=3"}) {
		t.Errorf("zset reversed: %v", rowsOf(got))
	}
	// stream: by ID, the fields of the page as columns, a missing one NULL
	k = keyOf(t, dsn, "stream:events", 50, 100)
	cols, _ = k.Columns(ctx)
	if cols.NoInsert == "" || cols.NoUpdate == "" || len(cols.Sortable) != 0 {
		t.Errorf("stream columns: %+v", cols)
	}
	got := page(k, db.PageQuery{Limit: 2})
	if !slices.Equal(got.Cols[0:1], []db.Col{{Name: "id"}}) || len(got.Cols) != 4 || !got.Next || got.Cursor != "1700000000000-2" ||
		!slices.Equal(rowsOf(got), []string{"1700000000000-1=NULL=login=ann", "1700000000000-2=button=click=ann"}) {
		t.Errorf("stream: %v %v next %v cursor %q", got.Cols, rowsOf(got), got.Next, got.Cursor)
	}
	if got := page(k, db.PageQuery{Limit: 2, Cursor: "1700000000000-2"}); !slices.Equal(rowsOf(got), []string{"1700000001000-0"}) && !strings.HasPrefix(rowsOf(got)[0], "1700000000001-1=") || got.Next {
		t.Errorf("stream, the next page: %v %v", rowsOf(got), got.Next)
	}
	// a key that is not there
	if _, err := keyOf(t, dsn, "no:such:key", 50, 100).Columns(ctx); !errors.Is(err, ErrNoKey) {
		t.Errorf("a key that is gone: %v", err)
	}
}

// An export gives all of what a key reads whole, a page when asked for it, the rest
// page after page up to the budget, which it says it stopped at (F16.3).
func TestKeyEach(t *testing.T) {
	ctx := context.Background()
	dsn := IntegrationDSN(t)
	each := func(k Key, q db.PageQuery) (n int, head []db.Col, err error) {
		err = k.Each(ctx, q, func(c []db.Col) error { head = c; return nil }, func([]db.Val) error { n++; return nil })
		return n, head, err
	}
	if n, head, err := each(keyOf(t, dsn, "hash:user", 50, 100), db.PageQuery{Filter: "*e"}); err != nil || n != 2 || len(head) != 2 {
		t.Errorf("a hash read whole: %d %v %v", n, head, err)
	}
	if n, _, err := each(keyOf(t, dsn, "hash:user", 50, 100), db.PageQuery{Limit: 2, Offset: 1}); err != nil || n != 2 {
		t.Errorf("a page: %d %v", n, err)
	}
	if n, _, err := each(keyOf(t, dsn, "hash:big", 50, 100000), db.PageQuery{}); err != nil || n != 6000 {
		t.Errorf("a long hash: %d %v", n, err)
	}
	if n, _, err := each(keyOf(t, dsn, "hash:big", 50, 1200), db.PageQuery{}); !errors.Is(err, db.ErrTruncated) || n < 1200 || n >= 6000 {
		t.Errorf("past the budget: %d %v", n, err)
	}
	if n, _, err := each(keyOf(t, dsn, "list:queue", 50, 100), db.PageQuery{}); err != nil || n != 5 {
		t.Errorf("a list: %d %v", n, err)
	}
	if n, _, err := each(keyOf(t, dsn, "stream:events", 50, 100), db.PageQuery{}); err != nil || n != 3 {
		t.Errorf("a stream: %d %v", n, err)
	}
	if n, _, err := each(keyOf(t, dsn, "zset:rank", 2, 100), db.PageQuery{}); err != nil || n != 5 {
		t.Errorf("a zset by offset: %d %v", n, err)
	}
}

// The commands that make a key again make it as it is, in a db of the test's own, for
// each type but JSON (F16.3).
func TestKeyRebuild(t *testing.T) {
	ctx := context.Background()
	seed := IntegrationDSN(t)
	own := OwnDB(t)
	c := connectTo(t, own)
	for _, name := range []string{"str:plain", "str:json", "str:unicode", "str:binary", "hash:user", "hash:binary", "list:queue", "list:dups", "set:tags", "zset:rank", "stream:events"} {
		k := keyOf(t, seed, name, 50, 100)
		text, err := k.Rebuild(ctx)
		if err != nil || text == "" {
			t.Fatalf("%s: %q %v", name, text, err)
		}
		if _, err := c.Exec(ctx, text, 0); err != nil {
			t.Fatalf("%s: %v\n%s", name, err, text)
		}
		if strings.HasPrefix(name, "str:") { // bytes too: the rows say only how many
			a, errA := c.Query(ctx, "GET", db.Val{S: name})
			b, errB := connectTo(t, seed).Query(ctx, "GET", db.Val{S: name})
			if errA != nil || errB != nil || a.Rows[0][0].S != b.Rows[0][0].S {
				t.Errorf("%s: not as it was\n%s", name, text)
			}
		}
		// as it reads: the same rows from either
		pa, _ := keyOf(t, seed, name, 50, 100).Page(ctx, db.PageQuery{Limit: 100})
		pb, _ := keyOf(t, own, name, 50, 100).Page(ctx, db.PageQuery{Limit: 100})
		if !slices.Equal(rowsOf(pa), rowsOf(pb)) {
			t.Errorf("%s: %v, rebuilt %v", name, rowsOf(pa), rowsOf(pb))
		}
	}
	long, err := keyOf(t, seed, "hash:big", 5, 100).Rebuild(ctx)
	if err != nil || !strings.HasPrefix(long, "# ") || strings.Count(long, "field") > 12 {
		t.Errorf("a long hash is cut and says so: %q %v", long[:min(len(long), 120)], err)
	}
}
