package app

import (
	"encoding/json"
	"strings"

	tea "charm.land/bubbletea/v2"
	uv "github.com/charmbracelet/ultraviolet"

	"github.com/kackerx/sqlmux/internal/db"
	"github.com/kackerx/sqlmux/internal/editor"
	"github.com/kackerx/sqlmux/internal/i18n"
	"github.com/kackerx/sqlmux/internal/keymap"
	"github.com/kackerx/sqlmux/internal/ui"
)

// valueEditor is a cell's value open in the editor of a box under its cell
// (§10「值编辑器」, F12.2): the editor of the queries, JSON laid out and
// colored, its text written back to the cell as a change.
type valueEditor struct {
	ed       *editor.Editor
	title    string
	json     bool
	readOnly bool   // a result's, or a cell that takes no change
	trail    bool   // the value ended in a newline, which the editor does not keep
	opened   string // what the editor began with: written back, it is no change
	saved    string // what :w last wrote back, or opened

	cell *cellSrc // where it goes back to; none for a result's
}

// valueOpen is e (grid.value): the cell under the cursor of the focused table
// or result in a box of its own, the text a NULL or DEFAULT starts empty, as
// in the cell's own input. A cell that takes no change opens too, read only.
func (a *App) valueOpen() tea.Cmd {
	p := a.focused()
	v := &valueEditor{}
	var text, typ string
	switch rt := resultOf(p); {
	case dataOf(p) != nil:
		t, ok := a.focusedGrid()
		if !ok {
			return nil
		}
		c, ok := t.cellAtCursor()
		if !ok {
			return nil
		}
		v.cell, text, typ, v.title = &c, c.text, c.col.Type, c.key.col
		v.readOnly = !c.editable || c.why != ""
	case rt != nil && rt.page.Cols != nil && rt.run != nil && rt.row < len(rt.page.Rows):
		c := rt.page.Cols[rt.col]
		text = rt.page.Rows[rt.row][rt.col].S
		typ, v.title, v.readOnly = c.Type, c.Name, true
	default:
		return nil
	}
	if typ != "" {
		v.title += " · " + typ
	}
	if raw := []byte(text); json.Valid(raw) && (colType(typ) == ui.ColJSON || isJSONObject(text)) {
		if pretty, err := indentJSON(raw); err == nil {
			text, v.json = pretty, true
		}
	}
	v.trail = !v.json && strings.HasSuffix(text, "\n")
	v.ed = editor.New(text)
	v.ed.ReadOnly = v.readOnly
	v.ed.SetPlain(a.plain)
	v.opened = strings.Join(v.ed.Lines(), "\n")
	v.saved = v.opened
	a.value = v
	return nil
}

// isJSONObject is whether s, valid JSON, is an object or an array: a number
// or a string in a text column is none to lay out.
func isJSONObject(s string) bool {
	s = strings.TrimSpace(s)
	return strings.HasPrefix(s, "{") || strings.HasPrefix(s, "[")
}

func inValue(a *App) bool { return a.value != nil }

// valueActive is whether the value editor has the keys: a question asked over
// it has them instead.
func (a *App) valueActive() bool { return a.value != nil && a.confirm == nil }

// text is what the editor holds.
func (v *valueEditor) text() string { return strings.Join(v.ed.Lines(), "\n") }

// valueBox is where the editor is: left at the cursor's cell and from the row
// under it down to the foot of the pane, half the pane wide (40 at least, the
// pane at most); over the cell, its bottom border on the row above it, when
// the rows below are fewer than 8 and fewer than those above (F12.8).
func (a *App) valueBox() uv.Rectangle {
	p := a.focused()
	pane := bodyRect(a.layout()[p.ID])
	cell := uv.Rect(pane.Min.X, pane.Min.Y, 0, 1) // a cell scrolled out of sight: from the pane's top
	// ponytail: the grid is made again on every call; a page of thousands of rows wants the cell kept
	if _, g, area, ok := a.gridOf(p); ok {
		if r := g.CellRect(area); !r.Empty() {
			cell = r
		}
	}
	w := min(max(pane.Dx()/2, 40), pane.Dx())
	x := min(cell.Min.X, pane.Max.X-w) // the cell is in the pane, which w does not outgrow
	below, above := pane.Max.Y-cell.Max.Y, cell.Min.Y-pane.Min.Y
	if below < valueMinRows && above > below {
		return uv.Rect(x, pane.Min.Y, w, above)
	}
	return uv.Rect(x, cell.Max.Y, w, below)
}

