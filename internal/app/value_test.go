package app

import (
	"image/color"
	"strconv"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/exp/golden"

	"github.com/kackerx/sqlmux/internal/db"
	"github.com/kackerx/sqlmux/internal/editor"
	"github.com/kackerx/sqlmux/internal/ui"
)

// valueTab is t_order's first row with its meta, a jsonb, open in the value
// editor.
func valueTab(t *testing.T) (*App, *dataTab) {
	t.Helper()
	a := wide(160, 45)
	tab := loadOrders(t, a, 3)
	feed(t, a, "4le")
	if a.value == nil {
		t.Fatal("e does not open the value editor")
	}
	return a, tab
}

// A json column opens as 2-space JSON in a box titled by its column and type,
// the text colored by token (F12.2).
func TestValueOpensJSON(t *testing.T) {
	a, _ := valueTab(t)
	v := a.value
	want := "{\n  \"n\": 1,\n  \"tags\": [\n    \"a\",\n    \"b\"\n  ]\n}"
	if v.title != "meta · jsonb" || !v.json || v.readOnly || v.text() != want || a.mode().String() != "normal" {
		t.Errorf("title %q json %v read only %v mode %v:\n%s", v.title, v.json, v.readOnly, a.mode(), v.text())
	}
	f := a.render()
	if !strings.Contains(f.String(), " meta · jsonb ") {
		t.Error("no title")
	}
	th := a.theme
	for _, c := range []struct {
		at   string
		off  int // from where at starts to the character that has the color
		want color.Color
	}{{`  "n"`, 2, th.SQLColumn}, {`: 1,`, 2, th.Number}, {`    "a"`, 4, th.SQLString}, {`tags": [`, 7, th.SQLOperator}} {
		found := false
		for y := range a.h {
			row := ""
			for x := range a.w {
				row += f.Buf.CellAt(x, y).Content
			}
			if i := strings.Index(row, c.at); i >= 0 && y > 3 && y < 12 {
				if x := len([]rune(row[:i])) + c.off; f.Buf.CellAt(x, y).Style.Fg == c.want {
					found = true
				}
			}
		}
		if !found {
			t.Errorf("%s is not in its color", c.at)
		}
	}
	golden.RequireEqual(t, f.String())
}

// :w records the text as the cell's change and the box stays; with it written,
// :q closes at once; with nothing written, :q asks; ↵ in the question's
// "discard" closes it and nothing was recorded (F12.2).
func TestValueWriteAndClose(t *testing.T) {
	a, tab := valueTab(t)
	key := a.value.cell.key
	feed(t, a, "j$i9<Esc>:w<CR>")
	if e, ok := tab.edits[key]; !ok || !strings.Contains(e.val.S, `"n": 19,`) || strings.Count(e.val.S, "\n") != 6 || a.value == nil {
		t.Fatalf("after :w: %+v %v, box %v", e, ok, a.value != nil)
	}
	feed(t, a, ":q<CR>")
	if a.value != nil || a.confirm != nil {
		t.Errorf(":q with it written: box %v, question %v", a.value != nil, a.confirm != nil)
	}

	feed(t, a, "e")
	feed(t, a, "ggx:q<CR>")
	if a.value == nil || a.confirm == nil || !strings.Contains(a.confirm.text, "放弃") {
		t.Fatalf(":q with a change: box %v, question %v", a.value != nil, a.confirm != nil)
	}
	feed(t, a, "y")
	if a.value != nil || a.confirm != nil || !strings.Contains(tab.edits[key].val.S, `"n": 19,`) {
		t.Errorf("discarded: box %v, edit %q", a.value != nil, tab.edits[key].val.S)
	}
}

