package app

import (
	"math/rand/v2"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/exp/golden"

	"github.com/kackerx/sqlmux/internal/ui"
)

// withTips is a test with the status bar's tips on, as the defaults have them.
func withTips(t *testing.T) {
	t.Helper()
	tipsInTests = true
	t.Cleanup(func() { tipsInTests = false })
}

// tipsApp is an App with tips on and the keys of keys, the tips picked by
// pick (the first of those left when it is nil).
func tipsApp(t *testing.T, conf, keys string) *App {
	t.Helper()
	withTips(t)
	a := sizedWith(200, 45, userConfig(t, conf, keys))
	a.tip.pick = func(int) int { return 0 }
	a.tip.cur, a.tip.seen = -1, nil
	return a
}

// Every tip is about an action there is, bound by default in the scope it names,
// and says where its key goes; one action has one tip (F15.4).
func TestTipsAreOfActions(t *testing.T) {
	a := sized(160, 45, "nerd")
	seen := map[string]bool{}
	for _, tp := range tips {
		if actions[tp.action].Run == nil {
			t.Errorf("%s: no such action", tp.action)
		}
		if a.keys.Hint(tp.action, tp.scope) == "" {
			t.Errorf("%s: no key in %s by default", tp.action, tp.scope)
		}
		if strings.Count(tp.text, "{key}") != 1 {
			t.Errorf("%s: %q", tp.action, tp.text)
		}
		if seen[tp.action] {
			t.Errorf("%s: twice", tp.action)
		}
		seen[tp.action] = true
	}
}

// Each tip once before one comes again; the next round starts with another
// than the last; a tip whose action has no key is skipped (F15.4).
func TestTipsRotate(t *testing.T) {
	a := tipsApp(t, "", "")
	n := len(tips)
	var order []int
	for range 2 * n {
		a.nextTip()
		order = append(order, a.tip.cur)
	}
	for round := range 2 {
		got := map[int]bool{}
		for _, i := range order[round*n : (round+1)*n] {
			got[i] = true
		}
		if len(got) != n {
			t.Fatalf("round %d: %d different of %d: %v", round, len(got), n, order)
		}
	}
	if order[n] == order[n-1] {
		t.Errorf("the tip of a round's end comes first of the next: %v", order)
	}
	// the random pick too
	a = tipsApp(t, "", "")
	a.tip.pick = rand.IntN
	seenNow := map[int]bool{}
	for range n {
		a.nextTip()
		if seenNow[a.tip.cur] {
			t.Fatalf("%d again within a round", a.tip.cur)
		}
		seenNow[a.tip.cur] = true
	}
}

// A tip shows the key its action has now: one the user changed reads as it is
// changed, one unbound is not up at all (F15.4).
func TestTipsFollowTheKeys(t *testing.T) {
	a := tipsApp(t, "", "[keys.grid]\n\"yl\" = \"\"\n\"Q\" = \"grid.yank.insert\"\n\"yi\" = \"\"\n")
	var lines []string
	for range tips {
		a.nextTip()
		lines = append(lines, a.tipText())
	}
	for _, l := range lines {
		if strings.Contains(l, "复制当前单元格") {
			t.Errorf("an unbound action: %q", l)
		}
	}
	if !strings.Contains(strings.Join(lines, "\n"), "table pane 里 Q 把当前行复制成 INSERT 语句") {
		t.Errorf("the key it has now: %q", lines)
	}
	// no tip at all
	a = tipsApp(t, "", "")
	a.tip.cur = 0
	a.tipsOn = false
	if a.tipText() != "" {
		t.Error("tips off")
	}
}

// A round's end does not bring the tip that was up: of the rest, the only one
// there is, or none when none has a key.
func TestTipsRoundEnds(t *testing.T) {
	a := tipsApp(t, "", "")
	a.tip.seen = map[int]bool{}
	for i := range tips {
		a.tip.seen[i] = true
	}
	a.tip.cur = 5
	a.tip.pick = func(int) int { return 5 } // what would be the tip up, were it left in
	a.nextTip()
	if a.tip.cur == 5 || a.tip.cur < 0 || len(a.tip.seen) != 1 {
		t.Errorf("the new round starts with %d, seen %v", a.tip.cur, a.tip.seen)
	}
	was := tips
	t.Cleanup(func() { tips = was })
	tips = tips[:1] // one tip: it comes again
	a.tip.cur, a.tip.seen, a.tip.pick = 0, map[int]bool{0: true}, func(int) int { return 0 }
	a.nextTip()
	if a.tip.cur != 0 {
		t.Errorf("the only tip: %d", a.tip.cur)
	}
	tips = nil
	a.nextTip()
	if a.tip.cur != -1 || a.tipText() != "" {
		t.Errorf("no tips: %d %q", a.tip.cur, a.tipText())
	}
}

