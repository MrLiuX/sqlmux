package app

import (
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/exp/golden"

	"sqlmux/internal/config"
	"sqlmux/internal/db"
	"sqlmux/internal/db/mysql"
	"sqlmux/internal/db/postgres"
	"sqlmux/internal/editor"
	"sqlmux/internal/keymap"
	"sqlmux/internal/ui"
)

// Completion (§9.7): columns and keywords with a word starting as the one typed does,
// an enum's values where one goes. The first is selected as the list opens,
// Tab and S-Tab move around the ends, ↵ takes the selected one and runs
// nothing; esc closes the list before the input.
func TestWhereCompletion(t *testing.T) {
	a, tab, _ := withRecorder(t)
	feed(t, a, "/a")
	if tab.comp == nil || len(tab.comp.items) != 3 || tab.comp.items[0].label != "amount" || tab.comp.items[1].label != "created_at" || tab.comp.sel != 0 || a.mode() != keymap.Insert {
		t.Fatalf("a: amount, created_at (a word of it), and: %+v", tab.comp)
	}
	at := a.whereAt(a.focused(), tab)
	f := a.render()
	row := strings.Split(f.String(), "\n")[at.Y+2] // under the input and the list's border
	i := strings.Index(row, "amount")
	if bg := f.Buf.CellAt(ui.Width(row[:i])-1, at.Y+2).Style.Bg; bg != a.theme.Select { // the cell before it: the row's, not the match's
		t.Errorf("selected as it opens: %v", bg)
	}
	if feed(t, a, "<Tab>"); tab.comp.sel != 1 {
		t.Fatalf("the first Tab moves to the second: %+v", tab.comp)
	}
	feed(t, a, "<BS>sta")
	if tab.comp == nil || tab.comp.items[0].label != "status" || tab.comp.sel != 0 {
		t.Fatalf("sta: %+v", tab.comp)
	}
	feed(t, a, "<CR>")
	if tab.where.Text != "status " || tab.comp != nil || tab.applied != "" {
		t.Fatalf("↵ takes status: %q applied %q", tab.where.Text, tab.applied)
	}
	feed(t, a, "= ")
	var labels []string
	for _, c := range tab.comp.items {
		labels = append(labels, c.label)
	}
	if strings.Join(labels, " ") != "pending running done failed" {
		t.Fatalf("values: %v", labels)
	}
	for _, c := range []struct {
		keys string
		sel  int
	}{{"<S-Tab>", 3}, {"<Tab>", 0}, {"<C-p><Up>", 2}, {"<C-n><Down>", 0}, {"<Down>", 1}} {
		if feed(t, a, c.keys); tab.comp.sel != c.sel {
			t.Errorf("%s: sel %d, want %d", c.keys, tab.comp.sel, c.sel)
		}
	}
	feed(t, a, "<CR>")
	if tab.where.Text != "status = 'running'" || tab.applied != "" {
		t.Fatalf("↵ on running: %q applied %q", tab.where.Text, tab.applied)
	}
	feed(t, a, " and paid is not nu")
	if tab.comp == nil || tab.comp.items[0].label != "null" || len(tab.comp.items) != 1 {
		t.Fatalf("nu: only null starts with n, not is null: %+v", tab.comp)
	}
	feed(t, a, "ll<CR>")
	if tab.applied != "" || tab.where.Text != "status = 'running' and paid is not null" || tab.comp != nil {
		t.Errorf("↵ takes null, a value that takes no space, and does no more: %q, applied %q", tab.where.Text, tab.applied)
	}
	feed(t, a, "<CR>")
	if tab.applied != "status = 'running' and paid is not null" || tab.typing != "" {
		t.Errorf("↵ with the list closed runs: %q", tab.applied)
	}
	feed(t, a, "/ i<Esc>")
	if tab.comp != nil || tab.typing != "where" {
		t.Fatal("the first esc closes the list")
	}
	feed(t, a, "<Esc>")
	if tab.typing != "" {
		t.Error("the second esc leaves the input")
	}
}

// A WHERE's ↵ with the candidates up only takes the selected one, though
// what is typed is all of it: it is not run till the list is gone (§9.7, F8.1).
func TestWhereEnterOnlyAccepts(t *testing.T) {
	a, tab, _ := withRecorder(t)
	feed(t, a, "/id<CR>")
	if tab.where.Text != "id " || tab.applied != "" || tab.typing != "where" || tab.comp != nil {
		t.Fatalf("id: %q, applied %q, typing %q", tab.where.Text, tab.applied, tab.typing)
	}
	feed(t, a, "> 5<CR>")
	if tab.applied != "id > 5" {
		t.Errorf("a second ↵ runs: %q", tab.applied)
	}
}

