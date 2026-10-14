package redis

import (
	"errors"
	"slices"
	"testing"
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
	got, cursor, err := scanLoop(0, "", 4, 2, step)
	if err != nil || len(got) != 6 || calls != 2 || cursor != 2 {
		t.Errorf("limit 4: %d entries, %d calls, cursor %d, %v", len(got), calls, cursor, err)
	}
	calls = 0
	if got, cursor, _ = scanLoop(0, "", 0, 2, step); len(got) != 15 || calls != 5 || cursor != 0 {
		t.Errorf("no limit: %d entries, %d calls, cursor %d", len(got), calls, cursor)
	}
	if got, _, _ = scanLoop(0, "", 4, 2, step); !slices.Equal(got[0], []string{"a", "1"}) {
		t.Errorf("an entry is its width: %v", got[0])
	}
	boom := errors.New("connection reset")
	n := 0
	got, _, err = scanLoop(0, "", 0, 2, func(uint64, string, int64) ([]string, uint64, error) {
		if n++; n == 2 {
			return nil, 0, boom
		}
		return []string{"a", "1"}, 7, nil
	})
	if !errors.Is(err, boom) || len(got) != 1 {
		t.Errorf("a failing batch: %v %v", got, err)
	}
}
