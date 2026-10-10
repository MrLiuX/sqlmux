package keymap

import (
	"fmt"
	"strconv"
	"strings"
)

// Mode is the vim mode (B-02). COMMAND resolves like INSERT: the command line
// is an input.
type Mode uint8

const (
	Normal Mode = iota
	Visual
	Insert
	Command
)

func (m Mode) String() string { return [...]string{"normal", "visual", "insert", "command"}[m] }

// typing reports whether keys go into an input rather than to NORMAL/VISUAL bindings.
func (m Mode) typing() bool { return m == Insert || m == Command }

// Context is where a key press lands; it picks and orders the scopes (§6.4).
type Context struct {
	Overlay string   // topmost overlay scope, if one is open
	Under   string   // an overlay under it, whose keys come next: options or segments under newrow (F3.33), the palette under quick SQL's candidates (F4.1)
	Focus   []string // focused widget scopes, most specific first: {"result", "grid"}
	Pane    string   // pane type for [map.<pane>.<mode>]: console | grid | tree
	Mode    Mode
	// Firm is a scope whose complete bindings stay complete: a longer
	// sequence of a lower scope that starts with one is not bound (select: d
	// does not wait for dd, F10.7). The others keep the first to bind an
	// exact sequence winning and a longer one hanging under it.
	Firm string
	// Plain is the modeless editor (§6.9): typed keys are text, so of the
	// pane's and NORMAL's bindings only the Ctrl ones apply, as C-h and C-e.
	Plain bool
}

// scopes lists the tables for c, highest priority first: overlay → user maps
// (NORMAL/VISUAL only) → focused widget → normal (NORMAL only) → global.
// A Ctrl leader works in every mode, so leader bindings from normal come last
// when normal itself is not in play.
func (m *Map) scopes(c Context, maps bool) []string {
	var ts []string
	for _, o := range []string{c.Overlay, c.Under} {
		if o != "" {
			ts = append(ts, "keys."+o)
		}
	}
	if maps && !c.Mode.typing() {
		if c.Pane != "" {
			ts = append(ts, "map."+c.Pane+"."+c.Mode.String())
		}
		ts = append(ts, "map."+c.Mode.String())
	}
	for _, f := range c.Focus {
		// grid/tree/console/result keys are NORMAL/VISUAL keys: in INSERT only
		// the input being typed into has bindings (↵ in console inserts a newline).
		if !c.Mode.typing() || f == "cell" || f == "input" || c.Plain {
			ts = append(ts, "keys."+f)
		}
	}
	if c.Mode == Normal || c.Plain {
		ts = append(ts, "keys.normal")
	}
	ts = append(ts, "keys.global")
	if c.Mode != Normal && strings.HasPrefix(string(m.Leader[0]), "<C-") {
		ts = append(ts, "leader")
	}
	return ts
}

type node struct {
	next   map[Key]*node
	order  []Key // children in binding order, for which-key
	bound  bool
	action string
	rhs    []Key
	table  string // the scope that put it there first, the highest (§6.5)
	firm   bool   // bound by the Firm scope of the context (F10.7)
}

// trie merges the scopes of c into one trie, cached per context. The first
// scope to bind an exact sequence wins; a longer sequence from a lower scope
// still hangs under it, which is what makes a node ambiguous. A node's
// table is the scope that made it, so a table makes all its nodes before
// the next one does: children come grouped by table in scope order, which
// Next relies on (§6.5).
// ponytail: a node one table binds and a higher one makes a prefix of
// (xx in [map.grid.normal], x in [keys.grid]) is listed in the higher
// table's group; keeping the binding's table instead needs Next to sort.
func (m *Map) trie(c Context, maps bool) *node {
	id := fmt.Sprint(c, maps)
	if t, ok := m.tries[id]; ok {
		return t
	}
	root := &node{}
	for _, t := range m.scopes(c, maps) {
		bs := m.tables[t]
		if c.Plain && t != "keys."+c.Overlay && t != "keys."+c.Under && t != "keys.global" && t != "leader" && t != "keys.input" {
			bs = ctrlOnly(m, bs) // not the keys that are text
		}
		if t == "leader" {
			bs, t = nil, "keys.normal"
			for _, b := range m.tables[t] {
				if b.Keys[0] == Leader {
					bs = append(bs, b)
				}
			}
		}
		for _, b := range bs {
			n, longer := root, false
			for _, k := range m.expand(b.Keys) {
				if n.firm { // a complete binding of the Firm scope: nothing hangs under it
					longer = true
					break
				}
				if n.next == nil {
					n.next = map[Key]*node{}
				}
				if n.next[k] == nil {
					n.next[k] = &node{table: t}
					n.order = append(n.order, k)
				}
				n = n.next[k]
			}
			if !longer && !n.bound {
				n.bound, n.action, n.rhs, n.firm = true, b.Action, b.RHS, c.Firm != "" && t == "keys."+c.Firm
			}
		}
	}
	m.tries[id] = root
	return root
}