// valueMinRows is the fewest rows the value editor's box has when there is
// room for them.
const valueMinRows = 8

// valueText is where the editor's text goes in its box.
func (a *App) valueText() uv.Rectangle { return a.valueTextBox().Inner(a.valueBox()) }

// valueTextBox is the editor's box: the column and the type for a title, the
// keys under the text, from the keymap where there is a key (F12.8).
func (a *App) valueTextBox() ui.TextBox {
	v := a.value
	hs := []ui.Hint{{Key: a.keys.Hint("value.leave", "value"), Label: i18n.T("关闭"), Action: "value.leave"}}
	switch {
	case a.plain: // no : commands, no gq
		if !v.readOnly {
			hs = append(hs, ui.Hint{Key: a.keys.Hint("value.save", "value"), Label: i18n.T("写回并关闭"), Action: "value.save"})
		}
	case v.readOnly:
		hs = append(hs, ui.Hint{Key: "y", Label: i18n.T("复制")})
	default:
		hs = append(hs,
			ui.Hint{Key: a.keys.Hint("value.save", "value"), Label: i18n.T("写回并关闭"), Action: "value.save"},
			ui.Hint{Key: ":w", Label: i18n.T("写回")},
			ui.Hint{Key: "gqq", Label: i18n.T("格式化 JSON")})
	}
	return ui.TextBox{Title: v.title, Hints: bound(hs...)}
}

// valueSize gives the editor the size of its box, which H M L, C-d and the
// scrolling go by.
func (a *App) valueSize() {
	ed := a.value.ed
	text := ui.Query{Lines: ed.Lines()}.TextArea(a.valueText())
	ed.TabWidth, ed.AutoPairs = a.tabWidth, a.autoPairs
	ed.SetHeight(text.Dy())
	ed.SetWidth(text.Dx())
}

// valueAt is the line and display column of the value's text under p.
func (a *App) valueAt(p uv.Position) (int, int) {
	a.valueSize()
	ed := a.value.ed
	text := ui.Query{Lines: ed.Lines()}.TextArea(a.valueText())
	return ed.Top() + p.Y - text.Min.Y, max(ed.Left()+p.X-text.Min.X, 0)
}

// valueWheel scrolls the value by notches, as a query's.
func (a *App) valueWheel(notches int) {
	a.valueSize()
	a.value.ed.Scroll(notches * wheelStep)
}

// valueView is the editor as it draws.
// ponytail: the colors of the whole text are made every frame; a value of
// some MB wants them kept by the text's version, or made for the lines in view.
func (a *App) valueView() ui.Query {
	v := a.value
	ed := v.ed
	q := ui.Query{
		Lines: ed.Lines(), TabWidth: ed.TabWidth, Top: ed.Top(), Left: ed.Left(), Pane: ui.ValuePane,
		Cursor: ui.TextPos(ed.Cursor()), CursorCol: ed.CursorCol(), Sel: vimSel(ed),
		Colors: ui.ValueColors(a.theme, v.text(), v.json),
	}
	if f := a.flash; f != nil {
		q.Yank = f.text
	}
	q.Prompt, q.Text, q.Pos, _ = ed.CmdLine()
	return q
}

// drawValue paints the box over the frame.
func (a *App) drawValue(f *ui.Frame) {
	box, tb := a.valueBox(), a.valueTextBox()
	if !tb.Frame(f, box) {
		return
	}
	a.valueSize()
	if cur := a.valueView().Draw(f, tb.Inner(box)); cur.X >= 0 {
		f.Cursor = &cur
	}
}

