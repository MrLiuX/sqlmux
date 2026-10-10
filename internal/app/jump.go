package app

import (
	"cmp"
	"fmt"
	"slices"
	"strings"
	"unicode"

	tea "charm.land/bubbletea/v2"
	uv "github.com/charmbracelet/ultraviolet"

	"github.com/kackerx/sqlmux/internal/editor"
	"github.com/kackerx/sqlmux/internal/keymap"
	"github.com/kackerx/sqlmux/internal/ui"
)

// jumpLabels are the letters a match's label is, as flash.nvim has them; once
// they are used up the same in capitals (§6.10).
const jumpLabels = "asdfghjklqwertyuiopzxcvbnm"

// jumpState is a flash-style jump going on: the pattern typed and the
// matches of it on the screen as the last key left it, labelled (§6.10,
// F10.8). scanning is set while the frame drawn is the one the matches are
// read from, which has no backdrop and no labels yet.
type jumpState struct {
	pattern  string
	matches  []jumpMatch
	scanning bool
}

// jumpMatch is a place on the screen the pattern is: in the hit region
// target, the cell at where it starts and the width it takes; label is the
// letter that jumps there, "" for none.
type jumpMatch struct {
	target ui.Target
	at     uv.Position
	width  int
	label  string
	dist   int // from the cursor, for the focused pane's: what the labels are given by
}

// cluster is a grapheme on the screen: its text, and the cell it starts at
// and the cells it takes.
type cluster struct {
	s    string
	x, w int
}

// clustersOf is the graphemes of row y of the frame inside the columns
// x0 to x1.
func clustersOf(f *ui.Frame, y, x0, x1 int) []cluster {
	var out []cluster
	for x := x0; x < x1; {
		c := f.Buf.CellAt(x, y)
		if c == nil || c.Width <= 0 { // the half of a wide one
			x++
			continue
		}
		out = append(out, cluster{c.Content, x, c.Width})
		x += c.Width
	}
	return out
}

// sameText is whether grapheme g is the pattern's rune p, ignoring case when
// fold.
func sameText(g string, p rune, fold bool) bool {
	r := []rune(g)
	if len(r) != 1 {
		return false
	}
	if fold {
		return unicode.ToLower(r[0]) == unicode.ToLower(p)
	}
	return r[0] == p
}

// findIn is where pat starts in cs, as indexes into it; from is where to look
// from.
func findIn(cs []cluster, pat []rune, fold bool, from int) int {
	for i := from; i+len(pat) <= len(cs); i++ {
		ok := true
		for j, p := range pat {
			if !sameText(cs[i+j].s, p, fold) {
				ok = false
				break
			}
		}
		if ok {
			return i
		}
	}
	return -1
}

// jumpStart is s: a jump begins, on a console in NORMAL only, as a console's
// VISUAL has its own s (F10.8).
func (a *App) jumpStart() tea.Cmd {
	if c := a.focusedConsole(); c != nil && c.ed.Mode() != editor.Normal {
		return a.consoleKey(a.focused(), c, "s")
	}
	a.jump = &jumpState{}
	return nil
}

// jumpKey is a key the jump has no binding for: a letter that labels a match
// jumps there, any other text goes onto the pattern, the matches read anew.
func (a *App) jumpKey(k keymap.Key) tea.Cmd {
	j := a.jump
	text := keymap.Text(k)
	if text == "" {
		return nil
	}
	if i := slices.IndexFunc(j.matches, func(m jumpMatch) bool { return m.label == text }); i >= 0 {
		return a.jumpTo(j.matches[i])
	}
	j.pattern += text
	a.jumpScan()
	return nil
}

// jumpBack is backspace: the last character of the pattern goes; with none
// left the jump goes on.
func (a *App) jumpBack() {
	if j := a.jump; j != nil && j.pattern != "" {
		r := []rune(j.pattern)
		j.pattern = string(r[:len(r)-1])
		a.jumpScan()
	}
}

// jumpConfirm is ↵: the jump to the match the cursor is nearest, or the first
// there is; none, nothing.
func (a *App) jumpConfirm() tea.Cmd {
	if j := a.jump; j != nil && len(j.matches) > 0 {
		return a.jumpTo(j.matches[0])
	}
	return nil
}

// jumpTo is the click the match stands for, the jump over: a cell is
// focused and gone to, a tree node is where the cursor goes, neither
// opened nor unfolded, a console's cursor is put on the character (§6.10).
func (a *App) jumpTo(m jumpMatch) tea.Cmd {
	a.jump = nil
	t := m.target
	switch {
	case t.Kind == ui.KindCell:
		return tea.Batch(a.run(fmt.Sprintf("pane.focus %d", t.Pane), 0), a.run(t.Action, 0))
	case t.Kind == ui.KindNode:
		a.focusPane(t.Pane)
		a.treeGo(t.I)
	case t.Kind == ui.KindText:
		a.focusPane(t.Pane)
		if c, n, v := a.consoleAt(t.Pane, m.at); c != nil {
			c.ed.Click(n, v)
			c.comp = nil
		}
	}
	return nil
}