// After AND or OR and a blank the WHERE's list opens with nothing typed:
// the columns, no keyword; an empty input opens none, ↵ there clears the
// filter (§9.7, F8.4).
func TestWhereColumnsAfterAnd(t *testing.T) {
	a, tab, _ := withRecorder(t)
	feed(t, a, "/")
	if tab.comp != nil {
		t.Fatalf("an empty input: %+v", tab.comp)
	}
	feed(t, a, "id > 5 and")
	if tab.comp != nil && tab.comp.items[0].label != "and" {
		t.Errorf("and, no blank: %+v", tab.comp.items)
	}
	feed(t, a, " ")
	if tab.comp == nil || len(tab.comp.items) != len(tab.cols.Cols) || tab.comp.items[0].kw {
		t.Fatalf("and and a blank: the columns: %+v", tab.comp)
	}
	feed(t, a, "<BS><BS><BS><BS>or ")
	if tab.comp == nil || tab.comp.items[0].kw {
		t.Errorf("or: %+v", tab.comp)
	}
	feed(t, a, "<Esc>")
	feed(t, a, "<BS><BS><BS><BS>")
	if tab.comp != nil {
		t.Errorf("after id > 5 : %+v", tab.comp)
	}
}

// Completion ignores case throughout, first character and fuzzy rest
// alike; the palette keeps fzf's smart case (§9.7).
func TestRankedIgnoresCase(t *testing.T) {
	cands := []candidate{{label: "status"}, {label: "amount"}, {label: "t_order"}, {label: "Mixed_Case"}}
	for pattern, want := range map[string]string{"St": "status", "STA": "status", "Am": "amount", "T_OR": "t_order", "mix": "Mixed_Case"} {
		if c := ranked(pattern, len(pattern), pattern, 0, cands); c == nil || len(c.items) != 1 || c.items[0].label != want {
			t.Errorf("%s: %+v, want %s", pattern, c, want)
		}
	}
	a := sized(160, 45, "nerd")
	if feed(t, a, "<C-p>@t_ord"); !slices.Contains(namesOf(a), "表:t_order") {
		t.Fatalf("palette: %v", namesOf(a))
	}
	if feed(t, a, "<BS><BS><BS><BS><BS>T_ORD"); slices.Contains(namesOf(a), "表:t_order") {
		t.Error("the palette's T_ORD must stay exact about case")
	}
}

// The first character typed matches at a word start: the first, one after
// _ . - $, or a lower-to-upper turn; the rest is fuzzy (§9.7).
func TestRankedWordStart(t *testing.T) {
	cands := []candidate{{label: "mt_event"}, {label: "t_order"}, {label: "t_event"}, {label: "max"}, {label: "exists"}, {label: "orderId"}, {label: "a.b-c$d"}}
	for pattern, want := range map[string][]string{
		"evt": {"t_event", "mt_event"}, "tord": {"t_order"}, "ev": {"t_event", "mt_event"}, "x": nil,
		"id": {"orderId"}, "b": {"a.b-c$d"}, "c": {"a.b-c$d"}, "d": {"a.b-c$d"},
	} {
		var got []string
		if c := ranked(pattern, len(pattern), pattern, 0, cands); c != nil {
			for _, it := range c.items {
				got = append(got, it.label)
			}
		}
		if !slices.Equal(got, want) {
			t.Errorf("%s: %v, want %v", pattern, got, want)
		}
	}
	// fzf's best match takes the run abcdef after x; the one from the word
	// start after _ still counts, and is what gets highlighted
	s := "xabcdef_a" + strings.Repeat("q", 20) + "bcdef"
	if m := ui.Filter("abcdef", []string{s}); len(m) != 1 || m[0].Pos[0] != 1 {
		t.Fatalf("fzf no longer prefers the run: %+v", m)
	}
	c := ranked("abcdef", len("abcdef"), "abcdef", 0, []candidate{{label: s}})
	if c == nil || !slices.Equal(c.items[0].pos, []int{8, 29, 30, 31, 32, 33}) {
		t.Errorf("from the word start: %+v", c)
	}
	// a value with a space: fzf's terms each match anywhere, the first
	// position may be the second term's
	c = ranked("in p", len("in p"), "in p", 0, []candidate{{label: "prod in"}, {label: "in progress"}, {label: "on hold"}})
	if c == nil || len(c.items) != 1 || c.items[0].label != "in progress" {
		t.Errorf("in p: %+v", c)
	}
}