// C-s and :wq write back and close; :q! closes whatever was changed; esc in
// NORMAL closes with the question; esc in INSERT is the editor's (F12.2).
func TestValueKeys(t *testing.T) {
	a, tab := valueTab(t)
	key := a.value.cell.key
	feed(t, a, "jf1r2<C-s>")
	if a.value != nil || !strings.Contains(tab.edits[key].val.S, `"n": 2,`) {
		t.Errorf("C-s: box %v, %q", a.value != nil, tab.edits[key].val.S)
	}
	delete(tab.edits, key)
	feed(t, a, "e")
	feed(t, a, "jf1r3A?<Esc>")
	if a.value == nil || a.value.ed.Mode() != editor.Normal {
		t.Fatalf("esc in INSERT: box %v", a.value != nil)
	}
	feed(t, a, "<Esc>")
	if a.value == nil || a.confirm == nil {
		t.Fatalf("esc in NORMAL with a change: %v %v", a.value != nil, a.confirm != nil)
	}
	feed(t, a, "n")
	feed(t, a, ":wq<CR>")
	if a.value == nil || a.confirm != nil { // "n": 3,? is no JSON: refused, the box stays
		t.Fatalf(":wq of no JSON: box %v", a.value != nil)
	}
	feed(t, a, "x:wq<CR>")
	if a.value != nil || !strings.Contains(tab.edits[key].val.S, `"n": 3,`) {
		t.Errorf(":wq: box %v, %q", a.value != nil, tab.edits[key].val.S)
	}
	delete(tab.edits, key)
	feed(t, a, "e")
	feed(t, a, "jA?<Esc>:q!<CR>")
	if a.value != nil || a.confirm != nil || len(tab.edits) != 0 {
		t.Errorf(":q!: box %v, edits %v", a.value != nil, tab.edits)
	}
	feed(t, a, "e<Esc>") // nothing changed: esc closes
	if a.value != nil || a.confirm != nil {
		t.Errorf("esc with nothing changed")
	}
}

// What was not changed is no change, however JSON was laid out for it; an
// empty text is an empty string, not NULL; a change written and taken back is
// no change (F12.2).
func TestValueNoChangeEmptyAndTakenBack(t *testing.T) {
	a, tab := valueTab(t)
	key := a.value.cell.key
	feed(t, a, ":w<CR>")
	if len(tab.edits) != 0 {
		t.Errorf("an unchanged :w: %v", tab.edits)
	}
	feed(t, a, "jx:w<CR>")
	if len(tab.edits) != 1 {
		t.Fatalf("a change: %v", tab.edits)
	}
	feed(t, a, "u:w<CR>") // back to what it opened with
	if len(tab.edits) != 0 {
		t.Errorf("taken back: %v", tab.edits)
	}
	feed(t, a, "ggdG:w<CR>") // no JSON: refused
	if len(tab.edits) != 0 || a.toast == "" {
		t.Errorf("an empty jsonb: %v, toast %q", tab.edits, a.toast)
	}
	feed(t, a, ":q!<CR>l")
	nkey := editKey{key.row, "note"}
	feed(t, a, "eggdG:w<CR>") // note, a text: empty is empty
	if e := tab.edits[nkey]; e.val.Null || e.val.S != "" || e.val == e.orig {
		t.Errorf("an empty text: %+v", e)
	}
	feed(t, a, ":q!<CR>h")
	delete(tab.edits, nkey)
	if a.value != nil {
		t.Fatal("not closed")
	}
	tab.edits[key] = edit{val: db.Val{S: "mine"}, orig: tab.page.Rows[0][4]} // a change of the cell is what it opens with
	feed(t, a, "e")
	if a.value.text() != "mine" || a.value.json {
		t.Errorf("a changed cell opens with the change: %q", a.value.text())
	}
	feed(t, a, "x:w<CR>")
	feed(t, a, "u:w<CR>") // taken back: the change it had before
	if e := tab.edits[key]; e.val.S != "mine" {
		t.Errorf("back to the change before: %+v", e)
	}
}

