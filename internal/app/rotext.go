package app

import (
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/kackerx/sqlmux/internal/editor"
	"github.com/kackerx/sqlmux/internal/i18n"
	"github.com/kackerx/sqlmux/internal/keymap"
	"github.com/kackerx/sqlmux/internal/ui"
)

// What the value editor's box and a grid's JSON view have in common (F12.2,
// F14.5): an editor over a text that colors, selects and yanks as a query's
// does, with a way out. Each has its own scope in the keymap, and its own
// leave.

// roKey gives key k, which no binding of the scope took, to ed: without modes
// esc ends a selection first; esc at rest, in NORMAL or without modes, leaves,
// and q does where leaveOnQ; the rest is the editor's, did acting on what it did
// (§6.8).
func (a *App) roKey(ed *editor.Editor, k keymap.Key, leaveOnQ bool, leave func() tea.Cmd, did func(editor.Effect) tea.Cmd) tea.Cmd {
	if k == keymap.Esc && ed.Plain() && ed.HasSelection() { // as in a plain query: the selection first (§6.9)
		ed.ClearSelection()
		return nil
	}
	if (ed.Mode() == editor.Normal || ed.Plain()) && ed.Pending() == "" && (k == keymap.Esc || k == "q" && leaveOnQ) {
		return leave()
	}
	return did(ed.Feed(string(k)))
}

// roDid acts on what a read only editor did: the clipboard, through put when
// the clipboard's answer comes, the toast that it is read only, a : command
// through ex.
func (a *App) roDid(eff editor.Effect, put func(string, editor.ClipPut) tea.Cmd, ex func(string) tea.Cmd) tea.Cmd {
	cmds := a.vimCmds(eff, put)
	if eff.ReadOnly {
		cmds = append(cmds, a.showToast(i18n.T("只读")))
	}
	if eff.Ex != "" {
		cmds = append(cmds, ex(eff.Ex))
	}
	return tea.Batch(cmds...)
}

// textView is ed as it draws, its text in the colors of JSON or not; pane is
// what a click on the text is on, ui.ValuePane for the box's. A yank flashes in
// the box, and in the pane it was made in.
func (a *App) textView(ed *editor.Editor, pane int, json bool) ui.Query {
	q := ui.Query{
		Lines: ed.Lines(), TabWidth: ed.TabWidth, Top: ed.Top(), Left: ed.Left(), Pane: pane,
		Cursor: ui.TextPos(ed.Cursor()), CursorCol: ed.CursorCol(), Sel: vimSel(ed),
		Colors: ui.ValueColors(a.theme, strings.Join(ed.Lines(), "\n"), json),
	}
	if f := a.flash; f != nil && (pane == ui.ValuePane || f.pane == pane) {
		q.Yank = f.text
	}
	q.Prompt, q.Text, q.Pos, _ = ed.CmdLine()
	searchView(ed, &q)
	return q
}

// searchView puts in q the matches of ed's last search that are lit, and the
// line the search left under the text (F15.9).
func searchView(ed *editor.Editor, q *ui.Query) {
	for _, h := range ed.Hits() {
		q.Hits = append(q.Hits, ui.Span(h))
	}
	q.Stat, q.StatRight, _ = ed.Stat()
}
