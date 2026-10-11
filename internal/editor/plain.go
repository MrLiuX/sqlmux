package editor

import (
	"slices"
	"strings"
	"unicode/utf8"
)

// The editor without modes (tech-design §6.9): INSERT that never ends, with
// a selection, so what is typed is text and the arrow keys move. It is the
// same buffer, undo steps and drawing as the vim's; what differs is the keys
// that come in, and no nvim to check it against.

// SetPlain turns the modeless editing on or off: on, the editor is in
// INSERT and stays there; Esc does nothing in it.
func (e *Editor) SetPlain(on bool) {
	if e.plain == on {
		return
	}
	e.plain, e.anchor = on, nil
	if on && e.mode != Insert {
		e.mode, e.ins = Insert, &insertion{cmd: "i", count: 1, start: e.cur, fresh: true}
	}
}

// Plain is whether the editor has no modes.
func (e *Editor) Plain() bool { return e.plain }

// plainSelection is the selection of the modeless editor as the vim's VISUAL
// has it: from the end it began at to the cursor, in text order, the last
// character in, the end of a line counting as one; ok is false with none.
func (e *Editor) plainSelection() (from, to Pos, ok bool) {
	from, to, ok = e.SelectionSpan()
	if !ok {
		return Pos{}, Pos{}, false
	}
	if to.Col > 0 {
		to.Col = prev(e.lines[to.Line], to.Col)
	} else {
		to = Pos{to.Line - 1, len(e.lines[to.Line-1])}
	}
	return from, to, true
}

// SelectionSpan is the selection as the text between two cursor places, from
// in and to out; ok is false with none. In the modeless editor only.
func (e *Editor) SelectionSpan() (from, to Pos, ok bool) {
	if e.anchor == nil || *e.anchor == e.cur {
		return Pos{}, Pos{}, false
	}
	from, to = *e.anchor, e.cur
	if to.less(from) {
		from, to = to, from
	}
	return from, to, true
}

// HasSelection is whether the modeless editor has a selection.
func (e *Editor) HasSelection() bool {
	_, _, ok := e.SelectionSpan()
	return ok
}

// ClearSelection drops the modeless selection, the text as it is.
func (e *Editor) ClearSelection() { e.anchor = nil }

// deleteSelection takes the selected text out, the cursor where it was; it
// is false with none.
func (e *Editor) deleteSelection() bool {
	from, to, ok := e.SelectionSpan()
	e.anchor = nil
	if !ok {
		return false
	}
	e.arrived()
	e.setLine(from.Line, e.lines[from.Line][:from.Col]+e.lines[to.Line][to.Col:])
	if to.Line > from.Line {
		e.deleteLines(from.Line+1, to.Line+1)
	}
	e.cur, e.want = from, wantUnset
	e.ins.ai = false
	return true
}

// plainMoves are the keys a ReadOnly modeless editor takes: those that move.
var plainMoves = map[string]bool{"<Left>": true, "<Right>": true, "<Up>": true, "<Down>": true, "<S-Left>": true, "<S-Right>": true,
	"<S-Up>": true, "<S-Down>": true, "<C-a>": true, "<C-e>": true}

// plainKey is key k in the modeless editor. A selection is replaced by what
// is typed, taken out by BS and Delete; the arrow keys end it, with Shift
// they move its far end. Typing is one undo step; an arrow key, ↵, a paste
// and a formatting end it.
func (e *Editor) plainKey(k string) {
	if e.ReadOnly && !plainMoves[k] {
		e.eff.ReadOnly = k != "<Esc>"
		return
	}
	in := e.ins
	switch k {
	case "<Esc>":
		return
	case "<C-z>", "<C-y>":
		e.anchor = nil
		e.endChange()
		if k == "<C-z>" {
			e.undo(1)
		} else {
			before, n := e.lines, len(e.undone)
			e.redo(1)
			if len(e.undone) < n { // with nothing to redo the cursor stays
				e.cur = redoEnd(before, e.lines)
			}
		}
		e.clampCursor() // the step's cursor is of the text before it
		in.start, in.fresh, in.arrowed = e.cur, true, false
		e.want = wantUnset
		return
	case "<S-Left>", "<S-Right>", "<S-Up>", "<S-Down>":
		if e.anchor == nil {
			at := e.cur
			e.anchor = &at
		}
		e.curswant()
		if k == "<S-Up>" || k == "<S-Down>" {
			e.arrow("<" + strings.TrimPrefix(k, "<S-"))
		} else {
			e.step(k == "<S-Right>")
		}
		return
	case "<Tab>", "<S-Tab>":
		if e.indentSelection(k == "<S-Tab>") {
			return
		}
		e.anchor = nil
	case "<Left>", "<Right>":
		if from, to, ok := e.SelectionSpan(); ok { // it ends where the key points
			was := e.cur
			e.cur, e.anchor = from, nil
			if k == "<Right>" {
				e.cur = to
			}
			e.want = wantUnset
			e.insMoved(was)
			return
		}
		e.step(k == "<Right>")
		return
	case "<C-a>", "<C-e>":
		// A query never gets C-e: default-novim.toml binds it to query.run
		// and the line end has no key there (§6.9); a WHERE has no such binding.
		was := e.cur
		e.anchor = nil
		if k == "<C-a>" {
			e.cur.Col = 0
		} else {
			e.cur.Col = len(e.line())
		}
		e.want = wantUnset
		e.insMoved(was)
		return
	case "<BS>", "<C-h>", "<Del>", "<C-w>", "<M-BS>":
		if e.deleteSelection() {
			return
		}
		if k == "<C-w>" || k == "<M-BS>" { // C-w stops nowhere: not where typing began
			in.start = Pos{-1, -1}
		}
	case "<Up>", "<Down>":
		e.anchor = nil
	default:
		text := keyText(k)
		if r, _ := utf8.DecodeRuneInString(text); e.AutoPairs && e.surround(r) {
			return
		}
		switch {
		case k == "<CR>": // a step of its own
			e.endChange()
			e.deleteSelection()
		case text != "" || k == "<Tab>":
			e.deleteSelection()
		default:
			e.anchor = nil
		}
	}
	e.insertKey(k)
}