// A NULL cell opens empty; text in a column that is none that holds JSON opens
// laid out, and gq lays out what is typed, or toasts and leaves what is not
// JSON (F12.2).
func TestValueNullAndFormat(t *testing.T) {
	a := wide(160, 45)
	tab := loadOrders(t, a, 3)
	feed(t, a, "5l2je") // note of the third row: <null>
	if a.value == nil || a.value.text() != "" || a.value.json {
		t.Fatalf("a NULL: %+v", a.value)
	}
	feed(t, a, "gqq")
	if a.toast != "不是 JSON" || a.value.json {
		t.Errorf("gq on nothing: toast %q", a.toast)
	}
	feed(t, a, `i{"a":1,"b":[2]}<Esc>gqq`)
	if want := "{\n  \"a\": 1,\n  \"b\": [\n    2\n  ]\n}"; a.value.text() != want || !a.value.json {
		t.Errorf("gq: %q json %v", a.value.text(), a.value.json)
	}
	feed(t, a, "ggdGinot json<Esc>gqq")
	if a.value.text() != "not json" || a.toast != "不是 JSON" {
		t.Errorf("gq on text: %q, toast %q", a.value.text(), a.toast)
	}
	feed(t, a, ":q!<CR>")
	tab.page.Rows[1][5] = db.Val{S: `{"a":1}`}
	feed(t, a, "gg0j5le")
	if a.value == nil || !a.value.json || a.value.text() != "{\n  \"a\": 1\n}" {
		t.Errorf("JSON in a text column: %+v", a.value)
	}
}

// A cell that takes no change, a result's and a read-only connection's, opens
// to be read: a change is toasted and not made, y copies, esc and q close
// (F12.2).
func TestValueReadOnly(t *testing.T) {
	a, tab := valueTab(t)
	feed(t, a, ":q!<CR>")
	tab.conn.readOnly = true
	feed(t, a, "e")
	if a.value == nil || !a.value.readOnly {
		t.Fatal("a read-only connection's cell does not open read only")
	}
	for _, ks := range []string{"x", "dd", "ihello", "p", "o", "J", "gqq", "~"} {
		feed(t, a, "e")
		a.toast = ""
		feed(t, a, ks)
		if a.toast != "只读" || a.value.text() != a.value.opened || a.value.ed.Mode() != editor.Normal {
			t.Errorf("%s: toast %q, text %q, mode %v", ks, a.toast, a.value.text(), a.value.ed.Mode())
		}
		if feed(t, a, "<Esc>"); a.value != nil || a.confirm != nil {
			t.Fatalf("%s: esc closes a read-only value without a question: %v %v", ks, a.value != nil, a.confirm != nil)
		}
	}
	feed(t, a, "e")
	a.toast = ""              // the last key above left its own
	cmd := keys(t, a, `"+yy`) // yy alone goes to the register, not the clipboard (§11)
	if text, ok := clipText(cmd); a.toast != "" || !ok || !strings.HasPrefix(text, "{") {
		t.Errorf("yy: %q %v, toast %q", text, ok, a.toast)
	}
	feed(t, a, ":w<CR>")
	if len(tab.edits) != 0 {
		t.Errorf("a write: %v", tab.edits)
	}
	feed(t, a, "q")
	if a.value != nil {
		t.Error("q does not close a read-only value")
	}
	tab.conn.readOnly = false

	r := db.Result{Cols: []db.Col{{Name: "doc", Type: "jsonb"}}, Rows: [][]db.Val{{{S: `{"k": [1, 2]}`}}}}
	ra, _ := inRun(t, &execDB{res: map[string]db.Result{"select 1": r}}, "select 1;")
	press(t, ra, "<CR>")
	ra.win().focus(ra.win().Result.ID)
	feed(t, ra, "e")
	if ra.value == nil || !ra.value.readOnly || ra.value.title != "doc · jsonb" || !strings.Contains(ra.value.text(), "\"k\": [") {
		t.Fatalf("a result's value: %+v", ra.value)
	}
	feed(t, ra, "x")
	if ra.toast != "只读" || !strings.Contains(ra.value.text(), "\"k\"") {
		t.Errorf("x: toast %q", ra.toast)
	}
	feed(t, ra, "<Esc>")
	if ra.value != nil {
		t.Error("esc does not close it")
	}
}

// What is of the app and not the editor's stays out of the box: the leader,
// the palette and the pane keys do nothing in it, and C-c is esc (F12.2).
func TestValueTakesTheKeys(t *testing.T) {
	a, _ := valueTab(t)
	focus := a.win().Focus
	feed(t, a, "<Space>b<C-p><C-l><C-j>")
	if a.value == nil || a.palette != nil || a.win().Focus != focus || !a.win().TreeOpen {
		t.Errorf("box %v palette %v focus %d tree %v", a.value != nil, a.palette != nil, a.win().Focus, a.win().TreeOpen)
	}
	feed(t, a, "ix<C-c>")
	if a.value == nil || a.value.ed.Mode() != editor.Normal {
		t.Errorf("C-c in INSERT: %v", a.value != nil)
	}
	feed(t, a, "u<C-c>")
	if a.value != nil {
		t.Errorf("C-c in NORMAL with nothing changed closes it")
	}
}