// Autopairs in a WHERE, the quick SQL and a console's INSERT, not in a
// cell's edit or the palette's other scopes, off with autopairs = false
// (§7.9). Taking a value swallows the closing quote after the cursor.
func TestAutoPairs(t *testing.T) {
	a, tab, _ := withRecorder(t)
	feed(t, a, "/status = '")
	if tab.where.Text != "status = ''" || tab.where.Pos != 10 {
		t.Fatalf("paired: %q at %d", tab.where.Text, tab.where.Pos)
	}
	if feed(t, a, "done'"); tab.where.Text != "status = 'done'" || tab.where.Pos != 15 {
		t.Fatalf("stepped over: %q at %d", tab.where.Text, tab.where.Pos)
	}
	feed(t, a, "<BS><BS><BS><BS><BS><BS>")
	if tab.where.Text != "status = " {
		t.Fatalf("BS: %q", tab.where.Text)
	}
	if feed(t, a, "'ru<CR>"); tab.where.Text != "status = 'running'" || tab.where.Pos != 18 || tab.applied != "" {
		t.Fatalf("taking 'running': %q at %d", tab.where.Text, tab.where.Pos)
	}
	feed(t, a, "<Esc><Esc>i(")
	if tab.cell == nil || tab.cell.in.Text != "(" {
		t.Fatalf("a cell's edit doesn't pair: %+v", tab.cell)
	}
	feed(t, a, "<Esc><C-p>(")
	if a.palette.input.Text != "(" {
		t.Fatalf("the palette's other scopes don't: %q", a.palette.input.Text)
	}
	if feed(t, a, "<BS>;select count("); a.palette.input.Text != ";select count()" || a.palette.input.Pos != 14 {
		t.Fatalf("quick SQL: %q at %d", a.palette.input.Text, a.palette.input.Pos)
	}
	a.paste("count(")
	if a.palette.input.Text != ";select count(count()" || !a.autoPairs {
		t.Fatalf("a paste goes in as it is: %q", a.palette.input.Text)
	}
	a.autoPairs = false
	if in := (ui.Input{}); a.editPaired(&in, "(") && in.Text != "(" {
		t.Errorf("off: %q", in.Text)
	}

	a, c := inConsole(t, "", "")
	if feed(t, a, "icount("); text(c) != "count()" || c.ed.Cursor().Col != 6 {
		t.Fatalf("console: %q at %v", text(c), c.ed.Cursor())
	}
	if feed(t, a, "<Esc>u"); text(c) != "" {
		t.Errorf("one u takes the INSERT: %q", text(c))
	}
	a, c = inConsole(t, "autopairs = false", "")
	if feed(t, a, "i("); text(c) != "(" {
		t.Errorf("console off: %q", text(c))
	}
}

// Taking a candidate that differs from the word only in case changes
// nothing: SQL's keywords and bare names ignore it (§9.7).
func TestAcceptIgnoresCase(t *testing.T) {
	c := &completion{items: []candidate{{label: "null", insert: "null"}}, start: 4}
	in := ui.Input{Text: "a = NULL", Pos: 8}
	if c.accept(&in); in.Text != "a = NULL" {
		t.Errorf("NULL: changed to %q", in.Text)
	}
	in = ui.Input{Text: "a = nu", Pos: 6}
	if c.accept(&in); in.Text != "a = null" || in.Pos != 8 {
		t.Errorf("nu: %q at %d", in.Text, in.Pos)
	}
}

// A keyword takes a space as it is taken, or the cursor goes past the
// whitespace that is there, in a space or a tab, and what is typed stays in
// its case; the closing quote that is there already is not put in again (§9.7). The words of a
// keyword written before the cursor are not put in twice.
func TestAcceptKeywordSpace(t *testing.T) {
	kw := func(s string) candidate { return candidate{insert: s, note: "关键字", kw: true, space: true} }
	val := candidate{insert: "null", note: "关键字"}
	quoted := candidate{insert: "'done'", note: "值"}
	for _, c := range []struct {
		text       string
		pos, start int // the cursor, and where the list's word starts
		cd         candidate
		want       string
		wantPos    int
	}{
		{"selec", 5, 0, kw("select"), "select ", 7},
		{"a = 1 an", 8, 6, kw("and"), "a = 1 and ", 10},
		{"a = 1 and", 9, 6, kw("and"), "a = 1 and ", 10},
		{"a = 1 and b", 9, 6, kw("and"), "a = 1 and b", 10}, // past the space
		{"a = 1 and\tb", 9, 6, kw("and"), "a = 1 and\tb", 10},
		{"a = 1 AND b", 9, 6, kw("and"), "a = 1 AND b", 10}, // typed case kept
		{"SELECT", 6, 0, kw("select"), "SELECT ", 7},
		{"insert into", 11, 7, kw("insert into"), "insert into ", 12},
		{"insert in", 9, 7, kw("insert into"), "insert into ", 12},
		{"x rename to", 11, 9, kw("rename to"), "x rename to ", 12},
		{"x REPLACE in", 12, 10, kw("replace into"), "x replace into ", 15}, // partly typed: the candidate's
		{"a = nul", 7, 4, val, "a = null", 8},
		{"a = null", 8, 4, val, "a = null", 8},
		{"a = NULL", 8, 4, val, "a = NULL", 8},
		{"s = 'done", 9, 4, quoted, "s = 'done'", 10},
		{"s = 'done'", 9, 4, quoted, "s = 'done'", 10}, // the quote autopairs put there: ↵ takes the value all the same
	} {
		in := ui.Input{Text: c.text, Pos: c.pos}
		cp := &completion{items: []candidate{c.cd}, start: c.start}
		if cp.accept(&in); in.Text != c.want || in.Pos != c.wantPos {
			t.Errorf("%q at %d: %q at %d", c.text, c.pos, in.Text, in.Pos)
		}
	}
}

