package config

import (
	"encoding/json"
	"slices"
	"time"
)

// base is the state as this program last had it from the file, or last wrote
// to it as it was then (what it had, not what the merge made): what a write is
// told apart from the file's by, so another program's changes in the file
// since are not undone by this one's, nor this one's drops undone by the
// file's (F13.3). Guarded by saveMu.
var base = &State{}

// merge is what the program's state ours says, over base, which it began
// from, on the file as it is now, disk, which others may have changed: each
// entry this program left alone is the file's; one it changed or added is its
// own, the lists of an entry both changed put together; one either dropped is
// gone.
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
	return out
}

func mergeTable(b, o, d *TableState) *TableState {
	for _, t := range []**TableState{&b, &o, &d} { // a null entry reads as none
		if *t == nil {
			*t = &TableState{}
		}
	}
	at := map[string]time.Time{} // a WHERE both ran: the later time
	for _, q := range append(slices.Clone(o.History), d.History...) {
		at[q.Key()] = laterOf(at[q.Key()], q.At)
	}
	history := union(b.History, o.History, d.History, Query.Key)
	for i := range history {
		history[i].At = at[history[i].Key()]
	}
	slices.SortStableFunc(history, func(x, y Query) int { return y.At.Compare(x.At) }) // newest first, whose program it was
	return &TableState{History: history[:min(len(history), WhereRows)], Favorites: union(b.Favorites, o.Favorites, d.Favorites, Query.Key)}
}

func laterOf(a, b time.Time) time.Time {
	if b.After(a) {
		return b
	}
	return a
}

func ident[T any](v T) T { return v }

// union is ours, then what is in disk that ours has not, each once, by key: an
// item is kept if base did not have it, or both ours and disk do; one base had
// that either dropped stays dropped.
// ponytail: lists with no time (quick SQL, the palette's picks) keep the
// other program's newest after ours, where a cut at their limit takes them
// first; sort them by a time if that shows.
func union[T any, K comparable](base, ours, disk []T, key func(T) K) []T {
	keys := func(list []T) map[K]bool {
		set := map[K]bool{}
		for _, v := range list {
			set[key(v)] = true
		}
		return set
	}
	inBase, inDisk := keys(base), keys(disk)
	seen := map[K]bool{}
	var out []T
	for _, v := range ours {
		if k := key(v); !seen[k] && (!inBase[k] || inDisk[k]) { // the file dropped it: it stays dropped
			seen[k] = true
			out = append(out, v)
		}
	}
	for _, v := range disk {
		if k := key(v); !seen[k] && !inBase[k] {
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
