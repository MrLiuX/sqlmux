package app

import (
	"testing"

	tea "charm.land/bubbletea/v2"
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
	b.ResetTimer()
	for i := 0; b.Loop(); i++ {
		y := top + i%(2*(bottom-top))
		if y > bottom {
			y = 2*bottom - top - y // and back up
		}
		a.Update(tea.MouseMotionMsg{X: x, Y: y})
		a.View()
	}
}
