package editor

import (
	"slices"
	"strings"
)

// Snippets (§11「snippet 与 Select 模式」, F15.8), the least of nvim's
// vim.snippet: a text with tabstops, the first selected in SELECT, what is
// typed taking its place; Tab goes to the next, S-Tab to the one before; the
// last Tab ends the snippet, and so do the cursor leaving its text and the
// text being replaced whole. There are no $0, no nesting, no choices.

// Range is the text from From to To, To not in.
type Range struct{ From, To Pos }

// snippet keeps its places as offsets into the text with its lines joined by
// a newline, moved by what changes the text (snipSync).
type snippet struct {
	text     []string // the text the offsets are of
	from, to int      // the snippet's own text
	stops    [][2]int // each tabstop's text; an empty one has both alike
	cur      int
}

// offset is where p is in the text, its lines joined by a newline.
func (e *Editor) offset(p Pos) int {
	n := p.Col
	for _, l := range e.lines[:p.Line] {
		n += len(l) + 1
	}
	return n
}

// posAt is offset's other way.
func (e *Editor) posAt(off int) Pos {
	for i, l := range e.lines {
		if off <= len(l) {
			return Pos{i, off}
		}
		off -= len(l) + 1
	}
	return Pos{len(e.lines) - 1, len(e.lines[len(e.lines)-1])}
}

// Snippet starts a snippet over the text from from to to, tabstops in the
// order they are visited, and goes to the first. With no tabstop there is no
// snippet.
func (e *Editor) Snippet(from, to Pos, stops []Range) {
	if len(stops) == 0 || e.ReadOnly {
		return
	}
	s := &snippet{text: slices.Clone(e.lines), from: e.offset(from), to: e.offset(to)}
	for _, r := range stops {
		s.stops = append(s.stops, [2]int{e.offset(r.From), e.offset(r.To)})
	}
	e.snip = s
	e.goTo(0)
	e.scrollToCursor()
}

// SnippetStops are the tabstops on the lines in view that are not the one
// being filled in, for the view to mark; an empty one has no width to mark.
func (e *Editor) SnippetStops() []Hit {
	s := e.snipAlive()
	if s == nil || e.height <= 0 {
		return nil
	}
	var hits []Hit
	for i, st := range s.stops {
		from, to := e.posAt(st[0]), e.posAt(st[1])
		for n := max(from.Line, e.top); i != s.cur && st[0] < st[1] && n <= min(to.Line, e.top+e.height-1); n++ {
			h := Hit{Line: n, To: len(e.lines[n])}
			if n == from.Line {
				h.From = from.Col
			}
			if n == to.Line {
				h.To = to.Col
			}
			hits = append(hits, h)
		}
	}
	return hits
}

// snipAlive is the snippet, brought up to date with the text, nil when it has
// ended: in INSERT or SELECT (the modeless editor is in INSERT) the cursor is out of every tabstop (o, or A and
// typing after the text, are), as nvim's CursorMovedI has it; moving in NORMAL
// does not end it.
func (e *Editor) snipAlive() *snippet {
	e.snipSync()
	if s := e.snip; s != nil && (e.mode == Insert || e.mode == Replace || e.mode == Select) {
		off := e.offset(e.cur)
		if !slices.ContainsFunc(s.stops, func(st [2]int) bool { return st[0] <= off && off <= st[1] }) {
			e.snip = nil
		}
	}
	return e.snip
}

// snipSync moves the snippet's places by what changed in the text since it
// last looked: one change, from the first byte that differs to the last, which
// is of what typing, a completion or a paste make. A change inside the tabstop
// being filled in, or inside another, makes it longer or shorter; one before a
// place moves it; one that cuts a tabstop leaves a point where it was; the
// snippet's own text growing with a change in it, and ending with any other.
func (e *Editor) snipSync() {
	s := e.snip
	if s == nil || slices.Equal(s.text, e.lines) {
		return
	}
	old, now := strings.Join(s.text, "\n"), strings.Join(e.lines, "\n")
	s.text = slices.Clone(e.lines)
	a, b := changeOf(old, now, e.offset(e.cur))
	d := len(now) - len(old)
	inStop := s.stops[s.cur][0] <= a && b <= s.stops[s.cur][1] // the one being filled in is what changed
	for i := range s.stops {
		from, to := s.stops[i][0], s.stops[i][1]
		switch {
		case i == s.cur && from <= a && b <= to, i != s.cur && from < a && b < to: // typed in
			to += d
		case b <= from: // before it
			from, to = from+d, to+d
		case a >= to: // after it
		default: // over an end of it: a point where the change began
			from, to = a, a
		}
		s.stops[i] = [2]int{from, to}
	}
	switch {
	case a <= s.from && b >= s.to && !inStop: // taken whole
		e.snip = nil
	case s.from <= a && b <= s.to:
		s.to += d
	default: // a change outside it, or over an end of it
		e.snip = nil
	}
}

