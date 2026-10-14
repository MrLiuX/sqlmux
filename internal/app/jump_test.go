package app

import (
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/exp/golden"

	"github.com/kackerx/sqlmux/internal/db"
	"github.com/kackerx/sqlmux/internal/editor"
	"github.com/kackerx/sqlmux/internal/keymap"
	"github.com/kackerx/sqlmux/internal/ui"
)

// matchLabels are the labels of a's matches, nearest first, with the cell or
// node each is in.
func matchLabels(a *App) (labels, actions []string) {
	for _, m := range a.jump.matches {
		labels, actions = append(labels, m.label), append(actions, m.target.Action)
	}
	return labels, actions
}

// s begins a jump on a table in NORMAL: the mode block says JUMP and the keys
// are its pattern; a pattern is found in the cells, smartcase, once to a
// cell, the nearest to the cursor first, and the labels leave out the
// letter that follows a match; a label goes there and ends the jump; ↵ goes to
// the nearest; backspace to nothing stays in the jump; esc leaves it with the
// cursor where it was (F10.8).
func TestJumpInTable(t *testing.T) {
	a := wide(160, 45)
	tab := loadOrders(t, a, 6)
	feed(t, a, "s")
	if a.jump == nil || a.mode() != keymap.Insert || a.context().Overlay != "jump" || !strings.Contains(statusRow(a), "JUMP") {
		t.Fatalf("not in a jump: %+v, mode %v, status %q", a.jump, a.mode(), statusRow(a))
	}
	feed(t, a, "run") // running in the first row and the fifth; n follows it
	labels, actions := matchLabels(a)
	if strings.Join(labels, "") != "as" || strings.Join(actions, "|") != "grid.goto 0 1|grid.goto 4 1" {
		t.Fatalf("matches of run: %v in %v, the nearest first and no n", labels, actions)
	}
	feed(t, a, "<BS><BS><BS>") // one more than there is
	if a.jump == nil || a.jump.pattern != "" || len(a.jump.matches) != 0 {
		t.Errorf("backspace to nothing: %+v", a.jump)
	}
	feed(t, a, "Run")
	if a.jump != nil || a.mode() != keymap.Normal || tab.row != 0 || tab.col != 0 {
		t.Errorf("a capital makes it exact, and with no match the jump ends, the cursor where it was: %+v", a.jump)
	}
	feed(t, a, "srun<Esc>")
	if a.jump != nil || tab.row != 0 || tab.col != 0 || a.mode() != keymap.Normal {
		t.Errorf("esc leaves, the cursor stays: jump %v at %d, %d", a.jump, tab.row, tab.col)
	}
	feed(t, a, "srun<CR>") // the nearest
	if a.jump != nil || tab.row != 0 || tab.col != 1 {
		t.Errorf("↵: jump %v at %d, %d", a.jump, tab.row, tab.col)
	}
	feed(t, a, "srun")
	feed(t, a, "s")
	if a.jump != nil || tab.row != 4 || tab.col != 1 {
		t.Errorf("the label s: jump %v at %d, %d", a.jump, tab.row, tab.col)
	}
}

// The labels go to the matches of the focused pane first, the nearest to the
// cursor first, the others after as the screen goes; a cell has one match,
// its first, whatever its text holds (F10.8).
func TestJumpOrder(t *testing.T) {
	a := wide(160, 45)
	loadOrders(t, a, 6)
	feed(t, a, "4jsrun") // the cursor on the fifth row: its running is nearer than the first's
	if labels, actions := matchLabels(a); strings.Join(labels, "") != "as" || strings.Join(actions, "|") != "grid.goto 4 1|grid.goto 0 1" {
		t.Errorf("nearest first: %v in %v", labels, actions)
	}
	feed(t, a, "<Esc>sdo") // done in the table, doraemon in the tree, which comes first on the screen
	var kinds []ui.Kind
	for _, m := range a.jump.matches {
		kinds = append(kinds, m.target.Kind)
	}
	if len(kinds) != 3 || kinds[0] != ui.KindCell || kinds[1] != ui.KindCell || kinds[2] != ui.KindNode {
		t.Errorf("the focused pane's first: %v", kinds)
	}
	feed(t, a, "<BS><BS>line") // "line one↵line two" twice in a cell: once
	seen := map[string]int{}
	for _, m := range a.jump.matches {
		seen[m.target.Action]++
	}
	if len(seen) == 0 {
		t.Fatal("no line")
	}
	for action, n := range seen {
		if n != 1 {
			t.Errorf("%d matches in %s", n, action)
		}
	}
}