// Every WHERE run goes into the table's history; C-r lists favorites then
// history, a star or a clock before each, all of it till something is
// typed, then filtered by it; Tab, ↓, S-Tab and ↑ move around the ends;
// C-f stars; ↵ runs one with its ORDER and LIMIT; it all outlives a
// restart (Q-02, §9.7, §14).
func TestWhereHistory(t *testing.T) {
	a, tab, _ := withRecorder(t)
	feed(t, a, "/id > 5<CR>")
	tab.order, tab.limit = "amount", 500
	wipe := strings.Repeat("<BS>", 20)
	feed(t, a, "/"+wipe+"status = 'done'<CR>")
	tab.order, tab.limit = "", 100
	feed(t, a, "/"+wipe+"id > 5<CR>") // again: to the top, once
	st := a.tableState(tab)
	if len(st.History) != 2 || st.History[0].Where != "id > 5" || st.History[1].Limit != 500 {
		t.Fatalf("history %+v", st.History)
	}
	feed(t, a, "/"+wipe+"xyz<C-r>") // what is there does not filter it
	if es, _ := a.histEntries(tab); tab.hist == nil || a.mode() != keymap.Command || len(es) != 2 {
		t.Fatalf("C-r opens the list, all of it: %+v", es)
	}
	feed(t, a, "<C-n><C-f>")
	if len(st.Favorites) != 1 || st.Favorites[0].Where != "status = 'done'" {
		t.Fatalf("favorites %+v", st.Favorites)
	}
	f := a.render().String()
	if !strings.Contains(f, a.icons.Star.Text+" status = 'done'") || !strings.Contains(f, a.icons.History.Text+" id > 5") || !strings.Contains(f, "amount ↑ · 500") {
		t.Errorf("the list:\n%s", f)
	}
	for keys, want := range map[string]int{"<Up>": 2, "<Down>": 1, "<Tab><Tab><Tab>": 0, "<S-Tab>": 2} { // from the first
		if tab.hist.sel = 0; feed(t, a, keys) || tab.hist.sel != want {
			t.Errorf("%s: at %d, want %d", keys, tab.hist.sel, want)
		}
	}
	feed(t, a, wipe+"don")                                                    // filters the list
	if es, _ := a.histEntries(tab); len(es) != 2 || es[0].fav || !es[1].fav { // its run, and the favorite after
		t.Fatalf("filtered: %+v", es)
	}
	feed(t, a, "<CR>")
	if tab.applied != "status = 'done'" || tab.order != "amount" || tab.limit != 500 || tab.hist != nil {
		t.Fatalf("applied %q order %q limit %d", tab.applied, tab.order, tab.limit)
	}

	loaded, err := config.LoadState()
	if err != nil || len(loaded.Tables["doraemon/public.t_order"].Favorites) != 1 {
		t.Fatalf("after a restart: %+v %v", loaded, err)
	}
}

// C-f in the WHERE input stars what is typed with the tab's ORDER and LIMIT,
// again un-stars it, and says which; nothing typed, or another input, is
// nothing (F9.2).
func TestWhereStarInTheInput(t *testing.T) {
	a, tab, _ := withRecorder(t)
	st := a.tableState(tab)
	feed(t, a, "/<C-f>")
	if len(st.Favorites) != 0 || a.toast != "" {
		t.Fatalf("nothing typed: %+v, toast %q", st.Favorites, a.toast)
	}
	tab.order, tab.limit = "amount", 500
	feed(t, a, "id > 5 <C-f>") // trimmed
	if len(st.Favorites) != 1 || st.Favorites[0].Where != "id > 5" || st.Favorites[0].Order != "amount" || st.Favorites[0].Limit != 500 || a.toast != "已收藏" {
		t.Fatalf("starred: %+v, toast %q", st.Favorites, a.toast)
	}
	if tab.typing != "where" || tab.where.Text != "id > 5 " {
		t.Errorf("the input: %q, typing %q", tab.where.Text, tab.typing)
	}
	feed(t, a, "<C-f>")
	if len(st.Favorites) != 0 || a.toast != "已取消收藏" {
		t.Errorf("again: %+v, toast %q", st.Favorites, a.toast)
	}
	tab.order, tab.limit = "", 100 // another ORDER: another entry
	feed(t, a, "<C-f>")
	tab.order = "amount"
	feed(t, a, "<C-f>")
	if len(st.Favorites) != 2 || st.Favorites[0].Order != "amount" || st.Favorites[1].Order != "" || st.Favorites[1].Limit != 0 {
		t.Errorf("two: %+v", st.Favorites)
	}
	if loaded, err := config.LoadState(); err != nil || len(loaded.Tables["doraemon/public.t_order"].Favorites) != 2 {
		t.Errorf("after a restart: %+v %v", loaded, err)
	}
	// another input has the key and does nothing
	feed(t, a, "<Esc><Esc><C-p>id<C-f>")
	if len(st.Favorites) != 2 || a.palette == nil || a.palette.input.Text != "id" {
		t.Errorf("in the palette: %d favorites, palette %+v", len(st.Favorites), a.palette)
	}
	tab.where.Text = "id > 5" // what a PAGE input has the key to do nothing about
	feed(t, a, "<Esc>gp")
	if tab.typing != "page" {
		t.Fatalf("typing %q", tab.typing)
	}
	if feed(t, a, "<C-f>"); len(st.Favorites) != 2 {
		t.Errorf("in the PAGE input: %d favorites", len(st.Favorites))
	}
}