// jump is Tab (forward) and S-Tab: the next tabstop is selected, in INSERT
// when it is empty; past the last one the snippet ends, the cursor at the end
// of its text in INSERT (nvim's $0), before the first nothing happens.
func (e *Editor) jump(forward bool) {
	s := e.snip
	n := s.cur - 1
	if forward {
		n = s.cur + 1
	}
	switch {
	case n < 0:
	case n >= len(s.stops):
		end := e.posAt(s.to)
		e.snip = nil
		e.place(end, end)
	default:
		s.cur = n
		e.goTo(n)
	}
}

// goTo selects tabstop i.
func (e *Editor) goTo(i int) {
	st := e.snip.stops[i]
	e.place(e.posAt(st[0]), e.posAt(st[1]))
}

// place selects from to to, or puts the cursor in INSERT at from when there is
// nothing between; it leaves INSERT as esc does but for the cursor stepping
// back.
func (e *Editor) place(from, to Pos) {
	if e.plain {
		e.endChange()
		e.anchor, e.cur, e.want = &from, to, wantUnset
		if from == to {
			e.anchor = nil
		}
		e.ins.start, e.ins.fresh, e.ins.arrowed = e.cur, true, false
		return
	}
	if e.ins != nil {
		e.dropIndent()
		e.ins = nil
		e.endChange()
	}
	if e.visual() {
		e.endVisual()
	}
	e.keys, e.want = nil, wantUnset
	if from == to {
		e.cur = from
		e.startInsert(Insert, cmd{name: "i"})
		return
	}
	e.vstart, e.mode = from, Select
	if to.Col > 0 {
		e.cur = Pos{to.Line, prev(e.lines[to.Line], to.Col)}
	} else { // it ends with a line break: the end of the line before is its last place
		e.cur = Pos{to.Line - 1, len(e.lines[to.Line-1])}
	}
}

// selectKey is key k in SELECT (:help Select-mode, measured on nvim 0.12.4):
// a character, a paste or ↵ takes the selection's place, in INSERT; BS and
// Del delete it, in NORMAL; esc and C-c end SELECT; the arrow keys move the
// end of the selection; Tab and S-Tab go to the other tabstops. The keys that
// go in and out of SELECT (gh, C-g, C-o) are not here.
func (e *Editor) selectKey(k string) {
	switch k {
	case "<Esc>", "<C-c>":
		e.endVisual()
	case "<Tab>", "<S-Tab>":
		if e.snipAlive() != nil {
			e.jump(k == "<Tab>")
		}
	case "<Left>", "<Right>", "<Up>", "<Down>":
		e.keys = []string{map[string]string{"<Left>": "h", "<Right>": "l", "<Up>": "k", "<Down>": "j"}[k]}
		e.normal()
	case "<BS>", "<C-h>", "<Del>":
		e.mode = Visual
		e.visualOp("d", cmd{})
	default:
		if keyText(k) != "" || k == "<CR>" {
			e.mode = Visual
			e.visualOp("c", cmd{})
			e.insertKey(k)
		}
	}
}

// changeOf is what changed from old to now as old[a:b] replaced: from the first
// byte that differs to the last. A text put in or taken out between equal bytes
// (a line break before another) could have been at any of several places: it is
// taken to be where the cursor, at offset at of now, is after it, as typing has it.
func changeOf(old, now string, at int) (a, b int) {
	for a < len(old) && a < len(now) && old[a] == now[a] {
		a++
	}
	same := 0 // the bytes the two end alike
	for same < min(len(old), len(now)) && old[len(old)-1-same] == now[len(now)-1-same] {
		same++
	}
	b = len(old) - min(same, len(old)-a, len(now)-a)
	if r, m := b-a, len(now)-len(old)+b-a; r == 0 || m == 0 {
		lo := min(len(old), len(now)) - same
		a = min(max(at-m, lo), a)
		b = a + r
	}
	return a, b
}
