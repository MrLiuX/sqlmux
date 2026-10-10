package app

import (
	"cmp"
	"slices"

	"github.com/kackerx/sqlmux/internal/config"
)

// groupOrder puts cs in the order the tree and the manager show them (§7.8,
// F10.6): those with no group first, then each group, in the order its first
// connection comes, the connections of each in the order cs has them, which
// is the file's. A connection added at the end of the file lands at the end
// of its group, or makes one.
func groupOrder(cs []*Conn) {
	rank := map[string]int{"": 0}
	for _, c := range cs {
		if _, ok := rank[c.Spec.Group]; !ok {
			rank[c.Spec.Group] = len(rank)
		}
	}
	slices.SortStableFunc(cs, func(x, y *Conn) int { return rank[x.Spec.Group] - rank[y.Spec.Group] })
}

// groupNames are the groups the session's connections have, in that order.
func (a *App) groupNames() (names []string) {
	for _, c := range a.sess.Conns {
		if g := c.Spec.Group; g != "" && !slices.Contains(names, g) {
			names = append(names, g)
		}
	}
	return names
}

// regroup puts every session's connections in the file's order and then in
// the tree's, as a group edited moves a connection to the place the file
// gives it (F10.6). A file that does not read leaves the order as it is.
func (a *App) regroup() {
	at := map[string]int{}
	if cs, _, err := config.LoadConnections(""); err == nil {
		for i, c := range cs {
			at[c.Name] = i
		}
	}
	pos := func(c *Conn) int {
		if i, ok := at[c.Spec.Name]; ok {
			return i
		}
		return len(at)
	}
	for _, s := range a.sessions {
		slices.SortStableFunc(s.Conns, func(x, y *Conn) int { return cmp.Compare(pos(x), pos(y)) })
		groupOrder(s.Conns)
	}
}