// With vim = false the box closes with esc and C-c, and a read-only value with
// q too: the editor never leaves INSERT there (F12.2).
func TestValuePlainCloses(t *testing.T) {
	a := sizedWith(160, 45, userConfig(t, "vim = false", ""))
	tab := loadOrders(t, a, 3)
	feed(t, a, "4le")
	if a.value == nil {
		t.Fatal("not opened")
	}
	feed(t, a, "<Esc>")
	if a.value != nil {
		t.Errorf("esc")
	}
	feed(t, a, "e<C-c>")
	if a.value != nil {
		t.Errorf("C-c")
	}
	tab.conn.readOnly = true
	feed(t, a, "ex")
	if a.value == nil || a.toast != "只读" || a.value.text() != a.value.opened {
		t.Errorf("a key in a read-only plain value: toast %q", a.toast)
	}
	feed(t, a, "<Esc>")
	if a.value != nil {
		t.Errorf("esc on a read-only one")
	}
}

// A text that ends in a newline keeps it through an edit; a write of what is
// no value of the column is refused and :wq, C-s leave the box as it is; ? is
// the editor's search (F12.2).
func TestValueWriteDetails(t *testing.T) {
	a := wide(160, 45)
	tab := loadOrders(t, a, 3)
	tab.page.Rows[0][5] = db.Val{S: "abc\n"}
	feed(t, a, "5le")
	if a.value.text() != "abc" {
		t.Fatalf("text %q", a.value.text())
	}
	feed(t, a, "x:wq<CR>")
	if e := tab.edits[editKey{`["1"]`, "note"}]; a.value != nil || e.val.S != "bc\n" {
		t.Errorf("the newline: %+v", tab.edits)
	}
	feed(t, a, "4le")
	feed(t, a, "ggdG<C-s>") // an empty jsonb
	if a.value == nil || a.toast == "" {
		t.Errorf("C-s of no JSON: box %v, toast %q", a.value != nil, a.toast)
	}
	feed(t, a, ":q!<CR>04le?tags<CR>")
	if a.value.ed.Cursor().Line != 2 {
		t.Errorf("? is the search: at %+v", a.value.ed.Cursor())
	}
}

// An empty text is empty even where it ended in a newline, which the editor
// does not keep (F12.2).
func TestValueEmptiedKeepsNoNewline(t *testing.T) {
	a := wide(160, 45)
	tab := loadOrders(t, a, 3)
	tab.page.Rows[0][5] = db.Val{S: "abc\n"}
	feed(t, a, "5le")
	feed(t, a, "ggdG:wq<CR>")
	if e := tab.edits[editKey{`["1"]`, "note"}]; e.val.Null || e.val.S != "" {
		t.Errorf("emptied: %+v", e)
	}
}

// What takes no change is not changed by :w and C-s: a column of a type that
// cannot be set, a row marked for deletion, a result's value, which has no
// table to write to (F12.2).
func TestValueWriteRefused(t *testing.T) {
	a := wide(160, 45)
	tab := loadOrders(t, a, 3)
	for i := range tab.cols.Cols {
		if tab.cols.Cols[i].Name == "note" {
			tab.cols.Cols[i].Fixed = true
		}
	}
	feed(t, a, "5le")
	if !a.value.readOnly {
		t.Fatal("a fixed column opens to be changed")
	}
	feed(t, a, "ggx:w<CR>")
	if len(tab.edits) != 0 || a.toast != "只读" {
		t.Errorf("a fixed column: %v, toast %q", tab.edits, a.toast)
	}
	feed(t, a, ":q<CR>")
	for i := range tab.cols.Cols {
		tab.cols.Cols[i].Fixed = false
	}
	feed(t, a, "dde")
	if a.value == nil || !a.value.readOnly {
		t.Fatal("a row marked for deletion opens to be changed")
	}
	feed(t, a, "ggx:w<CR>")
	if len(tab.edits) != 0 {
		t.Errorf("a deleted row: %v", tab.edits)
	}

	r := db.Result{Cols: []db.Col{{Name: "doc", Type: "jsonb"}}, Rows: [][]db.Val{{{S: `{"k": 1}`}}}}
	ra, _ := inRun(t, &execDB{res: map[string]db.Result{"select 1": r}}, "select 1;")
	press(t, ra, "<CR>")
	ra.win().focus(ra.win().Result.ID)
	feed(t, ra, "e:w<CR>")
	if ra.value == nil || ra.toast != "只读" {
		t.Errorf(":w in a result's value: box %v, toast %q", ra.value != nil, ra.toast)
	}
	ra.toast = ""
	feed(t, ra, "<C-s>")
	if ra.value == nil || ra.toast != "只读" {
		t.Errorf("C-s in a result's value: box %v, toast %q", ra.value != nil, ra.toast)
	}

	empty := db.Result{Cols: []db.Col{{Name: "doc", Type: "jsonb"}}}
	ea, _ := inRun(t, &execDB{res: map[string]db.Result{"select 1": empty}}, "select 1;")
	press(t, ea, "<CR>")
	ea.win().focus(ea.win().Result.ID)
	if feed(t, ea, "e"); ea.value != nil {
		t.Error("a value of no row")
	}
}

