package config

import (
	"encoding/json"
	"slices"
)

// base is the state as this program last had it from the file or wrote it
// to it, what a write is told apart from the file's by: another program's
// changes in the file since are not undone by this one's, and what this one
// dropped does not come back from the file (F13.3); basePath the file it is of,
// another one being none. Guarded by saveMu.
var (
	base     = &State{}
	basePath string
)

// merge is what the program's state ours says, over base, which it began
// from, on the file as it is now, disk, which others may have changed: each
// entry this program left alone is the file's; one it changed or added is its
// own, the lists of an entry both changed put together; one it dropped is gone.
func merge(base, ours, disk *State) *State {
	out := &State{
		Tables:  mergeMap(base.Tables, ours.Tables, disk.Tables, mergeTable),
		SQL:     mergeMap(base.SQL, ours.SQL, disk.SQL, func(b, o, d []string) []string { return union(b, o, d, ident) }),
		Queries: mergeMap(base.Queries, ours.Queries, disk.Queries, func(_, o, _ QueryState) QueryState { return o }),
	}
	if same(base.Recent, ours.Recent) { // not touched here
		out.Recent = disk.Recent
	} else {
		out.Recent = union(base.Recent, ours.Recent, disk.Recent, ident)
	}
	return out
}

// mergeMap is merge of one map by key; both says what becomes of an entry
// both programs changed.
func mergeMap[V any](base, ours, disk map[string]V, both func(base, ours, disk V) V) map[string]V {
	out := map[string]V{}
	changed := func(a, b map[string]V, k string) bool {
		av, aok := a[k]
		bv, bok := b[k]
		return aok != bok || aok && !same(av, bv)
	}
	keys := map[string]bool{}
	for _, m := range []map[string]V{base, ours, disk} {
		for k := range m {
			keys[k] = true
		}
	}
	for k := range keys {
		b := base[k]
		o, ook := ours[k]
		d, dok := disk[k]
		switch {
		case !changed(ours, base, k): // left alone: the file's, or gone from it
			if dok {
				out[k] = d
			}
		case !ook: // dropped here
		case dok && changed(disk, base, k):
			out[k] = both(b, o, d)
		default:
			out[k] = o
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func mergeTable(b, o, d *TableState) *TableState {
	for _, t := range []**TableState{&b, &o, &d} { // a null entry reads as none
		if *t == nil {
			*t = &TableState{}
		}
	}
	history := union(b.History, o.History, d.History, queryKey)
	slices.SortStableFunc(history, func(x, y Query) int { return y.At.Compare(x.At) }) // newest first, whose program it was
	return &TableState{History: history, Favorites: union(b.Favorites, o.Favorites, d.Favorites, queryKey)}
}

// queryKey is what makes two WHEREs one: all but the time.
func queryKey(q Query) string {
	b, _ := json.Marshal(struct {
		Where string
		Order any
		Desc  bool
		Limit int
	}{q.Where, q.Order, q.Desc, q.Limit})
	return string(b)
}

func ident[T any](v T) T { return v }

// union is ours, then what is in disk that ours has not, each once, by key;
// what base had and ours has not was dropped here and stays so.
// ponytail: lists with no time (quick SQL, the palette's picks) keep the
// other program's newest after ours, where a cut at their limit takes them
// first; sort them by a time if that shows.
func union[T any, K comparable](base, ours, disk []T, key func(T) K) []T {
	seen := map[K]bool{}
	var out []T
	for _, v := range ours {
		if k := key(v); !seen[k] {
			seen[k] = true
			out = append(out, v)
		}
	}
	gone := map[K]bool{}
	for _, v := range base {
		if k := key(v); !seen[k] {
			gone[k] = true
		}
	}
	for _, v := range disk {
		if k := key(v); !seen[k] && !gone[k] {
			seen[k] = true
			out = append(out, v)
		}
	}
	return out
}

func same(a, b any) bool {
	x, _ := json.Marshal(a)
	y, _ := json.Marshal(b)
	return slices.Equal(x, y)
}
