package app

import (
	"strings"
	"testing"

	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/exp/golden"

	"github.com/kackerx/sqlmux/internal/ui"
)

// textCells are where s is in the part r of frame f, a position for each place
// it is, by the cell of its first character.
func textCells(f *ui.Frame, r uv.Rectangle, s string) (at [][2]int) {
	for y := r.Min.Y; y < r.Max.Y; y++ {
		row := ""
		for x := r.Min.X; x < r.Max.X; x++ {
			row += f.Buf.CellAt(x, y).Content
		}
		for from := 0; ; {
			i := strings.Index(row[from:], s)
			if i < 0 {
				break
			}
			at = append(at, [2]int{r.Min.X + len([]rune(row[:from+i])), y})
			from += i + len(s)
		}
	}
	return at
}

// A search in a query lights its matches, the one the cursor is in in the
// current match's color; the line under the text keeps the pattern with
// [1/2] at its right end until a key that is no n or N; n moves the current
// one; esc in NORMAL puts them out (F15.9).
func TestQuerySearchLit(t *testing.T) {
	a := sized(160, 45, "nerd")
	loadOrders(t, a, 3)
	c := queryOf(a.win().Root.B.Pane)
	a.focusPane(a.win().Root.B.Pane.ID)
	c.ed.Load("select 1;\nselect 2;\nfrom t where x = 1;")
	th := a.theme
	area := bodyRect(a.layout()[a.win().Root.B.Pane.ID])
	bgs := func() (first, second string) {
		f := a.render()
		cells := textCells(f, area, "select")
		if len(cells) < 2 { // the line under the text has the pattern too, last
			t.Fatalf("%d select in the screen", len(cells))
		}
		name := func(x, y int) string {
			switch f.Buf.CellAt(x, y).Style.Bg {
			case th.Search:
				return "search"
			case th.SearchCurrent:
				return "current"
			}
			return "none"
		}
		return name(cells[0][0], cells[0][1]), name(cells[1][0], cells[1][1])
	}
	if f, s := bgs(); f != "none" || s != "none" {
		t.Fatalf("lit before a search: %s %s", f, s)
	}
	feed(t, a, "/select<CR>")
	if f, s := bgs(); f != "search" || s != "current" {
		t.Errorf("after the search: %s %s", f, s)
	}
	row := func() string { // the query's last row, with the pane's border and gutter off
		for _, r := range strings.Split(a.render().String(), "\n") {
			if strings.Contains(r, "/select") {
				return r
			}
		}
		return ""
	}
	if r := row(); !strings.Contains(r, "/select") || !strings.Contains(r, "[2/2]") || strings.Index(r, "[2/2]") < strings.Index(r, "/select")+20 {
		t.Errorf("the line under the text: %q", r)
	}
	feed(t, a, "n")
	if f, s := bgs(); f != "current" || s != "search" || !strings.Contains(row(), "W [1/2]") {
		t.Errorf("after n: %s %s, %q", f, s, row())
	}
	feed(t, a, "$")
	if row() != "" {
		t.Errorf("the line stays after another key: %q", row())
	}
	if f, s := bgs(); f != "search" || s != "search" {
		t.Errorf("the cursor off the matches: %s %s", f, s)
	}
	feed(t, a, "<Esc>")
	if f, s := bgs(); f != "none" || s != "none" {
		t.Errorf("esc in NORMAL: %s %s", f, s)
	}
	feed(t, a, "n")
	if f, s := bgs(); f == "none" || s == "none" {
		t.Errorf("n lights them again: %s %s", f, s)
	}
}