// A row added takes the value as a change of its cell, and what is written
// back as it began is no change, so the cell is DEFAULT again (F12.2).
func TestValueOnAddedRow(t *testing.T) {
	a := wide(160, 45)
	tab := loadOrders(t, a, 3)
	feed(t, a, "o5le")
	if a.value == nil || a.value.cell.add == nil {
		t.Fatal("not on the row added")
	}
	feed(t, a, "ix<Esc>:w<CR>")
	add := a.value.cell.add
	if e, ok := add.cells["note"]; !ok || e.val.S != "x" {
		t.Fatalf("written: %+v", add.cells)
	}
	feed(t, a, "x:w<CR>") // empty again, as it opened
	if _, ok := add.cells["note"]; ok {
		t.Errorf("not taken back: %+v", add.cells)
	}
	if len(tab.edits) != 0 {
		t.Errorf("the table's: %v", tab.edits)
	}
}

// A paste goes to the value's editor, not to the cell under the box (F12.2).
func TestValuePaste(t *testing.T) {
	a, tab := valueTab(t)
	a.Update(tea.PasteMsg{Content: "zz"})
	if tab.typing != "" || tab.cell != nil || !strings.Contains(a.value.text(), "zz") {
		t.Errorf("typing %q, cell %v, text %q", tab.typing, tab.cell, a.value.text())
	}
}

// The mouse is the editor's, as a query's: a click puts the cursor there, a
// drag selects, the wheel scrolls the value and not the table under the box
// (F12.2).
func TestValueMouse(t *testing.T) {
	a := wide(160, 45)
	tab := loadOrders(t, a, 60) // long enough for the wheel to scroll it
	feed(t, a, "4le")
	a.View()
	at := func(line, col int) uv.Position {
		ed := a.value.ed
		text := ui.Query{Lines: ed.Lines()}.TextArea(a.valueText())
		return uv.Pos(text.Min.X+col, text.Min.Y+line)
	}
	p := at(3, 5) // the first "a" of tags
	click(a, p)
	if got := a.value.ed.Cursor(); got != (editor.Pos{Line: 3, Col: 5}) || a.dragText != 0 {
		t.Errorf("click: at %v, dragging %d", got, a.dragText)
	}
	a.View()
	_, _ = a.Update(tea.MouseClickMsg{X: p.X, Y: p.Y, Button: tea.MouseLeft})
	q := at(5, 2)
	a.Update(tea.MouseMotionMsg{X: q.X, Y: q.Y, Button: tea.MouseLeft})
	if got := a.value.ed.Cursor(); got != (editor.Pos{Line: 5, Col: 2}) || a.value.ed.Mode() != editor.Visual {
		t.Errorf("drag: at %v, mode %v", got, a.value.ed.Mode())
	}
	a.Update(tea.MouseReleaseMsg{X: q.X, Y: q.Y, Button: tea.MouseLeft})
	if a.dragText != 0 {
		t.Error("the drag goes on after the button is up")
	}

	long := make([]string, 100)
	for i := range long {
		long[i] = "line " + strconv.Itoa(i)
	}
	a.value.ed.Load(strings.Join(long, "\n"))
	a.valueSize()
	top := tab.top
	a.Update(tea.MouseWheelMsg{X: 80, Y: 20, Button: tea.MouseWheelDown})
	if a.value.ed.Top() != wheelStep || tab.top != top {
		t.Errorf("wheel down: value at %d, table at %d, was %d", a.value.ed.Top(), tab.top, top)
	}
	a.Update(tea.MouseWheelMsg{X: 80, Y: 20, Button: tea.MouseWheelUp})
	if a.value.ed.Top() != 0 {
		t.Errorf("wheel up: value at %d", a.value.ed.Top())
	}
}