// C-e in the list puts the selected entry's WHERE in the input: INSERT, the
// cursor at the end, nothing run, its ORDER and LIMIT left where they are;
// ↵ then runs what was edited (F9.1).
func TestWhereHistoryEdit(t *testing.T) {
	a, tab, _ := withRecorder(t)
	wipe := strings.Repeat("<BS>", 20)
	feed(t, a, "/id > 5<CR>")
	tab.order, tab.limit = "amount", 500
	feed(t, a, "/"+wipe+"status = 'done'<CR>")
	tab.order, tab.limit = "", 100
	feed(t, a, "/"+wipe+"<C-r>")
	if es, _ := a.histEntries(tab); tab.hist == nil || len(es) != 2 || es[0].q.Where != "status = 'done'" {
		t.Fatalf("the list: %+v", es)
	}
	applied := tab.applied
	feed(t, a, "<C-e>")
	if tab.hist != nil || tab.typing != "where" || tab.ed.Mode() != editor.Insert || tab.where.Text != "status = 'done'" || tab.where.Pos != len("status = 'done'") {
		t.Fatalf("C-e: list %v, typing %q, mode %v, input %q at %d", tab.hist != nil, tab.typing, tab.ed.Mode(), tab.where.Text, tab.where.Pos)
	}
	if tab.applied != applied || tab.order != "" || tab.limit != 100 {
		t.Errorf("run, or order / limit taken: applied %q order %q limit %d", tab.applied, tab.order, tab.limit)
	}
	feed(t, a, "<BS><BS><BS><BS><BS>new'<CR>")
	if tab.applied != "status = 'new'" || tab.order != "" {
		t.Errorf("edited, then ↵: applied %q order %q", tab.applied, tab.order)
	}
	// an empty list: nothing to put in
	b, btab, _ := withRecorder(t)
	feed(t, b, "/id > 5<C-r>")
	if es, _ := b.histEntries(btab); len(es) != 0 {
		t.Fatalf("history of a table never run: %+v", es)
	}
	feed(t, b, "<C-e>")
	if btab.where.Text != "id > 5" || btab.typing != "where" {
		t.Errorf("C-e on nothing: input %q", btab.where.Text)
	}
	// the hint of the key, in the list's foot
	feed(t, a, "/<C-r>")
	if f := a.render().String(); !strings.Contains(f, "C-e 填入") {
		t.Errorf("the foot of the list:\n%s", f)
	}
}

// A table's WHERE history keeps its last 100 runs (§9.7).
func TestWhereHistoryRows(t *testing.T) {
	a, tab, _ := withRecorder(t)
	for i := range whereRows + 1 {
		feed(t, a, "/<C-u>id = "+strconv.Itoa(i)+"<CR>")
	}
	if h := a.tableState(tab).History; len(h) != whereRows || h[0].Where != "id = 100" || h[len(h)-1].Where != "id = 1" {
		t.Errorf("%d kept, %q to %q", len(h), h[0].Where, h[len(h)-1].Where)
	}
}

// The palette's recent picks are kept in the state by kind and id (§12, §14).
func TestPaletteRecentInState(t *testing.T) {
	a := sized(160, 45, "nerd")
	feed(t, a, "<C-p>@t_user<CR>")
	if r := a.state.Recent; len(r) != 1 || r[0] != (config.Recent{Kind: "table", ID: "doraemon/public.t_user"}) {
		t.Fatalf("recent %+v", r)
	}
	a.state.Recent = append(a.state.Recent, config.Recent{Kind: "command", ID: "pane.zoom"})
	if names := namesOf(openPaletteOn(t, a)); names[0] != "表:t_user" || names[1] != "命令:缩放 / 还原" {
		t.Errorf("recent first: %v", names[:2])
	}
}

func openPaletteOn(t *testing.T, a *App) *App {
	t.Helper()
	feed(t, a, "<C-p>")
	return a
}