// The selection of VISUAL is drawn over a match, and the text of a match is
// drawn in the theme's bg (F15.9).
func TestSearchLitUnderVisual(t *testing.T) {
	a := sized(160, 45, "nerd")
	c := queryOf(a.win().Root.B.Pane)
	a.focusPane(a.win().Root.B.Pane.ID)
	c.ed.Load("select 1;")
	feed(t, a, "/select<CR>")
	f := a.render()
	at := textCells(f, bodyRect(a.layout()[a.win().Root.B.Pane.ID]), "select")[0]
	if st := f.Buf.CellAt(at[0], at[1]).Style; st.Fg != a.theme.Bg {
		t.Errorf("the match's text: %v", st.Fg)
	}
	feed(t, a, "vl")
	f = a.render()
	if f.Buf.CellAt(at[0], at[1]).Style.Bg != a.theme.Visual {
		t.Error("the selection is over the match")
	}
	if f.Buf.CellAt(at[0]+3, at[1]).Style.Bg == a.theme.Visual {
		t.Error("the rest of the match is lit")
	}
}

// The value editor and the JSON view are the same editor: a search in them
// lights its matches and keeps the count line under their text (F15.9).
func TestValueAndJSONSearchLit(t *testing.T) {
	a, _ := valueTab(t)
	feed(t, a, "/tags<CR>")
	f := a.render()
	if at := textCells(f, a.valueBox(), "tags"); len(at) != 2 || f.Buf.CellAt(at[0][0], at[0][1]).Style.Bg != a.theme.SearchCurrent {
		t.Errorf("value editor: %v", at)
	}
	if r := a.render().String(); !strings.Contains(r, "/tags") || !strings.Contains(r, "[1/1]") {
		t.Errorf("no count line:\n%s", r)
	}
	feed(t, a, "<Esc><Esc>")
	b := sized(160, 45, "nerd")
	loadOrders(t, b, 3)
	feed(t, b, "gj/"+"\"n\""+"<CR>")
	f = b.render()
	cells := textCells(f, b.jsonArea(b.focused()), `"n"`)
	if len(cells) != 4 { // the three, and the pattern on the line under the text
		t.Fatalf("%d of \"n\" in the JSON view", len(cells))
	}
	lit := 0
	for _, c := range cells[:3] {
		if f.Buf.CellAt(c[0], c[1]).Style.Bg == b.theme.Search || f.Buf.CellAt(c[0], c[1]).Style.Bg == b.theme.SearchCurrent {
			lit++
		}
	}
	if lit != 3 || !strings.Contains(f.String(), "[1/3]") && !strings.Contains(f.String(), "[2/3]") {
		t.Errorf("JSON view: %d lit\n%s", lit, f.String())
	}
	golden.RequireEqual(t, f.String())
}

// With the matches of a search lit, the first esc of the value editor and the
// JSON view only puts them out, the second leaves; q leaves at once (F15.9).
func TestEscClearsTheSearchFirst(t *testing.T) {
	a, _ := valueTab(t)
	feed(t, a, "/tags<CR>")
	feed(t, a, "<Esc>")
	if a.value == nil || a.value.ed.Lit() {
		t.Fatalf("value editor, first esc: open %v", a.value != nil)
	}
	feed(t, a, "<Esc>")
	if a.value != nil {
		t.Error("value editor, second esc leaves")
	}
	b := sized(160, 45, "nerd")
	loadOrders(t, b, 3)
	p := b.focused()
	feed(t, b, "gj/status<CR><Esc>")
	if jv := jsonViewOf(p); jv == nil || jv.ed.Lit() {
		t.Fatalf("JSON view, first esc: view %v", jv != nil)
	}
	feed(t, b, "<Esc>")
	if jsonViewOf(p) != nil {
		t.Error("JSON view, second esc brings the table back")
	}
	feed(t, b, "gj/status<CR>q")
	if jsonViewOf(p) != nil {
		t.Error("q leaves with the matches lit")
	}
	feed(t, b, "gj/zzzz<CR><Esc>") // nothing found: nothing was lit, esc leaves at once
	if jsonViewOf(p) != nil {
		t.Error("esc after a search that found nothing leaves")
	}
}
