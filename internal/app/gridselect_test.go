package app

import (
	"slices"
	"strings"
	"testing"

	"github.com/charmbracelet/x/exp/golden"

	"github.com/kackerx/sqlmux/internal/db"
	"github.com/kackerx/sqlmux/internal/keymap"
)

func markedOf(s *gridState) []int {
	return slices.Sorted(func(yield func(int) bool) {
		for r := range s.marked {
			if !yield(r) {
				return
			}
		}
	})
}

// m marks the record under the cursor, again unmarks it, and the record
// numbers say so; d marks the records for deletion as dd does and nothing is
// selected after; when they all were marked already d takes the marks off
// (F10.7).
func TestSelectMarkAndDelete(t *testing.T) {
	a := wide(160, 45)
	tab := loadOrders(t, a, 5)
	feed(t, a, "mjjm")
	if got := markedOf(&tab.gridState); !slices.Equal(got, []int{0, 2}) || a.context().Focus[0] != "select" || a.mode() != keymap.Normal {
		t.Fatalf("marked %v, scopes %v, mode %v", got, a.context().Focus, a.mode())
	}
	if !strings.Contains(a.queryBar(a.focused(), tab).Right, " · 2 selected") && !strings.Contains(a.queryBar(a.focused(), tab).Right, " · 已选 2") {
		t.Errorf("the count: %q", a.queryBar(a.focused(), tab).Right)
	}
	feed(t, a, "mm") // unmarked, marked again
	if got := markedOf(&tab.gridState); !slices.Equal(got, []int{0, 2}) {
		t.Fatalf("m twice: %v", got)
	}
	feed(t, a, "d")
	if len(tab.deleted) != 2 || !tab.deleted[tab.rowKey(0)] || !tab.deleted[tab.rowKey(2)] || tab.selecting() || len(a.res.Pending()) != 0 {
		t.Fatalf("d: deleted %v, selecting %v, pending %v", tab.deleted, tab.selecting(), a.res.Pending())
	}
	feed(t, a, "kkmjjmd") // both again: the marks go
	if len(tab.deleted) != 0 || tab.selecting() {
		t.Errorf("d on records all marked: deleted %v", tab.deleted)
	}
	feed(t, a, "ddjmd") // one is marked: all of them are
	if len(tab.deleted) != 2 {
		t.Errorf("one marked, one not: %v", tab.deleted)
	}
}

// v begins VISUAL on the record under the cursor, moving widens it, esc ends
// it with the range marked; v again ends it too; with marks only, esc takes
// them back; a range and a mark are one selection (F10.7).
func TestSelectVisual(t *testing.T) {
	a := wide(160, 45)
	tab := loadOrders(t, a, 6)
	feed(t, a, "v")
	if a.mode() != keymap.Visual || !strings.Contains(statusRow(a), "VISUAL") {
		t.Fatalf("mode %v, status %q", a.mode(), statusRow(a))
	}
	feed(t, a, "2j")
	if got := slices.Sorted(func(yield func(int) bool) {
		for r := range tab.selected() {
			if !yield(r) {
				return
			}
		}
	}); !slices.Equal(got, []int{0, 1, 2}) {
		t.Fatalf("v 2j: %v", got)
	}
	feed(t, a, "<Esc>")
	if got := markedOf(&tab.gridState); !slices.Equal(got, []int{0, 1, 2}) || a.mode() != keymap.Normal || tab.visual {
		t.Fatalf("esc: marked %v, mode %v", got, a.mode())
	}
	feed(t, a, "jjmv")
	if feed(t, a, "jv"); !slices.Equal(markedOf(&tab.gridState), []int{0, 1, 2, 4, 5}) || tab.visual {
		t.Fatalf("a range ended by v joins the marks: %v", markedOf(&tab.gridState))
	}
	feed(t, a, "<Esc>")
	if tab.selecting() || a.context().Focus[0] != "grid" {
		t.Errorf("esc with marks only clears them: %v", markedOf(&tab.gridState))
	}
}

// y copies the records selected as a JSON array, in the order they show, each
// as Y writes it, the numbers flash, nothing is selected after; d and y do
// not wait for the dd and yy of the grid (F10.7).
func TestSelectYank(t *testing.T) {
	a := wide(160, 45)
	tab := loadOrders(t, a, 4)
	feed(t, a, "vj")
	cmd := keys(t, a, "y")
	text, ok := clipText(cmd)
	if !ok || !strings.HasPrefix(text, "[\n  {\n    \"id\": 1,") || strings.Count(text, "\"id\":") != 2 || !strings.Contains(text, "\"id\": 2,") || !strings.HasSuffix(text, "}\n]") {
		t.Fatalf("y: %q", text)
	}
	if tab.selecting() || a.flash == nil || !a.flash.nums[0] || !a.flash.nums[1] || a.flash.nums[2] || len(a.res.Pending()) != 0 {
		t.Errorf("after: selecting %v, flash %+v, pending %v", tab.selecting(), a.flash, a.res.Pending())
	}
	// out of a selection d and y are the grid's: they wait for the next key
	feed(t, a, "d")
	if len(a.res.Pending()) != 1 {
		t.Errorf("d with nothing selected waits: %v", a.res.Pending())
	}
	if feed(t, a, "d"); len(tab.deleted) != 1 {
		t.Errorf("dd as ever: %v", tab.deleted)
	}
}