// Names in SQL for their colors: a table the catalog has, its ASCII
// folded unless quoted, not a schema qualifying one; a column of the
// tables its statement names, not of an alias's or a CTE's; the tables whose columns are not fetched yet missing, and
// asked for once, the consoles on screen's (§7.3).
func TestSQLNames(t *testing.T) {
	a := sized(160, 45, "nerd")
	tab := loadOrders(t, a, 3)
	a.sess.Conns[0].Tables = append(a.sess.Conns[0].Tables, db.Table{Schema: "public", Name: "agentable"}, db.Table{Schema: "public", Name: "État"})
	text := `select o.status, "status", "Status", AMOUNT from T_ORDER o; select status from t_user; with x as (select 1) select status from x; table agentable.planner; table ÉTAT`
	names, missing := a.sqlNames(a.sess.Conns[0], text, "public", nil)
	for _, c := range []struct {
		word string
		n    int // the nth of it in text
		want ui.SQLName
	}{
		{"T_ORDER", 0, ui.TableName}, {"t_user", 0, ui.TableName}, {"o", 0, ui.OtherName},
		{"status", 0, ui.ColumnName}, {`"status"`, 0, ui.ColumnName}, {`"Status"`, 0, ui.OtherName}, {"AMOUNT", 0, ui.ColumnName},
		{"status", 2, ui.OtherName}, // t_user's, not fetched
		{"status", 3, ui.OtherName}, // x's, a CTE
		// a schema's name, a table's too, as s of s.t; PG folds ASCII only: État
		{"agentable", 0, ui.OtherName}, {"planner", 0, ui.TableName}, {"ÉTAT", 0, ui.TableName},
	} {
		at := -1
		for range c.n + 1 {
			at += 1 + strings.Index(text[at+1:], c.word)
		}
		if got := names(at, c.word); got != c.want {
			t.Errorf("%s #%d at %d: %v, want %v", c.word, c.n, at, got, c.want)
		}
	}
	if len(missing) != 1 || missing[0].Name != "t_user" {
		t.Errorf("missing %v", missing)
	}
	a.sess.Conns[0].eng = mysql.Engine{} // by any case, `quoted` and the catalog's too
	text = "select `Status`, ÉTAT from `T_ORDER`"
	names, _ = a.sqlNames(a.sess.Conns[0], text, "public", nil)
	if names(7, "`Status`") != ui.ColumnName || names(strings.Index(text, "`T"), "`T_ORDER`") != ui.TableName || names(0, "état") != ui.TableName {
		t.Error("MySQL's names")
	}
	a.sess.Conns[0].eng = postgres.Engine{}
	if n := a.queryBar(a.focused(), tab).Names; n(0, "note") != ui.ColumnName || n(0, "t_user") != ui.TableName {
		t.Error("the WHERE's: the table's columns")
	}
	consoleOf(a.win().pane(2)).ed = editor.New("select * from t_user") // on screen
	if _, cmd := a.Update(tea.WindowSizeMsg{Width: 160, Height: 45}); cmd == nil || !a.sess.Conns[0].colsAsked[tableID{"public", "t_user"}] {
		t.Fatal("the console's t_user: its columns asked for")
	}
	if _, cmd := a.Update(tea.WindowSizeMsg{Width: 160, Height: 45}); cmd != nil {
		t.Error("asked again")
	}
}