// redoEnd is where the cursor goes when a redo has put text after in place
// of before: the end of what came in, the place the common ending of the
// last line changed starts at; the start of the line the lines went from,
// when none came (§6.9; vim goes back to where the step began).
func redoEnd(before, after []string) Pos {
	p, oldN, newN := diff(before, after)
	if newN == 0 {
		return Pos{Line: min(p, len(after)-1)}
	}
	now := after[p+newN-1]
	same := 0
	if oldN > 0 { // the bytes the two last lines end alike
		old := before[p+oldN-1]
		for same < len(old) && same < len(now) && old[len(old)-1-same] == now[len(now)-1-same] {
			same++
		}
	}
	return Pos{p + newN - 1, head(now, len(now)-same)}
}

// indentSelection is Tab and S-Tab over a selection: the lines it touches
// move a tab stop right or left, as > and < do, and stay selected. It is
// false with none, Tab then being a tab stop's worth of spaces. The last
// line is not one when the selection ends before its first character.
func (e *Editor) indentSelection(left bool) bool {
	from, to, ok := e.SelectionSpan()
	if !ok {
		return false
	}
	if to.Col == 0 && to.Line > from.Line {
		to.Line--
	}
	e.shift(span{start: Pos{from.Line, 0}, end: Pos{to.Line, 0}, linewise: true}, left, 1)
	if slices.Equal(e.snap, e.lines) { // nothing moved: no step
		e.snap = nil
	}
	e.endChange()
	at := Pos{from.Line, 0}
	e.anchor, e.cur, e.want = &at, Pos{to.Line, len(e.lines[to.Line])}, wantUnset
	return true
}

// surround is an opening bracket or quote typed over a selection, which it
// puts the pair round, the text staying selected; it is false when r is
// none or nothing is selected (§7.9, F6.6).
func (e *Editor) surround(r rune) bool {
	closer, ok := closers[r]
	from, to, sel := e.SelectionSpan()
	if !ok || !sel {
		return false
	}
	open := string(r)
	backwards := e.cur.less(*e.anchor)
	e.arrived()
	l := e.lines[to.Line]
	e.setLine(to.Line, l[:to.Col]+closer+l[to.Col:])
	l = e.lines[from.Line]
	e.setLine(from.Line, l[:from.Col]+open+l[from.Col:])
	from.Col += len(open)
	if to.Line == from.Line {
		to.Col += len(open)
	}
	if backwards {
		from, to = to, from
	}
	e.anchor, e.cur, e.want = &from, to, wantUnset
	e.endChange() // a step of its own: typing over the pair's text does not undo with it
	return true
}

// step is a character right or left, past the end of a line to the next, as
// an editor's arrow keys go.
func (e *Editor) step(right bool) {
	was, l := e.cur, e.line()
	switch {
	case right && e.cur.Col < len(l):
		e.cur.Col = next(l, e.cur.Col)
	case right && e.cur.Line+1 < len(e.lines):
		e.cur = Pos{e.cur.Line + 1, 0}
	case !right && e.cur.Col > 0:
		e.cur.Col = prev(l, e.cur.Col)
	case !right && e.cur.Line > 0:
		e.cur = Pos{e.cur.Line - 1, len(e.lines[e.cur.Line-1])}
	}
	e.want = wantUnset
	e.insMoved(was)
}