// The screen as a jump leaves it: all of it dim, the matches in the match
// color, the label over the character after a match in the yank's colors
// (F10.8).
func TestJumpDrawn(t *testing.T) {
	a := wide(160, 45)
	loadOrders(t, a, 6)
	feed(t, a, "srun")
	f := a.render()
	th := a.theme
	for x := range a.w { // JUMP, in the info color, in the mode block
		if c := f.Buf.CellAt(x, a.h-1); c.Content == "J" && f.Buf.CellAt(x+1, a.h-1).Content == "U" {
			if c.Style.Bg != th.Info {
				t.Errorf("the mode block is %v, want info %v", c.Style.Bg, th.Info)
			}
			break
		}
	}
	m := a.jump.matches[0]
	if got := f.Buf.CellAt(m.at.X, m.at.Y).Style.Fg; got != th.Match {
		t.Errorf("the matched d is %v, want match %v", got, th.Match)
	}
	if c := f.Buf.CellAt(m.at.X+m.width, m.at.Y); c.Content != m.label || c.Style.Bg != th.Yank || c.Style.Fg != th.Bg {
		t.Errorf("the label over the n after: %+v", c)
	}
	if got := f.Buf.CellAt(m.at.X+m.width+1, m.at.Y).Style.Fg; got != th.Dim {
		t.Errorf("the rest is %v, want dim %v", got, th.Dim)
	}
	golden.RequireEqual(t, f.String())
}

// The nodes of the tree are jumped to: the cursor goes to the node and the
// tree has the focus, no table is opened and no node unfolded (F10.8).
func TestJumpInTree(t *testing.T) {
	a := sized(160, 45, "nerd")
	data := a.focused()
	tabs := len(data.Tabs)
	a.win().focus(0)
	feed(t, a, "sT_") // a capital T is exact: nothing is T_ and the jump ends
	if a.jump != nil {
		t.Fatalf("the jump goes on with %d matches", len(a.jump.matches))
	}
	feed(t, a, "st_sku")
	if len(a.jump.matches) != 1 || a.jump.matches[0].target.Kind != ui.KindNode {
		t.Fatalf("matches of t_sku: %+v", a.jump.matches)
	}
	a.win().focus(data.ID) // from elsewhere, the label takes the focus to the tree
	feed(t, a, a.jump.matches[0].label)
	if n, _ := a.treeNode(); n.Text != "t_sku" || a.win().Focus != 0 || len(data.Tabs) != tabs {
		t.Errorf("the cursor is on %q, focus %d, %d tabs", n.Text, a.win().Focus, len(data.Tabs))
	}
	feed(t, a, "s")
	if a.jump == nil {
		t.Fatal("s in the tree begins a jump")
	}
}

// A query's text is matched everywhere, a letter at a time, and a label puts
// the cursor on the character in its pane; s is the jump's in NORMAL and the
// vim's in VISUAL (F10.8).
func TestJumpInQuery(t *testing.T) {
	a := sized(160, 45, "nerd")
	loadOrders(t, a, 3)
	c := queryOf(a.win().Root.B.Pane)
	c.ed.Load("select 1;\nselect 2;")
	feed(t, a, "sct") // from the table: matches in the query too
	var in []int
	for _, m := range a.jump.matches {
		if m.target.Kind == ui.KindText {
			in = append(in, m.at.Y)
		}
	}
	if len(in) != 2 {
		t.Fatalf("matches of ct in the query: %d", len(in))
	}
	feed(t, a, a.jump.matches[len(a.jump.matches)-1].label) // the last on the screen
	cur := c.ed.Cursor()
	if a.focused() != a.win().Root.B.Pane || cur.Line != 1 || cur.Col != 4 {
		t.Errorf("focus %d, the cursor at %+v", a.win().Focus, cur)
	}
	if feed(t, a, "<Esc>s"); a.jump == nil {
		t.Fatal("s in the query's NORMAL begins a jump")
	}
	feed(t, a, "<Esc>")
	feed(t, a, "vs") // VISUAL: the vim's s
	if a.jump != nil {
		t.Error("s in VISUAL is the editor's")
	}
}

