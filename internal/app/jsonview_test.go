package app

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/exp/golden"

	"github.com/kackerx/sqlmux/internal/db"
	"github.com/kackerx/sqlmux/internal/editor"
)

// expectedJSON is the page of tab as the records Y yanks, in an array.
func expectedJSON(t *testing.T, tab *Tab, rows int) string {
	t.Helper()
	var objects []string
	for row := range rows {
		fs, ok := jsonFields(tab, row)
		if !ok {
			t.Fatalf("no record %d", row)
		}
		raw, err := rowRaw(fs)
		if err != nil {
			t.Fatal(err)
		}
		objects = append(objects, string(raw))
	}
	text, err := indentJSON([]byte("[" + strings.Join(objects, ",") + "]"))
	if err != nil {
		t.Fatal(err)
	}
	return text
}

// gJ puts the page of the table as a JSON array in its place: each record as Y
// has it, a chip JSON on the query bar, a vim over the text that takes no
// change; esc brings the table back; the text follows the page; it is the tab's
// and no layout's (F14.5).
func TestJSONView(t *testing.T) {
	a := sized(160, 45, "nerd")
	tab := loadOrders(t, a, 3)
	p := a.focused()
	feed(t, a, "gJ")
	jv := jsonViewOf(p)
	if jv == nil {
		t.Fatal("gJ opens no JSON view")
	}
	a.jsonSync(p)
	want := expectedJSON(t, p.tab(), 3)
	if got := strings.Join(jv.ed.Lines(), "\n"); got != want || !strings.HasPrefix(want, "[\n  {\n    \"id\": ") {
		t.Fatalf("the text:\n%s\nwant\n%s", got, want)
	}
	view := a.render().String()
	if !strings.Contains(view, `"status"`) || !strings.Contains(view, "JSON") || strings.Contains(view, "│ amount │") {
		t.Errorf("the screen has no JSON in the table's place:\n%s", view)
	}
	feed(t, a, "jj")
	if jv.ed.Cursor().Line != 2 {
		t.Errorf("j is the vim's: %v", jv.ed.Cursor())
	}
	if feed(t, a, "ix"); strings.Join(jv.ed.Lines(), "\n") != want || a.toast != "只读" {
		t.Errorf("a read only text: toast %q", a.toast)
	}
	a.toast = ""
	if jv.ed.Mode() != editor.Normal {
		t.Fatalf("a read only editor does not go to INSERT: %v", jv.ed.Mode())
	}
	feed(t, a, "<Esc>")
	if jsonViewOf(p) != nil || !strings.Contains(a.render().String(), "│ amount │") {
		t.Error("esc brings the table back")
	}
	// the page changes under it: the text follows
	feed(t, a, "gJ")
	jv = jsonViewOf(p)
	a.jsonSync(p)
	_, cols, page := ordersTable(2)
	a.Update(pageMsg{tab: tab, seq: tab.seq, cols: cols, page: page, next: true})
	a.render()
	if got := strings.Join(jv.ed.Lines(), "\n"); got != expectedJSON(t, p.tab(), 2) || strings.Count(got, `"id"`) != 2 {
		t.Errorf("a new page:\n%s", got)
	}
	// turned and JSON are one view at a time, the other way as well
	feed(t, a, "T")
	if jsonViewOf(p) != nil || !tab.transpose {
		t.Errorf("T from the JSON view: jv %v, transpose %v", jsonViewOf(p), tab.transpose)
	}
	feed(t, a, "gJ")
	if jsonViewOf(p) == nil || tab.transpose {
		t.Errorf("gJ from the turned table: jv %v, transpose %v", jsonViewOf(p), tab.transpose)
	}
	// q, as the value editor's, is esc too; a pending command is the editor's first
	feed(t, a, "g")
	if feed(t, a, "<Esc>"); jsonViewOf(p) == nil {
		t.Error("esc after a g is the g's")
	}
	feed(t, a, "q")
	if jsonViewOf(p) != nil {
		t.Error("q brings the table back")
	}
}

// A result has its JSON view too, its page, no more: a result is read only (F14.5).
func TestJSONViewOfResult(t *testing.T) {
	d := &execDB{res: map[string]db.Result{"select 1": rows(3)}}
	a, _ := inRun(t, d, "select 1")
	press(t, a, "<CR>")
	res := a.win().Result
	a.win().focus(res.ID)
	if resultOf(res) == nil || resultOf(res).run == nil {
		t.Fatalf("no result tab: %v", tabNames(res))
	}
	feed(t, a, "gJ")
	jv := jsonViewOf(res)
	if jv == nil {
		t.Fatal("no JSON view on a result")
	}
	a.jsonSync(res)
	if got, want := strings.Join(jv.ed.Lines(), "\n"), expectedJSON(t, res.tab(), 3); got != want {
		t.Errorf("the text:\n%s\nwant\n%s", got, want)
	}
	if view := a.render().String(); !strings.Contains(view, `"n"`) {
		t.Errorf("not in the screen:\n%s", view)
	}
	feed(t, a, "<Esc>")
	if jsonViewOf(res) != nil {
		t.Error("esc brings the result's table back")
	}
}

// The text of the table has what is changed and not yet saved, as Y has it; a
// change made while the view is open shows (F14.5).
func TestJSONViewHasChanges(t *testing.T) {
	a := sized(160, 45, "nerd")
	tab := loadOrders(t, a, 3)
	p := a.focused()
	feed(t, a, "gJ")
	jv := a.jsonSync(p)
	before := strings.Join(jv.ed.Lines(), "\n")
	tab.edits = map[editKey]edit{{tab.rowKey(0), "note"}: {val: db.Val{S: "changed here"}, orig: tab.page.Rows[0][noteField(tab)]}}
	a.render()
	if got := strings.Join(jv.ed.Lines(), "\n"); got == before || !strings.Contains(got, "changed here") {
		t.Errorf("a change shows: %q", got)
	}
}

func noteField(t *dataTab) int {
	for i, c := range t.page.Cols {
		if c.Name == "note" {
			return i
		}
	}
	return 0
}

// ] and [ and R are the table's, or the result's, in the JSON view; the cursor's
// keys are the vim's (F14.5).
func TestJSONViewKeys(t *testing.T) {
	a := sized(160, 45, "nerd")
	loadOrders(t, a, 3)
	p := a.focused()
	feed(t, a, "gJ")
	for key, want := range map[string]string{"]": "json.page.next", "[": "json.page.prev", "R": "json.refresh", "T": "grid.transpose", "gJ": "grid.view.json"} {
		if got := a.keys.Hint(want, "jsonview"); got != key {
			t.Errorf("%s is bound to %q", want, got)
		}
	}
	if a.mode() != 0 || a.vim() != jsonViewOf(p).ed {
		t.Errorf("the mode and the editor that has the keys: %v", a.mode())
	}
	if jv := jsonViewOf(p); jv.ed.Mode() != editor.Normal {
		t.Errorf("mode %v", jv.ed.Mode())
	}
}

func TestGoldenJSONView160x45(t *testing.T) {
	a := wide(160, 45)
	loadOrders(t, a, 3)
	feed(t, a, "gJ")
	golden.RequireEqual(t, a.render().String())
}