// Without modes a selection ends with esc or C-c before the box closes
// (F12.2, §6.9).
func TestValuePlainSelectionFirst(t *testing.T) {
	for _, k := range []string{"<Esc>", "<C-c>"} {
		a := sizedWith(160, 45, userConfig(t, "vim = false", ""))
		loadOrders(t, a, 3)
		feed(t, a, "4le<S-Right><S-Right>")
		if !a.value.ed.HasSelection() {
			t.Fatal("no selection")
		}
		if feed(t, a, k); a.value == nil || a.value.ed.HasSelection() {
			t.Errorf("%s with a selection: box %v", k, a.value != nil)
		}
		if feed(t, a, k); a.value != nil {
			t.Errorf("%s again does not close", k)
		}
	}
}

// The box is under the cell, from its left edge and the row after it, half the
// pane wide; with fewer than 8 rows below and more above it is over the cell,
// its bottom border on the row above; a pane of 20 columns or less is all of it
// (F12.8).
func TestValueBoxPlacement(t *testing.T) {
	cellOf := func(a *App) (cell, pane uv.Rectangle) {
		p := a.focused()
		_, g, area, _ := a.gridOf(p)
		return g.CellRect(area), bodyRect(a.layout()[p.ID])
	}
	a, _ := valueTab(t)
	cell, pane := cellOf(a)
	box := a.valueBox()
	if box.Min.X != cell.Min.X || box.Min.Y != cell.Max.Y || box.Max.Y != pane.Max.Y || box.Dx() != pane.Dx()/2 {
		t.Errorf("room below: box %v, cell %v, pane %v", box, cell, pane)
	}
	b := wide(160, 22)
	loadOrders(t, b, 60)
	feed(t, b, "9j4le")
	cell, pane = cellOf(b)
	box = b.valueBox()
	if cell.Max.Y+valueMinRows <= pane.Max.Y || box.Max.Y != cell.Min.Y || box.Min.Y != pane.Min.Y {
		t.Errorf("no room below: box %v, cell %v, pane %v", box, cell, pane)
	}
	golden.RequireEqual(t, b.render().String())
	// the right edge of the pane: moved left to stay in it
	feed(t, a, "<Esc>")
	feed(t, a, "$e")
	cell, pane = cellOf(a)
	if box = a.valueBox(); box.Max.X > pane.Max.X || box.Min.X > cell.Min.X {
		t.Errorf("the last column: box %v, cell %v, pane %v", box, cell, pane)
	}
}

// The keys under the text are the keymap's, the last ones given up when the
// box is narrow, and a value that is read only has two (F12.8).
func TestValueHints(t *testing.T) {
	a, tab := valueTab(t)
	if s := a.render().String(); !strings.Contains(s, "esc 关闭 · C-s 写回并关闭 · :w 写回 · gqq 格式化 JSON") {
		t.Errorf("hints:\n%s", s)
	}
	feed(t, a, "<Esc>")
	tab.conn.readOnly = true
	feed(t, a, "e")
	if s := a.render().String(); !strings.Contains(s, "esc 关闭 · y 复制") || strings.Contains(s, "gqq") {
		t.Errorf("read only:\n%s", s)
	}
	n := wide(60, 30) // a box of 40: the last of them does not fit
	loadOrders(t, n, 3)
	feed(t, n, "4le")
	if s := n.render().String(); !strings.Contains(s, "esc 关闭 ·") || strings.Contains(s, "gqq") {
		t.Errorf("narrow:\n%s", s)
	}
}