// s in a JSON view jumps in its text as in a query's: the matches are all of
// them in it, ↵ goes to the nearest, a label to its own, the cursor on the
// character; esc leaves it where it was (F16.8).
func TestJumpInJSONView(t *testing.T) {
	a := sized(160, 45, "nerd")
	loadOrders(t, a, 3)
	feed(t, a, "gj")
	jv := jsonViewOf(a.focused())
	feed(t, a, "sstat")
	var in []jumpMatch
	for _, m := range a.jump.matches {
		if m.target.Kind == ui.KindText && m.target.Pane == a.focused().ID {
			in = append(in, m)
		}
	}
	if len(in) != 3 {
		t.Fatalf("matches of stat in the text: %d of %d", len(in), len(a.jump.matches))
	}
	if feed(t, a, "<CR>"); a.jump != nil || jv.ed.Cursor() != (editor.Pos{Line: 3, Col: 5}) {
		t.Fatalf("↵ to the nearest: jump %v, cursor %+v", a.jump != nil, jv.ed.Cursor())
	}
	feed(t, a, "sstat")
	feed(t, a, a.jump.matches[len(a.jump.matches)-1].label) // the last on the screen
	if a.jump != nil || jv.ed.Cursor().Line != 3+2*15 || jv.ed.Cursor().Col != 5 {
		t.Errorf("a label: jump %v, cursor %+v", a.jump != nil, jv.ed.Cursor())
	}
	feed(t, a, "ggsstat<Esc>")
	if a.jump != nil || jv.ed.Cursor().Line != 0 || jsonViewOf(a.focused()) == nil {
		t.Errorf("esc: jump %v, cursor %+v", a.jump != nil, jv.ed.Cursor())
	}
}

func TestGoldenJumpJSONView(t *testing.T) {
	a := wide(160, 45)
	loadOrders(t, a, 3)
	feed(t, a, "gjsstat")
	golden.RequireEqual(t, a.render().String())
}

// A click or the wheel puts the jump away, the click going where it is (F10.8).
func TestJumpLeftByClickAndWheel(t *testing.T) {
	a := wide(160, 45)
	loadOrders(t, a, 3)
	feed(t, a, "srun")
	click(a, a.jump.matches[0].at)
	if a.jump != nil {
		t.Error("a click ends it")
	}
	feed(t, a, "sd")
	a.Update(tea.MouseWheelMsg{X: 70, Y: 10, Button: tea.MouseWheelDown})
	if a.jump != nil {
		t.Error("the wheel ends it")
	}
	feed(t, a, "sd")
	a.Update(tea.WindowSizeMsg{Width: 150, Height: 40})
	if a.jump != nil {
		t.Error("a resize ends it")
	}
}

// What is not the jump's own ends it: a global key does what it does (the
// palette opens and has the keys), a right click goes where it is; the
// palette's jump on a query in INSERT begins one and types nothing (F10.8).
func TestJumpEndedByOthers(t *testing.T) {
	a := sized(160, 45, "nerd")
	loadOrders(t, a, 3)
	feed(t, a, "s<C-p>")
	if a.jump != nil || a.palette == nil {
		t.Errorf("C-p in a jump: jump %v, palette %v", a.jump, a.palette != nil)
	}
	feed(t, a, "<Esc>s")
	a.Update(tea.MouseClickMsg{X: 1, Y: 1, Button: tea.MouseRight})
	if a.jump != nil {
		t.Error("a right click leaves the jump")
	}
	c := queryOf(a.win().Root.B.Pane)
	a.focusPane(a.win().Root.B.Pane.ID)
	c.ed.Load("select 1;")
	feed(t, a, "i")
	if c.ed.Mode() != editor.Insert {
		t.Fatalf("mode %v", c.ed.Mode())
	}
	a.run("jump.start", 0)
	if a.jump == nil || strings.Join(c.ed.Lines(), "\n") != "select 1;" {
		t.Errorf("jump %v, text %q", a.jump, c.ed.Lines())
	}
}