// Result is one thing a key press produced.
type Result struct {
	Action string // bound action, with args
	Count  int    // count typed before it; 0 when none
	Keys   []Key  // keys no binding claimed, for the focused widget (count digits included)
	Map    []Key  // a user mapping's right-hand side, a count typed before it first: feed each as typed, maps off
	// Again is the key that ended an ambiguous pending sequence: the shorter
	// binding ran, and this key is fed afresh once what ran before it has
	// had its effect, in the context it then meets (F8.15). Last in out.
	Again Key
}

// Resolver turns key presses into actions: counts, sequences, timeouts and
// user mappings (§6.5, §6.6).
type Resolver struct {
	m     *Map
	count string
	keys  []Key // pending sequence, after the count
	node  *node
	seq   int
}

func NewResolver(m *Map) *Resolver { return &Resolver{m: m} }

// Feed consumes one key press (a Result.Again in out is for the caller to
// feed once the results before it ran); maps is false for the keys of a mapping's
// right-hand side, which are not remapped (noremap). wait reports an
// ambiguous pending sequence: the caller should call Timeout(seq) after
// Map.Timeout unless another key comes first.
func (r *Resolver) Feed(c Context, k Key, maps bool) (out []Result, wait bool) {
	r.seq++
	r.feed(c, k, r.m.trie(c, maps), &out)
	return out, r.node != nil && r.node.bound
}

// Timeout fires the pending ambiguous binding, if seq is still current.
func (r *Resolver) Timeout(seq int) []Result {
	var out []Result
	if seq == r.seq && r.node != nil && r.node.bound {
		r.fire(&out)
	}
	return out
}

func (r *Resolver) Seq() int { return r.seq }

// Next is one way to continue the pending sequence.
type Next struct {
	Key    Key
	Action string // "" for a mapping or a further prefix
	RHS    []Key  // set for a mapping
	Table  string // the config table it comes from, "keys.grid" (§6.5)
}

// Next lists what can follow the pending keys (§6.5 which-key). It is
// empty when nothing is pending.
func (r *Resolver) Next() []Next { return r.node.children() }

// Next lists what can follow prefix in context c, user maps and all: the
// ? help's level (§6.5). It is empty past what c binds.
func (m *Map) Next(c Context, prefix []Key) []Next {
	n := m.trie(c, true)
	for _, k := range prefix {
		if n = n.next[k]; n == nil {
			return nil
		}
	}
	return n.children()
}

// children are n's in binding order, which is by table in the order of
// scopes: a table creates all its nodes before the next one's.
func (n *node) children() []Next {
	if n == nil {
		return nil
	}
	out := make([]Next, 0, len(n.order))
	for _, k := range n.order {
		c := n.next[k]
		out = append(out, Next{Key: k, Action: c.action, RHS: c.rhs, Table: c.table})
	}
	return out
}

// Pending is the count and keys typed so far, for the status bar (B-03).
func (r *Resolver) Pending() []Key { return append(digits(r.count), r.keys...) }

func (r *Resolver) Reset() { r.count, r.keys, r.node = "", nil, nil }

func (r *Resolver) feed(c Context, k Key, root *node, out *[]Result) {
	if r.node == nil {
		if !c.Mode.typing() && len(k) == 1 && (k >= "1" && k <= "9" || k == "0" && r.count != "") {
			r.count += string(k) // a lone 0 is a key (grid.first), not a count
			return
		}
		if k == Esc && r.count != "" && !c.Mode.typing() { // it takes the count back, as in vim, before a binding of its own
			r.Reset()
			return
		}
		r.node = root
	}
	next := r.node.next[k]
	switch {
	case next != nil:
		r.keys = append(r.keys, k)
		r.node = next
		if len(next.next) == 0 {
			r.fire(out)
		}
	case r.node != root && r.node.bound:
		// The shorter binding of an ambiguous node fires; k starts afresh.
		r.fire(out)
		*out = append(*out, Result{Again: k})
	case k == Esc && (r.node != root || r.count != ""):
		r.Reset() // esc cancels a pending sequence or count
	default:
		*out = append(*out, Result{Keys: append(r.Pending(), k)})
		r.Reset()
	}
}

func (r *Resolver) fire(out *[]Result) {
	n, count := r.node, r.count
	r.Reset()
	if n.rhs == nil {
		cnt, _ := strconv.Atoi(count)
		*out = append(*out, Result{Action: n.action, Count: cnt})
		return
	}
	// The right-hand side goes back to be fed key by key, each in the
	// context it then meets (i makes the next keys INSERT's), as vim's
	// typeahead; a count typed before the mapping goes in front.
	*out = append(*out, Result{Map: append(digits(count), r.m.expand(n.rhs)...)})
}

func digits(s string) []Key {
	var ks []Key
	for _, r := range s {
		ks = append(ks, Key(string(r)))
	}
	return ks
}

// ctrlOnly is the bindings of bs that start with a Ctrl key but C-h, which
// the modeless editor takes as Backspace: some terminals send that key as
// ^H (§6.2, §6.9).
func ctrlOnly(m *Map, bs []Binding) []Binding {
	var out []Binding
	for _, b := range bs {
		if ks := m.expand(b.Keys); len(ks) > 0 && strings.HasPrefix(string(ks[0]), "<C-") && ks[0] != "<C-h>" {
			out = append(out, b)
		}
	}
	return out
}
