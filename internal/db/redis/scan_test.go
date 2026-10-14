package redis

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/kackerx/sqlmux/internal/db"
)

// A scan of a key reads batches till it has the entries it is asked for, and no
// further: the cursor it stops at says there is more; one asked for no limit goes to
// the end; a batch that fails is the error, the entries before it kept.
func TestScanLoop(t *testing.T) {
	calls := 0
	step := func(cursor uint64, _ string, _ int64) ([]string, uint64, error) {
		calls++
		next := cursor + 1
		if next == 5 {
			next = 0
		}
		return []string{"a", "1", "b", "2", "c", "3"}, next, nil // three entries of two
	}
	got, cursor, skip, err := scanLoop(0, 0, "", 4, 2, step)
	if err != nil || len(got) != 4 || calls != 2 || cursor != 1 || skip != 1 { // the second batch is cut after its first entry
		t.Errorf("limit 4: %d entries, %d calls, cursor %d skip %d, %v", len(got), calls, cursor, skip, err)
	}
	if got, cursor, skip, _ = scanLoop(1, 1, "", 4, 2, step); len(got) != 4 || cursor != 2 || skip != 2 { // from there: 2 left of it, 2 of the batch after
		t.Errorf("from the middle: %d entries, cursor %d skip %d", len(got), cursor, skip)
	}
	calls = 0
	if got, cursor, _, _ = scanLoop(0, 0, "", 0, 2, step); len(got) != 15 || calls != 5 || cursor != 0 {
		t.Errorf("no limit: %d entries, %d calls, cursor %d", len(got), calls, cursor)
	}
	if got, _, _, _ = scanLoop(0, 0, "", 4, 2, step); !slices.Equal(got[0], []string{"a", "1"}) {
		t.Errorf("an entry is its width: %v", got[0])
	}
	boom := errors.New("connection reset")
	n := 0
	got, _, _, err = scanLoop(0, 0, "", 0, 2, func(uint64, string, int64) ([]string, uint64, error) {
		if n++; n == 2 {
			return nil, 0, boom
		}
		return []string{"a", "1"}, 7, nil
	})
	if !errors.Is(err, boom) || len(got) != 1 {
		t.Errorf("a failing batch: %v %v", got, err)
	}
}

// A key's pages are LIMIT rows each when a batch is more than that, a listpack
// being read whole by one HSCAN: the cursor of a page in the middle of a batch
// says how far in, and the pages together are every entry once.
func TestScanPages(t *testing.T) {
	batches := [][]string{{"a", "1", "b", "2", "c", "3"}, {"d", "4", "e", "5"}, {"f", "6"}}
	step := func(cursor uint64, _ string, _ int64) ([]string, uint64, error) {
		next := cursor + 1
		if int(next) == len(batches) {
			next = 0
		}
		return batches[cursor], next, nil
	}
	for _, limit := range []int{1, 2, 4, 6, 10} {
		var names []string
		q := db.PageQuery{Limit: limit}
		for pages := 0; ; pages++ {
			p, err := Key{}.scan(db.Paged{}, q, step, 2)
			if err != nil || len(p.Rows) > limit || pages > 7 {
				t.Fatalf("limit %d page %d: %d rows, %v", limit, pages, len(p.Rows), err)
			}
			for _, r := range p.Rows {
				names = append(names, r[0].S)
			}
			if !p.Next {
				break
			}
			q.Cursor = p.Cursor
		}
		if strings.Join(names, "") != "abcdef" {
			t.Errorf("limit %d: %v", limit, names)
		}
	}
	if _, err := (Key{}).scan(db.Paged{}, db.PageQuery{Cursor: "1:x"}, step, 2); err == nil {
		t.Error("a cursor that is not one")
	}
}
