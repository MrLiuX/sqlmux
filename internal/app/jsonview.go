package app

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	uv "github.com/charmbracelet/ultraviolet"

	"github.com/kackerx/sqlmux/internal/editor"
	"github.com/kackerx/sqlmux/internal/i18n"
	"github.com/kackerx/sqlmux/internal/keymap"
	"github.com/kackerx/sqlmux/internal/ui"
)

// jsonView is a grid's other view (F14.5, §7.6): the page as a JSON array in
// a read only editor, in the place of the table. It is the tab's, not the
// layout's.
type jsonView struct {
	ed  *editor.Editor
	sig string // what the text was made of: it is made again when that changes
}

// jsonViewOf is the JSON view of the grid pane p's current tab shows, nil when
// it shows its table.
func jsonViewOf(p *Pane) *jsonView {
	if s := gridStateOf(p); s != nil {
		return s.jv
	}
	return nil
}

// jsonArea is where pane p's grid is drawn, and so its JSON view.
func (a *App) jsonArea(p *Pane) uv.Rectangle {
	if t := dataOf(p); t != nil {
		return gridRect(a.layout()[p.ID], t)
	}
	return bodyRect(a.layout()[p.ID])
}

// jsonText is the page of tab as a JSON array, each record as Y yanks it
// (§7.5), a table's with its changes in, indented, in full.
func jsonText(tab *Tab) string {
	var objects []string
	for row := 0; ; row++ {
		fs, ok := jsonFields(tab, row)
		if !ok {
			break
		}
		raw, err := rowRaw(fs)
		if err != nil {
			return err.Error()
		}
		objects = append(objects, string(raw))
	}
	text, err := indentJSON([]byte("[" + strings.Join(objects, ",") + "]"))
	if err != nil {
		return err.Error()
	}
	return text
}

// jsonSig is what jsonText depends on: the rows, as the page's slice and its
// length, the columns as they are shown, the changes in a table.
func jsonSig(tab *Tab) string {
	if t := tab.Data; t != nil {
		return fmt.Sprintf("%p %d %v %d %d %d %d", t.page.Rows, len(t.page.Rows), t.ordered(), len(t.edits), len(t.added), len(t.deleted), t.seq)
	}
	rt := tab.Result
	return fmt.Sprintf("%p %d %d", rt.page.Rows, len(rt.page.Rows), len(rt.page.Cols))
}

// jsonSync makes the text of pane p's JSON view what its tab has now, as a
// page comes in or a change is made, the cursor where it was as far as the
// new text allows; the view, nil with none.
func (a *App) jsonSync(p *Pane) *jsonView {
	jv := jsonViewOf(p)
	if jv == nil {
		return nil
	}
	if sig := jsonSig(p.tab()); sig != jv.sig {
		jv.sig = sig
		jv.ed.Load(jsonText(p.tab()))
	}
	jv.ed.SetPlain(a.plain)
	return jv
}

// jsonSize gives the editor the size of its area, which H M L, C-d and the
// scrolling go by.
func (a *App) jsonSize(p *Pane, jv *jsonView) {
	a.sizeEditor(jv.ed, ui.Query{Lines: jv.ed.Lines()}.TextArea(a.jsonArea(p)))
}

// viewJSON is grid.view.json on the focused table or result: its JSON view
// opens, or closes; turned the other way, it goes (F14.5).
func (a *App) viewJSON() tea.Cmd {
	p := a.focused()
	s := gridStateOf(p)
	if s == nil {
		return nil
	}
	if s.jv != nil {
		s.jv = nil
		return nil
	}
	ed := editor.New("")
	ed.ReadOnly = true
	s.transpose, s.jv = false, &jsonView{ed: ed}
	a.jsonSync(p)
	return nil
}

// jsonKey gives key k, which no binding of the view took, to its editor: esc
// in NORMAL, and q, bring the table back (§6.8).
func (a *App) jsonKey(p *Pane, k keymap.Key) tea.Cmd {
	jv := a.jsonSync(p)
	if jv == nil { // an action run with no JSON view (the palette's run of everything)
		return nil
	}
	a.jsonSize(p, jv)
	if (jv.ed.Mode() == editor.Normal || jv.ed.Plain()) && jv.ed.Pending() == "" && (k == keymap.Esc || k == "q") {
		return a.viewJSON()
	}
	return a.jsonDid(p, jv, jv.ed.Feed(string(k)))
}

// jsonDid acts on what the editor did: the clipboard, the toasts.
func (a *App) jsonDid(p *Pane, jv *jsonView, eff editor.Effect) tea.Cmd {
	cmds := a.vimCmds(eff, func(text string, pt editor.ClipPut) tea.Cmd {
		if jsonViewOf(p) != jv {
			return nil
		}
		return a.jsonDid(p, jv, jv.ed.PutClip(text, pt))
	})
	if eff.ReadOnly {
		cmds = append(cmds, a.showToast(i18n.T("只读")))
	}
	if eff.Ex != "" {
		cmds = append(cmds, a.showToast(i18n.T("不支持的命令：%s", eff.Ex)))
	}
	return tea.Batch(cmds...)
}

// drawJSON paints pane p's JSON view in its grid's place.
func (a *App) drawJSON(f *ui.Frame, p *Pane) {
	jv := a.jsonSync(p)
	if jv == nil {
		return
	}
	a.jsonSize(p, jv)
	ed := jv.ed
	q := ui.Query{
		Lines: ed.Lines(), TabWidth: ed.TabWidth, Top: ed.Top(), Left: ed.Left(), Pane: ui.ValuePane,
		Cursor: ui.TextPos(ed.Cursor()), CursorCol: ed.CursorCol(), Sel: vimSel(ed),
		Colors: ui.ValueColors(a.theme, strings.Join(ed.Lines(), "\n"), true),
	}
	if fl := a.flash; fl != nil && fl.pane == p.ID {
		q.Yank = fl.text
	}
	q.Prompt, q.Text, q.Pos, _ = ed.CmdLine()
	if cur := q.Draw(f, a.jsonArea(p)); cur.X >= 0 && a.win().Focus == p.ID {
		f.Cursor = &cur
	}
}

// jsonAs is what the JSON view's ] [ and R do: what the table's and the
// result's do, by the name of each (F14.5).
func (a *App) jsonAs(what string) tea.Cmd {
	prefix := "grid."
	if resultOf(a.focused()) != nil {
		prefix = "result."
	}
	if what == "refresh" {
		what = map[string]string{"grid.": "grid.refresh", "result.": "result.rerun"}[prefix]
		return a.run(what, 0)
	}
	return a.run(prefix+what, 0)
}