// valueKey gives key k to the value's editor. esc in NORMAL, and q where it is
// read only, close it; C-c is esc (§6.8). Without modes a selection ends first.
func (a *App) valueKey(k keymap.Key) tea.Cmd {
	v := a.value
	a.valueSize()
	if k == keymap.Esc && v.ed.Plain() && v.ed.HasSelection() { // as in a plain query: the selection first (§6.9)
		v.ed.ClearSelection()
		return nil
	}
	if (v.ed.Mode() == editor.Normal || v.ed.Plain()) && v.ed.Pending() == "" && (k == keymap.Esc || k == "q" && v.readOnly) {
		return a.valueClose(false)
	}
	return a.valueDid(v.ed.Feed(string(k)))
}

// valueDid acts on what the editor did: the clipboard, the toasts, a : command
// it leaves, gq.
func (a *App) valueDid(eff editor.Effect) tea.Cmd {
	v := a.value
	cmds := a.vimCmds(eff, func(text string, p editor.ClipPut) tea.Cmd {
		if a.value != v {
			return nil
		}
		return a.valueDid(v.ed.PutClip(text, p))
	})
	if eff.ReadOnly {
		cmds = append(cmds, a.showToast(i18n.T("只读"), toastTTL))
	}
	if eff.Ex != "" {
		cmds = append(cmds, a.valueEx(eff.Ex))
	}
	if eff.Format != nil {
		cmds = append(cmds, a.valueFormat())
	}
	return tea.Batch(cmds...)
}

// valueEx runs a : command of the editor: :w writes back and stays, :wq and
// :x write back and close, :q closes, :q! closes whatever was changed.
func (a *App) valueEx(c string) tea.Cmd {
	switch strings.TrimSpace(c) {
	case "w":
		cmd, _ := a.valueWrite()
		return cmd
	case "wq", "x":
		cmd, ok := a.valueWrite()
		if ok {
			a.value = nil
		}
		return cmd
	case "q":
		return a.valueClose(false)
	case "q!":
		return a.valueClose(true)
	}
	return a.showToast(i18n.T("不支持的命令：%s", c), toastTTL)
}

// valueFormat is gq: the whole text laid out as JSON, or left as it is and
// toasted when it is none.
func (a *App) valueFormat() tea.Cmd {
	v := a.value
	pretty, err := indentJSON([]byte(v.text()))
	if err != nil {
		return a.showToast(i18n.T("不是 JSON"), toastTTL)
	}
	v.json = true
	v.ed.Load(pretty)
	return nil
}

// valueWrite is :w: the text is the cell's change, unless it is what the
// editor began with, which takes the change the cell had back; an empty text
// is an empty string, not NULL (§10.1). What is no value of the column and a
// read-only value are toasted and are not ok.
func (a *App) valueWrite() (cmd tea.Cmd, ok bool) {
	v := a.value
	if v.readOnly {
		return a.showToast(i18n.T("只读"), toastTTL), false
	}
	c := v.cell
	text := v.text()
	switch {
	case text != v.opened:
		if why := c.t.checkText(c.key.col, text); why != "" { // as the cell's input (§10.7)
			return a.showToast(why, toastTTL), false
		}
		out := text
		if v.trail && text != "" { // an empty value stays empty
			out += "\n"
		}
		c.t.put(c.add, c.key, edit{val: db.Val{S: out}, orig: c.orig})
	case c.had:
		c.t.put(c.add, c.key, c.before)
	case c.add != nil:
		delete(c.add.cells, c.key.col)
	default:
		delete(c.t.edits, c.key)
	}
	v.saved = text
	return nil, true
}

// valueClose closes the box; with text not written back it asks first, unless
// force.
func (a *App) valueClose(force bool) tea.Cmd {
	v := a.value
	if force || v.text() == v.saved {
		a.value = nil
		return nil
	}
	a.confirm = &confirmBox{text: i18n.T("放弃修改？"), yes: i18n.T("放弃"), then: func() tea.Cmd {
		if a.value == v {
			a.value = nil
		}
		return nil
	}}
	return nil
}

// valueSave is C-s: written back and closed.
func (a *App) valueSave() tea.Cmd {
	cmd, ok := a.valueWrite()
	if ok {
		a.value = nil
	}
	return cmd
}
