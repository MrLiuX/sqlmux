package app

import (
	"fmt"
	"strconv"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/kackerx/sqlmux/internal/db"
	"github.com/kackerx/sqlmux/internal/keymap"
)

// BenchmarkPointerMotion is one pointer-motion event in 1003 mode, the
// terminal reporting every cell the pointer crosses: an Update and a View
// each, over a t_order of 100 rows in the default layout at 160×45, the
// pointer sweeping down the grid and back (§15「性能」, F8.20).
func BenchmarkPointerMotion(b *testing.B) {
	a := wide(160, 45)
	loadOrders(b, a, 100)
	a.View()
	const x, top, bottom = 90, 4, 38
	b.ReportAllocs()
	for i := 0; b.Loop(); i++ {
		y := top + i%(2*(bottom-top))
		if y > bottom {
			y = 2*bottom - top - y // and back up
		}
		a.Update(tea.MouseMotionMsg{X: x, Y: y})
		a.View()
	}
}

// BenchmarkPointerWithinRow is the event of a pointer moving along a row, the
// terminal reporting each cell it crosses: the frame stays as it is (F8.20).
func BenchmarkPointerWithinRow(b *testing.B) {
	a := wide(160, 45)
	loadOrders(b, a, 100)
	a.View()
	b.ReportAllocs()
	for i := 0; b.Loop(); i++ {
		a.Update(tea.MouseMotionMsg{X: 40 + i%2, Y: 10})
		a.View()
	}
}

// heavyResult is n rows of 15 columns, half of the values Chinese and
// 100-character JSON, as mt_task shows (F11.4).
func heavyResult(n int) db.Result {
	r := db.Result{}
	for c := range 15 {
		r.Cols = append(r.Cols, db.Col{Name: fmt.Sprintf("col_%d", c), Type: "text"})
	}
	for i := range n {
		row := make([]db.Val, 15)
		for c := range row {
			switch {
			case c%2 == 0:
				row[c] = db.Val{S: strconv.Itoa(i*15 + c)}
			case c%4 == 1:
				row[c] = db.Val{S: fmt.Sprintf("任务 %d：同步用户资料并重新计算统计结果", i)}
			default:
				row[c] = db.Val{S: fmt.Sprintf(`{"id": %d, "name": "用户", "tags": ["a", "b"], "note": %q}`, i, strings.Repeat("x", 60))}
			}
		}
		r.Rows = append(r.Rows, row)
	}
	return r
}

// scrollResult is a result of 500 heavy rows in the result area of the
// default layout at 160×45.
func scrollResult(b testing.TB) *App {
	b.Helper()
	a, _ := inRun(b, &execDB{res: map[string]db.Result{"select 1": heavyResult(500)}}, "select 1;")
	press(b, a, "<CR>")
	a.win().focus(a.win().Result.ID)
	if r := resultOf(a.focused()); r == nil || len(r.page.Rows) != 500 {
		b.Fatalf("the result area has %+v", r)
	}
	a.View()
	return a
}

// scrollTable is a table tab of 100 heavy rows in the data pane.
func scrollTable(b testing.TB) (*App, *dataTab) {
	b.Helper()
	a := wide(160, 45)
	tab := loadOrders(b, a, 1)
	tab.page = heavyResult(100)
	tab.row, tab.col = 0, 0
	a.View()
	return a, tab
}

// BenchmarkResultScrollKeys is one j or k of a key held down in the result
// area: an Update and a View each, 200 down and 200 up (§15「性能」, F11.4).
func BenchmarkResultScrollKeys(b *testing.B) {
	a := scrollResult(b)
	b.ReportAllocs()
	for i := 0; b.Loop(); i++ {
		k := "j"
		if i/200%2 == 1 {
			k = "k"
		}
		a.Update(teaKey(keymap.Key(k)))
		a.View()
	}
}

// BenchmarkResultScrollWheel is one notch of the wheel over the result area.
func BenchmarkResultScrollWheel(b *testing.B) {
	a := scrollResult(b)
	b.ReportAllocs()
	for i := 0; b.Loop(); i++ {
		btn := tea.MouseWheelDown
		if i/100%2 == 1 {
			btn = tea.MouseWheelUp
		}
		a.Update(tea.MouseWheelMsg{X: 90, Y: 38, Button: btn})
		a.View()
	}
}

// BenchmarkTableScrollKeys is the same key held in a table pane of 100 heavy
// rows, for a comparison: its grid is made again every frame (F11.4).
func BenchmarkTableScrollKeys(b *testing.B) {
	a, _ := scrollTable(b)
	b.ReportAllocs()
	for i := 0; b.Loop(); i++ {
		k := "j"
		if i/50%2 == 1 {
			k = "k"
		}
		a.Update(teaKey(keymap.Key(k)))
		a.View()
	}
}
