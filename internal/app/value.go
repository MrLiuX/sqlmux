package app

import (
	"encoding/json"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/kackerx/sqlmux/internal/db"
	"github.com/kackerx/sqlmux/internal/editor"
	"github.com/kackerx/sqlmux/internal/i18n"
	"github.com/kackerx/sqlmux/internal/keymap"
	"github.com/kackerx/sqlmux/internal/ui"
)

// valueEditor is a cell's value open in the editor of a box in the middle
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

	// where it goes back to: a table's cell, the change it had before, if any
	tab    *dataTab
	key    editKey
	add    *newRow
	orig   db.Val
	before edit
	had    bool
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
		sr, key, ok := t.cursor()
		if !ok {
			return nil
		}
		field := t.fieldAt(t.col)
		name := t.page.Cols[field].Name
		v.tab, v.add, v.key = t, sr.add, editKey{key, name}
		v.orig = db.Val{Null: true} // a row added's: DEFAULT, till set
		if sr.add == nil {
			v.orig = t.page.Rows[sr.rec][field]
		}
		cur := v.orig
		if e, ok := t.change(sr, key, name); ok {
			cur, v.before, v.had = e.val, e, true
		}
		text = cur.S // a NULL or DEFAULT has none
		col := t.column(name)
		typ, v.title = col.Type, name
		v.readOnly = t.whyNoEdit() != "" || col.Fixed || !t.editable(sr, key)
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

// valueText is what the editor holds.
func (v *valueEditor) text() string { return strings.Join(v.ed.Lines(), "\n") }

// valueSize gives the editor the size of its box, which H M L, C-d and the
// scrolling go by.
func (a *App) valueSize() {
	ed := a.value.ed
	text := ui.Query{Lines: ed.Lines()}.TextArea(ui.TextBox{}.Inner(a.popupBox()))
	ed.TabWidth, ed.AutoPairs = a.tabWidth, a.autoPairs
	ed.SetHeight(text.Dy())
	ed.SetWidth(text.Dx())
}

// valueView is the editor as it draws.
// ponytail: the colors of the whole text are made every frame; a value of
// some MB wants them kept by the text's version, or made for the lines in view.
func (a *App) valueView() ui.Query {
	v := a.value
	ed := v.ed
	q := ui.Query{
		Lines: ed.Lines(), TabWidth: ed.TabWidth, Top: ed.Top(), Left: ed.Left(),
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
	box := a.popupBox()
	tb := ui.TextBox{Title: a.value.title}
	if !tb.Frame(f, box) {
		return
	}
	inner := tb.Inner(box)
	a.valueSize()
	if cur := a.valueView().Draw(f, inner); cur.X >= 0 {
		f.Cursor = &cur
	}
}

// valueKey gives key k to the value's editor. esc in NORMAL, and q where it is
// read only, close it; C-c is esc (§6.8).
func (a *App) valueKey(k keymap.Key) tea.Cmd {
	v := a.value
	a.valueSize()
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
// is an empty string, not NULL (§10.1). What is no value of the column, a row
// that went meanwhile and a read-only value are toasted and are not ok.
func (a *App) valueWrite() (cmd tea.Cmd, ok bool) {
	v := a.value
	if v.readOnly {
		return a.showToast(i18n.T("只读"), toastTTL), false
	}
	t := v.tab
	if v.add != nil && !slices.Contains(t.added, v.add) {
		return a.showToast(i18n.T("这一行已经不在了"), toastTTL), false
	}
	text := v.text()
	switch {
	case text != v.opened:
		if why := t.checkText(v.key.col, text); why != "" { // as the cell's input (§10.7)
			return a.showToast(why, toastTTL), false
		}
		out := text
		if v.trail {
			out += "\n"
		}
		t.put(v.add, v.key, edit{val: db.Val{S: out}, orig: v.orig})
	case v.had:
		t.put(v.add, v.key, v.before)
	case v.add != nil:
		delete(v.add.cells, v.key.col)
	default:
		delete(t.edits, v.key)
	}
	v.saved = text
	return nil, true
}

// valueClose closes the box; with text not written back it asks first, unless
// force.
func (a *App) valueClose(force bool) tea.Cmd {
	v := a.value
	if force || v.readOnly || v.text() == v.saved {
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