// The WHERE input is a vim of one line (F3.39): esc goes to its NORMAL,
// where dd, ciw, u and VISUAL work, a yank flashing, o does nothing and a
// click moves the cursor; ↵ runs it, / opens the history, what is typed then filtering
// it; esc or C-c goes back to the table, the change dropped. [map.normal]
// applies, a grid's maps don't; "+P puts the clipboard (F3.38).
func TestWhereVim(t *testing.T) {
	a := configured(t, 160, 45, "[map.normal]\nQ = \"dd\"\n[map.grid.normal]\nX = \"dd\"\n")
	tab := loadOrders(t, a, 10)
	mode := func() string { r := a.statusLine().Right; return r[len(r)-1].Runs[0].Text }
	feed(t, a, "/id > 5<Esc>")
	if tab.where.Text != "id > 5" || tab.where.Pos != 5 || mode() != " NORMAL " || a.View().Cursor.Shape != tea.CursorBlock {
		t.Fatalf("NORMAL: %+v in %q", tab.where, mode())
	}
	if feed(t, a, "X"); tab.where.Text != "id >5" {
		t.Errorf("X is vim's, not the grid's map: %q", tab.where.Text)
	}
	if feed(t, a, "Q"); tab.where.Text != "" {
		t.Errorf("[map.normal]'s dd: %q", tab.where.Text)
	}
	if feed(t, a, "u0ciwamount<Esc>o"); tab.where.Text != "amount >5" || len(tab.ed.Lines()) != 1 || mode() != " NORMAL " {
		t.Errorf("u ciw o: %q in %q", tab.ed.Lines(), mode())
	}
	r := a.queryBar(a.focused(), tab).InputRect(bodyRect(a.layout()[a.focused().ID]))
	feed(t, a, "0ve")
	if f := a.render(); f.Buf.CellAt(r.Min.X+5, r.Min.Y).Style.Bg != a.theme.Visual || f.Buf.CellAt(r.Min.X+6, r.Min.Y).Style.Bg == a.theme.Visual || mode() != " VISUAL " {
		t.Errorf("VISUAL: amount not selected alone, in %q", mode())
	}
	keys(t, a, "y")
	if st := a.render().Buf.CellAt(r.Min.X, r.Min.Y).Style; st.Bg != a.theme.Yank || mode() != " NORMAL " {
		t.Errorf("y flashes what it took: %+v", st)
	}
	a.Update(flashDone{a.flashSeq})
	if click(a, uv.Pos(r.Min.X+3, r.Min.Y)); tab.where.Pos != 3 || tab.ed.Mode() != editor.Normal {
		t.Errorf("click: %+v in %v", tab.where, tab.ed.Mode())
	}
	if feed(t, a, "/"); tab.hist == nil || tab.ed.Mode() != editor.Insert || a.mode() != keymap.Command {
		t.Fatalf("/: history %v in %v", tab.hist, tab.ed.Mode())
	}
	if feed(t, a, " "); !tab.hist.typed {
		t.Error("typing does not filter the history")
	}
	if feed(t, a, "<BS><Esc><Esc><CR>"); tab.applied != "amount >5" || tab.typing != "" {
		t.Fatalf("↵: applied %q typing %q", tab.applied, tab.typing)
	}
	for _, ks := range []string{"/<Esc>dd<Esc>", "/<Esc>dd<C-c>"} {
		if feed(t, a, ks); tab.typing != "" || tab.where.Text != "amount >5" || tab.ed != nil {
			t.Errorf("%s: typing %q input %q", ks, tab.typing, tab.where.Text)
		}
	}
	feed(t, a, `/<Esc>0"+P`)
	if a.Update(tea.ClipboardMsg{Content: "x\n"}); tab.where.Text != "xamount >5" {
		t.Errorf(`"+P puts the clipboard's line as characters: %q`, tab.where.Text)
	}
}

// The history over the WHERE's vim in any mode (▾, C-r, NORMAL's /) has
// it in INSERT or REPLACE, so what is typed filters the list: VISUAL and
// a command pending end first (F3.39).
func TestWhereHistoryModes(t *testing.T) {
	a, tab, _ := withRecorder(t)
	for _, ks := range []string{"/id > 5<Esc>vl", "/id > 5<Esc>d", "/id > 5<Esc>0R"} {
		feed(t, a, ks)
		a.run("where.history", 0)
		if feed(t, a, "x"); tab.hist == nil || !tab.hist.typed || tab.where.Text != "id > 5x" && tab.where.Text != "xd > 5" {
			t.Errorf("%s: %q in %v", ks, tab.where.Text, tab.ed.Mode())
		}
		feed(t, a, "<Esc><Esc><Esc>") // the list, INSERT, the WHERE
	}
}

// Keys that type nothing open no list in the WHERE: ↵ after an arrow or
// after A runs it; the space ↵ puts after a keyword opens one with nothing
// selected (F8.17); and
// what runs, and is kept, has no blanks around it (§9.7, F8.1, F8.4).
func TestWhereNoListWithoutTyping(t *testing.T) {
	a, tab, _ := withRecorder(t)
	feed(t, a, "/paid is not null<Esc>A<CR>")
	if tab.applied != "paid is not null" || tab.typing != "" {
		t.Errorf("A↵: applied %q typing %q", tab.applied, tab.typing)
	}
	feed(t, a, "/<C-u>id > 5 and paid"+strings.Repeat("<Left>", 4)+"<CR>")
	if tab.applied != "id > 5 and paid" {
		t.Errorf("←↵: applied %q", tab.applied)
	}
	feed(t, a, "/<C-u>id > 5 and<CR>")
	if tab.where.Text != "id > 5 and " || tab.comp == nil || tab.comp.sel >= 0 || tab.typing != "where" {
		t.Errorf("and↵: %q, list %v, typing %q", tab.where.Text, tab.comp != nil, tab.typing)
	}
	if feed(t, a, "<CR>"); tab.applied != "id > 5 and" || tab.comp != nil {
		t.Errorf("and↵↵ runs it: applied %q", tab.applied)
	}
	feed(t, a, "/<C-u>id > 5 and<CR><Tab><CR>") // Tab selects the first column, ↵ takes it
	if tab.where.Text != "id > 5 and "+tab.cols.Cols[0].Name+" " || tab.comp != nil || tab.typing != "where" {
		t.Errorf("and↵ Tab ↵: %q, typing %q", tab.where.Text, tab.typing)
	}
	feed(t, a, "<Esc><Esc>/<C-u>paid<CR><CR>")
	if st := a.tableState(tab); tab.applied != "paid" || st.History[0].Where != "paid" {
		t.Errorf("paid↵↵: applied %q, history %q", tab.applied, st.History[0].Where)
	}
}