// A transposed table's records are its columns: v and l select two, and the
// selection stays through the transpose; a new page, a row added and a row
// taken out put it back (F10.7).
func TestSelectTransposedAndCleared(t *testing.T) {
	a := wide(160, 45)
	tab := loadOrders(t, a, 5)
	feed(t, a, "Tvl")
	if got := len(tab.selected()); got != 2 {
		t.Fatalf("T v l: %d records", got)
	}
	feed(t, a, "<Esc>T")
	if len(tab.marked) != 2 || tab.transpose {
		t.Errorf("the marks stay through T: %v", tab.marked)
	}
	feed(t, a, "o")
	if tab.selecting() {
		t.Error("a row added puts the marks back")
	}
	feed(t, a, "mjm")
	answer(a, tab)
	if tab.selecting() {
		t.Error("a new page puts the marks back")
	}
	feed(t, a, "o<Esc>") // a row added, the cursor in it
	added := len(tab.added)
	tab.marked = map[int]bool{0: true}
	a.run("grid.row.delete", 0) // the − button takes it out: the rows after it move
	if tab.selecting() || len(tab.added) != added-1 {
		t.Errorf("a row taken out puts the marks back: %v, %d added of %d", tab.marked, len(tab.added), added)
	}
}

// A result's records are selected and copied as a table's are; d has nothing
// to delete there and leaves the selection (F10.7).
func TestSelectInResult(t *testing.T) {
	r := db.Result{Cols: []db.Col{{Name: "n", Type: "int4"}}, Rows: [][]db.Val{{{S: "1"}}, {{S: "2"}}, {{S: "3"}}}}
	a, _ := inRun(t, &execDB{res: map[string]db.Result{"select 1": r}}, "select 1;")
	press(t, a, "<CR>")
	a.win().focus(a.win().Result.ID)
	feed(t, a, "mjm")
	rt := resultOf(a.focused())
	if got := markedOf(&rt.gridState); !slices.Equal(got, []int{0, 1}) || a.context().Focus[0] != "select" || a.context().Focus[1] != "result" {
		t.Fatalf("marked %v, scopes %v", got, a.context().Focus)
	}
	if feed(t, a, "d"); !rt.selecting() {
		t.Error("d has no table to delete from, the selection stays")
	}
	text, _ := clipText(keys(t, a, "y"))
	if text != "[\n  {\n    \"n\": 1\n  },\n  {\n    \"n\": 2\n  }\n]" || rt.selecting() {
		t.Errorf("y: %q", text)
	}
}

// The numbers of the records selected, marked or in the range of VISUAL, are
// on the visual background, in a table turned or not, and the others are not
// (F10.7).
func TestSelectHighlight(t *testing.T) {
	for name, c := range map[string]struct {
		keys string
		want []string
	}{
		"vertical":   {"mjjmjv", []string{"1", "3", "4"}},
		"transposed": {"Tmlmlv", []string{"1", "2", "3"}},
	} {
		t.Run(name, func(t *testing.T) {
			a := wide(160, 45)
			loadOrders(t, a, 6)
			feed(t, a, c.keys)
			f := a.render()
			_, _, area, _ := a.gridOf(a.focused())
			var got []string
			for y := area.Min.Y; y < area.Max.Y; y++ {
				for x := area.Min.X; x < area.Min.X+3 || name == "transposed" && x < area.Max.X; x++ {
					cell := f.Buf.CellAt(x, y)
					// a number, alone in its cell, in the cells a record's number takes
					if n := strings.TrimSpace(cell.Content); n >= "1" && n <= "6" && len(n) == 1 && cell.Style.Bg == a.theme.Visual && (name == "vertical" && y > area.Min.Y+1 || name == "transposed" && y == area.Min.Y) {
						got = append(got, n)
					}
				}
			}
			if !slices.Equal(got, c.want) {
				t.Errorf("on the visual background: %v, want %v", got, c.want)
			}
		})
	}
}

// The numbers of the records selected, vertical and transposed, a range among
// them (F10.7).
func TestGoldenSelect(t *testing.T) {
	for name, keys := range map[string]string{"vertical": "mjjmjv", "transposed": "Tmlmlv"} {
		t.Run(name, func(t *testing.T) {
			a := wide(160, 45)
			loadOrders(t, a, 6)
			feed(t, a, keys)
			golden.RequireEqual(t, a.render().String())
		})
	}
}

// The marks are positions among the rows shown: a save that takes the added
// rows out of them and a refresh that fetches others leave none (F10.7).
func TestSelectClearedAsRowsMove(t *testing.T) {
	a := wide(160, 45)
	tab := loadOrders(t, a, 3)
	a.sess.Conns[0].Main = db.NewWorker(&tagDB{})
	feed(t, a, "olli5<Esc>jm") // a row added under the first; the record under it marked
	if !tab.selecting() {
		t.Fatal("not marked")
	}
	_, cmd := a.Update(teaKey("<C-s>"))
	a.Update(cmd())
	if tab.selecting() {
		t.Errorf("saved, the marks stay: %v", markedOf(&tab.gridState))
	}
	feed(t, a, "m")
	a.run("grid.refresh", 0)
	if tab.selecting() {
		t.Errorf("refreshed, the marks stay: %v", markedOf(&tab.gridState))
	}
}