// The bar has the tip left of the palette's entry in dim, a click is the next,
// the tick brings another and sets the next one; the info and the tip's
// room go as §7.8 says; with `[ui] tips = false` there is none (F15.4).
func TestTipsStatusBar(t *testing.T) {
	a := tipsApp(t, "", "")
	a.nextTip()
	first := a.tipText()
	row := statusRow(a)
	t.Run("wide", func(t *testing.T) { golden.RequireEqual(t, row) })
	i := strings.Index(row, "Tip: "+first)
	if first == "" || i < 0 || i > strings.Index(row, ui.NerdIcons.Search.Text) {
		t.Fatalf("the tip is not left of the palette's entry: %q", row)
	}
	f := a.render()
	if got := f.Buf.CellAt(ui.Width(row[:i])+1, a.h-1).Style.Fg; got != a.theme.Dim {
		t.Errorf("tip colour %v, not dim", got)
	}
	x := ui.Width(row[:i]) + 2
	click(a, uv.Pos(x, a.h-1))
	if second := a.tipText(); second == first || second == "" {
		t.Errorf("a click: %q, as before %q", second, first)
	}
	second := a.tipText()
	_, cmd := a.Update(tipTick{})
	if cmd == nil || a.tipText() == second {
		t.Errorf("the tick: next tip %q, a new tick %v", a.tipText(), cmd != nil)
	}
	if seg := a.statusLine().Right[0]; !seg.Tip || seg.Drop != dropTip || seg.Action != "tips.next" || a.statusLine().Right[1].Action != "palette.open" {
		t.Errorf("the tip's segment: %+v", seg)
	}
	withTips(t)
	if fresh := sizedWith(200, 45, userConfig(t, "", "")); fresh.tipText() == "" {
		t.Error("no tip up at a start")
	}
	narrow := tipsApp(t, "", "")
	narrow.nextTip()
	narrow.Update(tea.WindowSizeMsg{Width: 80, Height: 30})
	if strings.Contains(statusRow(narrow), "Tip:") || !strings.Contains(statusRow(narrow), "NORMAL") {
		t.Errorf("narrow: the tip gives way: %q", statusRow(narrow))
	}
	narrow.Update(tea.WindowSizeMsg{Width: 109, Height: 30}) // one column short of the tip beside the rest
	t.Run("too narrow", func(t *testing.T) { golden.RequireEqual(t, statusRow(narrow)) })
	narrow.Update(tea.WindowSizeMsg{Width: 110, Height: 30})
	t.Run("just wide enough", func(t *testing.T) { golden.RequireEqual(t, statusRow(narrow)) })
	off := tipsApp(t, "[ui]\ntips = false\n", "")
	off.nextTip()
	if strings.Contains(statusRow(off), "Tip:") {
		t.Errorf("off: %q", statusRow(off))
	}
}

// The tips start with Init's tick, which is not there with tips off.
func TestTipsTimer(t *testing.T) {
	old := tipEvery
	tipEvery = time.Millisecond
	t.Cleanup(func() { tipEvery = old })
	ticked := func(a *App) bool {
		msg := a.Init()()
		batch, _ := msg.(tea.BatchMsg)
		for _, c := range batch {
			done := make(chan tea.Msg, 1)
			go func() { done <- c() }()
			select {
			case m := <-done:
				if _, ok := m.(tipTick); ok {
					return true
				}
			case <-time.After(200 * time.Millisecond):
			}
		}
		return false
	}
	if a := tipsApp(t, "", ""); !ticked(a) {
		t.Error("tips on: no tick")
	}
	if a := tipsApp(t, "[ui]\ntips = false\n", ""); ticked(a) {
		t.Error("tips off: a tick")
	}
}

// tips.show lists the tips in the palette, which types the scope itself:
// searched like any list, ↵ does what the tip is about, and Tab does not
// stop at it (F15.4).
func TestTipsShow(t *testing.T) {
	a := tipsApp(t, "", "")
	a.run("tips.show", 0)
	if a.palette == nil || a.palette.input.Text != "?" {
		t.Fatal("not the tips scope")
	}
	items, ms := a.paletteMatches()
	if len(ms) != len(tips) {
		t.Fatalf("%d of %d tips", len(ms), len(tips))
	}
	if it := items[ms[0].Index]; it.kind != itemTip || strings.Contains(it.name, "{key}") { // the key is in, not its place
		t.Errorf("a row: %+v", it)
	}
	b := tipsApp(t, "", "[keys.grid]\n\"yl\" = \"\"\n") // one with no key is not in the list
	b.run("tips.show", 0)
	if bitems, bms := b.paletteMatches(); len(bms) != len(tips)-1 || slices.ContainsFunc(bms, func(m ui.Match) bool { return strings.Contains(bitems[m.Index].name, "复制当前单元格") }) {
		t.Errorf("%d rows, the unbound one among them or not left out", len(bms))
	}
	feed(t, a, "放大")
	items, ms = a.paletteMatches()
	if len(ms) != 1 || items[ms[0].Index].id != "pane.zoom" {
		t.Fatalf("searching 放大: %d rows", len(ms))
	}
	was := a.win().Zoom
	feed(t, a, "<CR>")
	if a.palette != nil || a.win().Zoom == was {
		t.Errorf("↵: palette %v, zoom %v", a.palette != nil, a.win().Zoom)
	}
	if len(a.state.Recent) != 0 {
		t.Errorf("a tip is not a recent pick: %+v", a.state.Recent)
	}
	a.openPalette("")
	for _, s := range a.paletteView().Scopes {
		if strings.Contains(s, "提示") {
			t.Errorf("a tab of the tips: %q", s)
		}
	}
	for i := range len(scopes) + 1 {
		feed(t, a, "<Tab>")
		if scope, _ := a.paletteScope(); scopes[scope].prefix == tipPrefix {
			t.Errorf("Tab %d stops at the tips", i)
		}
	}
	feed(t, a, "<S-Tab>")
	if scope, _ := a.paletteScope(); scopes[scope].prefix == tipPrefix {
		t.Error("S-Tab stops at the tips")
	}
}