// jumpScan reads the matches of the pattern off a frame of the screen as it
// is: in the cells and tree rows, the first of each, in a console's text all
// of them, then gives them labels (§6.10).
func (a *App) jumpScan() {
	j := a.jump
	j.matches = nil
	if j.pattern == "" {
		return
	}
	j.scanning = true
	f := a.drawOn(ui.NewFrame(a.w, a.h, a.theme))
	j.scanning = false
	pat := []rune(j.pattern)
	fold := j.pattern == strings.ToLower(j.pattern) // smartcase: a capital makes it exact
	cursor, haveCursor := a.jumpCursor(f)
	focus := a.focused().ID
	var skip = map[rune]bool{} // what follows a match: not a label, or typing on would take it for one
	for _, h := range f.Hits {
		var rect uv.Rectangle
		switch k := h.Target.Kind; {
		case k == ui.KindCell || k == ui.KindNode:
			rect = h.Rect
		case k == ui.KindText:
			if p := a.win().pane(h.Target.Pane); p != nil && consoleOf(p) != nil {
				view, body := a.consoleView(p, consoleOf(p))
				rect = view.TextArea(body)
			}
		}
		for y := rect.Min.Y; y < rect.Max.Y; y++ {
			cs := clustersOf(f, y, rect.Min.X, rect.Max.X)
			for from := 0; ; {
				i := findIn(cs, pat, fold, from)
				if i < 0 {
					break
				}
				last := cs[i+len(pat)-1]
				m := jumpMatch{target: h.Target, at: uv.Pos(cs[i].x, y), width: last.x + last.w - cs[i].x}
				if h.Target.Pane == focus && haveCursor {
					m.dist = abs(m.at.X-cursor.X) + 2*abs(m.at.Y-cursor.Y)
				}
				j.matches = append(j.matches, m)
				if i+len(pat) < len(cs) {
					if r := []rune(cs[i+len(pat)].s); len(r) == 1 {
						skip[unicode.ToLower(r[0])] = true
					}
				}
				if h.Target.Kind != ui.KindText {
					break // one match to a cell or a row
				}
				from = i + len(pat)
			}
		}
	}
	// the focused pane's by their distance from the cursor, the rest as the screen goes
	slices.SortStableFunc(j.matches, func(x, y jumpMatch) int {
		xf, yf := x.target.Pane == focus, y.target.Pane == focus
		switch {
		case xf != yf && xf:
			return -1
		case xf != yf:
			return 1
		case xf && x.dist != y.dist:
			return cmp.Compare(x.dist, y.dist)
		}
		return cmp.Or(cmp.Compare(x.at.Y, y.at.Y), cmp.Compare(x.at.X, y.at.X))
	})
	var labels []string
	for _, r := range jumpLabels {
		if !skip[r] {
			labels = append(labels, string(r))
		}
	}
	for _, r := range jumpLabels {
		if !skip[r] {
			labels = append(labels, strings.ToUpper(string(r)))
		}
	}
	for i := range j.matches {
		if i < len(labels) {
			j.matches[i].label = labels[i]
		}
	}
}

func abs(n int) int { return max(n, -n) }

// jumpCursor is where the focused pane's cursor is on the frame f: a
// table's cell, the tree's node, the console's cursor.
func (a *App) jumpCursor(f *ui.Frame) (uv.Position, bool) {
	p := a.focused()
	find := func(want ui.Target) (uv.Position, bool) {
		for _, h := range f.Hits {
			if h.Target == want {
				return h.Rect.Min, true
			}
		}
		return uv.Position{}, false
	}
	switch {
	case p == a.win().Tree:
		ns, _ := a.treeNodes()
		cur, _ := a.treeAt(ns)
		return find(ui.Target{Kind: ui.KindNode, Pane: p.ID, I: cur})
	case consoleOf(p) != nil:
		if f.Cursor != nil {
			return uv.Pos(f.Cursor.X, f.Cursor.Y), true
		}
	case dataOf(p) != nil:
		t := dataOf(p)
		return find(ui.Target{Kind: ui.KindCell, Pane: p.ID, Action: fmt.Sprintf("grid.goto %d %d", t.row, t.fieldAt(t.col))})
	default:
		if s := gridStateOf(p); s != nil {
			return find(ui.Target{Kind: ui.KindCell, Pane: p.ID, Action: fmt.Sprintf("grid.goto %d %d", s.row, s.col)})
		}
	}
	return uv.Position{}, false
}

// drawJump paints the jump over the frame the screen is: all of it dim, the
// matches in the match color, each label over the character after its match
// in the yank's colors (§6.10).
func (a *App) drawJump(f *ui.Frame) {
	th := a.theme
	for y := range f.Buf.Height() {
		for x := range f.Buf.Width() {
			f.Buf.CellAt(x, y).Style.Fg = th.Dim
		}
	}
	for _, m := range a.jump.matches {
		for x := m.at.X; x < m.at.X+m.width; x++ {
			f.Buf.CellAt(x, m.at.Y).Style.Fg = th.Match
		}
		if m.label != "" {
			x := min(m.at.X+m.width, f.Buf.Width()-1)
			f.Text(x, m.at.Y, x+1, m.label, uv.Style{Fg: th.Bg, Bg: th.Yank})
		}
	}
}