// What follows an occurrence of the pattern anywhere on the screen is no label
// as it goes on being typed, the occurrence being a match or not: a cell has
// the one match, its first, and a query's overlap (F10.8).
func TestJumpSkipsAfterEveryOccurrence(t *testing.T) {
	a := wide(160, 45)
	loadOrders(t, a, 30)
	feed(t, a, `s"`) // the cells of meta have "a" after a quote, and not as the first of them
	feed(t, a, "a")
	if a.jump == nil || a.jump.pattern != `"a` {
		t.Errorf("a was a label: %+v", a.jump)
	}
	a = sized(160, 45, "nerd")
	loadOrders(t, a, 3)
	c := queryOf(a.win().Root.B.Pane)
	a.focusPane(a.win().Root.B.Pane.ID)
	c.ed.Load("select 'oooa';")
	feed(t, a, "soo") // oo twice in ooo, one match, and what follows the second is a
	feed(t, a, "a")
	if a.jump == nil || a.jump.pattern != "ooa" {
		t.Errorf("a was a label after an overlapping oo: %+v", a.jump)
	}
}

// C-c is esc in a jump: it ends it, is not the first of two presses that quit
// and does not cancel a query (§6.8, F10.8).
func TestJumpCtrlC(t *testing.T) {
	a := wide(160, 45)
	loadOrders(t, a, 6)
	quit := feed(t, a, "s<C-c>")
	if a.jump != nil || a.toast != "" {
		t.Fatalf("jump %v, toast %q", a.jump, a.toast)
	}
	if quit = feed(t, a, "s<C-c>") || quit; quit {
		t.Errorf("s C-c s C-c quits")
	}
	a.conn().busy = 1
	feed(t, a, "s<C-c>")
	if a.jump != nil || a.conn().busy != 1 {
		t.Errorf("a running query cancelled: busy %d", a.conn().busy)
	}
}

// What is pasted in a jump is the pattern's whatever has the focus, a
// query's text left alone (F10.8).
func TestJumpPaste(t *testing.T) {
	a := sized(160, 45, "nerd")
	loadOrders(t, a, 3)
	c := queryOf(a.win().Root.B.Pane)
	a.focusPane(a.win().Root.B.Pane.ID)
	c.ed.Load("select 1;")
	feed(t, a, "ssel")
	a.Update(tea.PasteMsg{Content: "ec"})
	if a.jump == nil || a.jump.pattern != "selec" || strings.Join(c.ed.Lines(), "") != "select 1;" {
		t.Errorf("pattern %+v, text %q", a.jump, c.ed.Lines())
	}
	a.Update(tea.PasteMsg{Content: "\nx"}) // a newline is a space, which no text has before x: the jump ends
	if a.jump != nil || strings.Join(c.ed.Lines(), "") != "select 1;" {
		t.Errorf("a paste with no match: %+v, text %q", a.jump, c.ed.Lines())
	}
}

// The palette's jump on a query in VISUAL begins a jump and leaves the
// selection; the key is the vim's (F10.8).
func TestJumpFromPaletteInVisual(t *testing.T) {
	a := sized(160, 45, "nerd")
	loadOrders(t, a, 3)
	c := queryOf(a.win().Root.B.Pane)
	a.focusPane(a.win().Root.B.Pane.ID)
	c.ed.Load("select 1;")
	feed(t, a, "vl<C-p>")
	a.paletteDo(paletteItem{kind: itemCommand, id: "jump.start"})
	if a.jump == nil || strings.Join(c.ed.Lines(), "") != "select 1;" {
		t.Errorf("jump %v, text %q", a.jump, c.ed.Lines())
	}
}

