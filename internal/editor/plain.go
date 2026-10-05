package editor

import "strings"

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

// plainSelection is the selection of the modeless editor: from the end it
// began at to the cursor, in text order and as the vim's VISUAL has it, the
// last character in, the end of a line counting as one; ok is false with
// none.
func (e *Editor) plainSelection() (from, to Pos, ok bool) {
	if e.anchor == nil || *e.anchor == e.cur {
		return Pos{}, Pos{}, false
	}
	from, to = *e.anchor, e.cur
	if to.less(from) {
		from, to = to, from
	}
	if to.Col > 0 {
		to.Col = prev(e.lines[to.Line], to.Col)
	} else {
		to = Pos{to.Line - 1, len(e.lines[to.Line-1])}
	}
	return from, to, true
}

// SelectionSpan is the selection as the text between two cursor places, from
// in and to out; ok is false with none.
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

// SelectedText is what the modeless selection holds, "" with none: the
// statement run with a selection runs it.
func (e *Editor) SelectedText() string {
	from, to, ok := e.SelectionSpan()
	if !ok {
		return ""
	}
	if from.Line == to.Line {
		return e.lines[from.Line][from.Col:to.Col]
	}
	parts := []string{e.lines[from.Line][from.Col:]}
	parts = append(parts, e.lines[from.Line+1:to.Line]...)
	return strings.Join(append(parts, e.lines[to.Line][:to.Col]), "\n")
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

// plainKey is key k in the modeless editor. A selection is replaced by what
// is typed, taken out by BS and Delete; the arrow keys end it, with Shift
// they move its far end.
func (e *Editor) plainKey(k string) {
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
			e.redo(1)
		}
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
			e.arrow("<" + k[3:])
		} else {
			e.step(k == "<S-Right>")
		}
		return
	case "<Left>", "<Right>":
		if from, to, ok := e.SelectionSpan(); ok { // it ends where the key points
			was := e.cur
			e.cur, e.anchor = map[string]Pos{"<Left>": from, "<Right>": to}[k], nil
			e.want = wantUnset
			e.insMoved(was)
			return
		}
		e.step(k == "<Right>")
		return
	case "<C-a>", "<C-e>":
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
		if keyText(k) != "" || k == "<CR>" || k == "<Tab>" || strings.HasPrefix(k, pasteKey) {
			e.deleteSelection()
		} else {
			e.anchor = nil
		}
	}
	e.insertKey(k)
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
