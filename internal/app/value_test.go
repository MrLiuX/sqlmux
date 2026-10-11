package app

import (
	"image/color"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/exp/golden"

	"github.com/kackerx/sqlmux/internal/db"
	"github.com/kackerx/sqlmux/internal/editor"
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
	key := a.value.key
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
	key := a.value.key
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
	key := a.value.key
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
	cmd := keys(t, a, "yy")
	if text, ok := clipText(cmd); a.toast == "" && (!ok || !strings.HasPrefix(text, "{")) {
		t.Errorf("yy: %q %v", text, ok)
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
// no value of the column is refused and :wq, C-s leave the box as it is; the
// wheel is the box's, not the panes' under it; ? is the editor's search
// (F12.2).
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
	top := tab.top
	a.Update(tea.MouseWheelMsg{X: 80, Y: 20, Button: tea.MouseWheelDown})
	if tab.top != top {
		t.Errorf("the wheel moved the table under the box")
	}
	feed(t, a, ":q!<CR>04le?tags<CR>")
	if a.value.ed.Cursor().Line != 2 {
		t.Errorf("? is the search: at %+v", a.value.ed.Cursor())
	}
}