// The backdrop does not dim the status bar: the JUMP block keeps its own
// colors (F10.8).
func TestJumpBackdropSparesStatusBar(t *testing.T) {
	a := wide(160, 45)
	loadOrders(t, a, 6)
	feed(t, a, "s")
	f := a.render()
	for x := range a.w {
		if c := f.Buf.CellAt(x, a.h-1); c.Content == "J" && f.Buf.CellAt(x+1, a.h-1).Content == "U" {
			if c.Style.Fg == a.theme.Dim {
				t.Errorf("JUMP is dim")
			}
			return
		}
	}
	t.Error("no JUMP block")
}

// A message that changes the screen under the matches, a catalog that comes
// with a table before the one that matched, has them read again: the label
// goes where it is drawn (F10.8).
func TestJumpRescansWhatChanges(t *testing.T) {
	a := sized(160, 45, "nerd")
	a.win().focus(0)
	feed(t, a, "st_sku")
	conn := a.sess.Conns[0]
	tables := append([]db.Table{{Schema: "public", Name: "aaa_new"}}, conn.Tables...)
	a.Update(catalogMsg{conn: conn, schemas: conn.Schemas, current: conn.home, tables: tables})
	if len(a.jump.matches) != 1 {
		t.Fatalf("%d matches", len(a.jump.matches))
	}
	feed(t, a, a.jump.matches[0].label)
	if n, _ := a.treeNode(); n.Text != "t_sku" {
		t.Errorf("the cursor is on %q", n.Text)
	}
}

// The labels go to the nearest by lines first, then by columns, as flash.nvim
// has it: a cell in the next row is further than one in the row of the cursor
// however far right (F10.8).
func TestJumpNearestByLines(t *testing.T) {
	a := wide(160, 45)
	loadOrders(t, a, 6)
	feed(t, a, "s2") // the date of the first row, 2026, and the next row's id
	if got := a.jump.matches[0].target.Action; got != "grid.goto 0 6" {
		t.Errorf("the first label is on %s", got)
	}
}

// The pattern is case-blind when it has no capital, in a query too, and the
// letter after a match in capitals is no label either; a label of the second
// round is a capital (F10.8).
func TestJumpSmartcaseAndCapitals(t *testing.T) {
	a := sized(160, 45, "nerd")
	loadOrders(t, a, 3)
	c := queryOf(a.win().Root.B.Pane)
	a.focusPane(a.win().Root.B.Pane.ID)
	c.ed.Load(strings.TrimSpace(strings.Repeat("SELECT 1;\n", 14)))
	feed(t, a, "ssel")
	n := 0
	for _, m := range a.jump.matches {
		if m.target.Kind == ui.KindText {
			n++
		}
	}
	labels, _ := matchLabels(a)
	if n != 14 || slices.Contains(labels, "e") || slices.Contains(labels, "E") {
		t.Errorf("%d matches of sel in SELECT, labels %v", n, labels)
	}
	b := wide(160, 45)
	loadOrders(t, b, 30)
	feed(t, b, "s0") // more cells than letters: the rest are capitals
	labels, _ = matchLabels(b)
	if len(labels) < 30 || !slices.ContainsFunc(labels, func(l string) bool { return l != strings.ToLower(l) }) {
		t.Errorf("labels %v", labels)
	}
}

// A label on a table's cell, the jump begun from the query, puts the focus
// on the table and the cursor on the cell (F10.8).
func TestJumpToCellFocusesItsPane(t *testing.T) {
	a := sized(160, 45, "nerd")
	tab := loadOrders(t, a, 6)
	data := a.focused()
	a.focusPane(a.win().Root.B.Pane.ID)
	feed(t, a, "srun")
	i := slices.IndexFunc(a.jump.matches, func(m jumpMatch) bool { return m.target.Kind == ui.KindCell })
	feed(t, a, a.jump.matches[i].label)
	if a.focused() != data || tab.row != 0 || tab.col != 1 {
		t.Errorf("focus %d, cursor %d, %d", a.win().Focus, tab.row, tab.col)
	}
}