// The history list is the history, a rule, then the favorites, the rule
// only when both are there, and a foot of the keys; clicking a row takes
// the entry it is, not the rule's place; the WHERE row shows the key that
// opens it (F8.14).
func TestWhereHistoryLayout(t *testing.T) {
	a, tab, _ := withRecorder(t)
	st := a.tableState(tab)
	at := time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC)
	st.History = []config.Query{{Where: "id > 5", At: at}, {Where: "paid", At: at}}
	feed(t, a, "/<C-r>")
	if f := a.render().String(); !strings.Contains(f, "↵ 应用 · ") {
		t.Errorf("no foot:\n%s", f)
	}
	rule := func() bool {
		v, _, _ := a.histView(a.focused(), tab)
		return slices.ContainsFunc(v.Items, func(i ui.CompleteItem) bool { return i.Sep })
	}
	if rule() {
		t.Error("a rule with nothing under it")
	}
	st.Favorites = []config.Query{{Where: "status = 'done'", At: at}}
	feed(t, a, "<Esc>/<C-r>")
	if es, _ := a.histEntries(tab); len(es) != 3 || es[0].fav || es[1].fav || !es[2].fav || !rule() {
		t.Fatalf("history, favorites, the rule: %+v", es)
	}
	v, _, _ := a.histView(a.focused(), tab)
	if len(v.Items) != 4 || !v.Items[2].Sep || v.Items[3].Text != "status = 'done'" {
		t.Errorf("rows: %+v", v.Items)
	}
	feed(t, a, "<C-p>") // from the first up: the last, past the rule
	if v, _, _ = a.histView(a.focused(), tab); v.Sel != 3 {
		t.Errorf("the selection is the favorite's row: %d", v.Sel)
	}
	es, _ := a.histEntries(tab)
	for row, want := range []int{0, 1, -1, 2} {
		if got := histRow(es, row); got != want {
			t.Errorf("row %d is entry %d, want %d", row, got, want)
		}
	}
	if !strings.Contains(a.render().String(), " "+a.keys.Hint("where.history", "input")+" ▾") {
		t.Error("the key before the ▾")
	}
	// the favorite's row, third entry but fourth row: its entry, not the row's number
	click(a, find(t, a, ui.Target{Kind: ui.KindRow, I: 3}).Min)
	if tab.hist != nil || tab.where.Text != "status = 'done'" {
		t.Errorf("a click on the row after the rule: history %v, where %q", tab.hist, tab.where.Text)
	}
	// favorites only: no rule above them
	st.History = nil
	feed(t, a, "<Esc>/<C-r>")
	if es, _ := a.histEntries(tab); len(es) != 1 || rule() {
		t.Errorf("favorites only: %+v, a rule %v", es, rule())
	}
}

// The history list as it shows: history, a rule, favorites, the keys under
// them; only a history has no rule (F8.14).
func TestGoldenWhereHistory(t *testing.T) {
	at := time.Date(2026, 10, 7, 12, 0, 0, 0, time.Local)
	for name, favs := range map[string][]config.Query{
		"both":    {{Where: "status = 'done'", Order: "amount", Desc: true, Limit: 500, At: at}},
		"history": nil,
	} {
		t.Run(name, func(t *testing.T) {
			a, tab, _ := withRecorder(t)
			st := a.tableState(tab)
			st.History = []config.Query{{Where: "id > 5", At: at}, {Where: "paid is not null and id < 100", At: at}}
			st.Favorites = favs
			feed(t, a, "/<C-r>")
			golden.RequireEqual(t, a.render().String())
		})
	}
}

// A column's values, which the list opens on after its operator, have the
// first selected as ever, ↵ taking it; the column names after and have none
// (§9.7, F8.17).
func TestWhereValuesPreselected(t *testing.T) {
	a, tab, _ := withRecorder(t)
	feed(t, a, "/status = ")
	if tab.comp == nil || tab.comp.sel != 0 {
		t.Fatalf("the values: %+v", tab.comp)
	}
	if feed(t, a, "<CR>"); tab.where.Text != "status = 'pending'" || tab.applied != "" {
		t.Errorf("↵: %q, applied %q", tab.where.Text, tab.applied)
	}
	feed(t, a, "<Esc><Esc>/<C-u>status = '")
	if tab.comp == nil || tab.comp.sel != 0 {
		t.Errorf("after the quote: %+v", tab.comp)
	}
	feed(t, a, "<Esc><Esc>/<C-u>id > 5 and ")
	if tab.comp == nil || tab.comp.sel != -1 {
		t.Errorf("the columns after and: %+v", tab.comp)
	}
}