// The first labels are the nearest to the cursor of the focused pane in the
// tree, in a query and in a result as in a table: the cursor is found
// there (F10.8).
func TestJumpNearestInTreeQueryResult(t *testing.T) {
	a := sized(160, 45, "nerd")
	loadOrders(t, a, 3)
	a.win().focus(0)
	feed(t, a, "Gst_") // the cursor on the last node
	var ys []int
	for _, m := range a.jump.matches {
		if m.target.Kind == ui.KindNode {
			ys = append(ys, m.at.Y)
		}
	}
	if len(ys) < 2 || ys[0] != slices.Max(ys) {
		t.Errorf("the tree's matches: %v", ys)
	}
	feed(t, a, "<Esc>")
	c := queryOf(a.win().Root.B.Pane)
	a.focusPane(a.win().Root.B.Pane.ID)
	c.ed.Load("select 1;\nselect 2;\nselect 3;")
	feed(t, a, "Gssel") // the cursor on the last line
	ys = nil
	for _, m := range a.jump.matches {
		if m.target.Kind == ui.KindText {
			ys = append(ys, m.at.Y)
		}
	}
	if len(ys) != 3 || ys[0] != slices.Max(ys) {
		t.Errorf("the query's matches: %v", ys)
	}
	feed(t, a, "<Esc>")

	r := db.Result{Cols: []db.Col{{Name: "n", Type: "text"}}, Rows: [][]db.Val{{{S: "x1"}}, {{S: "x2"}}, {{S: "x3"}}}}
	b, _ := inRun(t, &execDB{res: map[string]db.Result{"select 1": r}}, "select 1;")
	press(t, b, "<CR>")
	b.win().focus(b.win().Result.ID)
	feed(t, b, "jjsx")
	if got := b.jump.matches[0].target.Action; got != "grid.goto 2 0" {
		t.Errorf("the result's first match is %s", got)
	}
}

// The letter after a match is no label in either case, the capitals of the
// second round too: more matches than letters, one followed by X (F10.8).
func TestJumpNoCapitalLabelAfterMatch(t *testing.T) {
	a := sized(160, 45, "nerd")
	loadOrders(t, a, 3)
	c := queryOf(a.win().Root.B.Pane)
	a.focusPane(a.win().Root.B.Pane.ID)
	line := strings.Repeat("sel ", 14)
	c.ed.Load(strings.Repeat(line+"\n", 4) + "selX")
	feed(t, a, "ssel")
	labels, _ := matchLabels(a)
	if len(labels) < 47 || slices.Contains(labels, "x") || slices.Contains(labels, "X") || !slices.ContainsFunc(labels, func(l string) bool { return l != strings.ToLower(l) }) {
		t.Errorf("%d labels: %v", len(labels), labels)
	}
}

// A character that no text on the screen follows the pattern with ends the
// jump at once, the cursor where it was and the character dropped, in a table,
// a query and the tree alike; it does not stay in a screen all dim (F15.7).
func TestJumpEndsWithNoMatch(t *testing.T) {
	a := sized(160, 45, "nerd")
	loadOrders(t, a, 3)
	c := queryOf(a.win().Root.B.Pane)
	a.focusPane(a.win().Root.B.Pane.ID)
	c.ed.Load("select 1;")
	cur := c.ed.Cursor()
	feed(t, a, "sselect")
	if a.jump == nil || len(a.jump.matches) != 1 {
		t.Fatalf("jump %+v", a.jump)
	}
	feed(t, a, "#")
	if a.jump != nil || a.mode() != keymap.Normal || a.context().Overlay != "" || c.ed.Cursor() != cur {
		t.Errorf("after a character no text follows with: jump %+v, mode %v, overlay %q", a.jump, a.mode(), a.context().Overlay)
	}
	feed(t, a, "s#") // the first character already
	if a.jump != nil {
		t.Errorf("a first character with no match: %+v", a.jump)
	}
}
