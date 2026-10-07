package app

import (
	"cmp"
	"math"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	uv "github.com/charmbracelet/ultraviolet"

	"sqlmux/internal/config"
	"sqlmux/internal/db"
	"sqlmux/internal/editor"
	"sqlmux/internal/i18n"
	"sqlmux/internal/keymap"
	"sqlmux/internal/sqlkit"
	"sqlmux/internal/ui"
)

// completion is the candidate list under a WHERE or a quick SQL being
// typed (§9.7, §12).
type completion struct {
	items []candidate
	sel   int // the first as it opens: ↵ takes it (§9.7)
	start int // where the text it replaces starts in the input
}

type candidate struct {
	label, insert, note string
	pos                 []int
	kw                  bool // a keyword, not by what the list says of it
	space               bool // a word, not a value, a function or a list's item, so the next word comes after a space (§9.7)
}

// whereKeywords are what a condition is made of besides columns (§9.7).
var whereKeywords = []string{"and", "or", "not", "is null", "is not null", "in", "like", "ilike", "between", "true", "false", "null"}

// complete finds the candidates for where t's WHERE cursor is: columns and
// keywords while a word is typed, an enum's or a boolean's values where a
// value of that column goes (§9.7).
func (a *App) complete(t *dataTab) {
	t.comp = nil
	in := t.where
	w := sqlkit.WhereContext(in.Text, in.Pos, t.conn.eng.Dialect())
	if !w.OK {
		return
	}
	var groups [][]candidate
	pattern := w.Prefix
	if w.Column != "" {
		var vals []candidate
		for _, c := range t.cols.Cols {
			if !strings.EqualFold(c.Name, w.Column) {
				continue
			}
			for _, e := range c.Enum {
				vals = append(vals, candidate{label: e, insert: "'" + strings.ReplaceAll(e, "'", "''") + "'", note: i18n.T("值")})
			}
			if c.Type == "boolean" || c.Type == "tinyint(1)" { // MySQL's takes true and false too
				vals = append(vals, candidate{label: "true", insert: "true", note: i18n.T("值")}, candidate{label: "false", insert: "false", note: i18n.T("值")})
			}
		}
		groups, pattern = [][]candidate{vals}, strings.TrimLeft(w.Prefix, `'"`) // MySQL's "…" too
	} else {
		if w.Prefix == "" { // nothing typed yet: no list
			return
		}
		var cols, kws []candidate
		for _, c := range t.cols.Cols {
			cols = append(cols, candidate{label: c.Name, insert: c.Name, note: c.Type, space: true})
		}
		for _, k := range whereKeywords {
			if k != "ilike" || t.conn.eng.Dialect() == sqlkit.PG { // PG's alone
				kws = append(kws, candidate{label: k, insert: k, note: i18n.T("关键字"), kw: true, space: sqlkit.SpaceAfter(k)})
			}
		}
		groups = [][]candidate{cols, kws}
	}
	t.comp = ranked(in.Text, in.Pos, pattern, w.Start, groups...)
}

// ranked is the candidates of groups that pattern matches, group by group
// and best first within each (§9.7), completing what starts at start in the
// text, the cursor at pos; nil when none does. One that is just what is
// typed comes first of all, or a table's name would take the place of a
// finished set or to. Pattern's first character matches at a word start,
// fuzzy past it as VS Code's does: evt finds mt_event and tord t_order, but
// x no longer max.
func ranked(text string, pos int, pattern string, start int, groups ...[]candidate) *completion {
	c := &completion{start: start}
	// no smart case: an upper-case letter would make fzf exact about case
	// (§9.7); the palette, tree and COLS keep it
	pattern = strings.ToLower(pattern)
	var exact []candidate
	add := func(cd candidate, from int) {
		if pattern != "" && strings.EqualFold(text[from:pos], cd.insert) {
			exact = append(exact, cd)
		} else {
			c.items = append(c.items, cd)
		}
	}
	for _, g := range groups {
		labels := make([]string, len(g))
		cover := make([]int, len(g)) // where each one replaces from
		for i, cd := range g {
			labels[i], cover[i] = cd.label, c.startOf(text, cd)
		}
		matched := make([]bool, len(g))
		for _, m := range ui.Filter(pattern, labels) {
			cd := g[m.Index]
			if cd.pos = atWordStart(pattern, cd.label, m.Pos); pattern != "" && cd.pos == nil {
				continue
			}
			matched[m.Index] = true
			add(cd, cover[m.Index])
		}
		// a keyword of several words, all typed: a word after a space is no
		// word start, so rename to is not found by the to, and is what is
		// typed (§9.7)
		for i, cd := range g {
			if cover[i] != start && !matched[i] && strings.EqualFold(text[cover[i]:pos], cd.insert) {
				cd.pos = nil
				for j := range []rune(cd.label) {
					cd.pos = append(cd.pos, j)
				}
				add(cd, cover[i])
			}
		}
	}
	if c.items = append(exact, c.items...); len(c.items) == 0 {
		return nil
	}
	return c
}

// startOf is where the text cd replaces starts: the list's, or further back
// when cd is a keyword of several words and its first are written already
// (insert in, the list's word is in): those are taken in, not typed again.
func (c *completion) startOf(text string, cd candidate) int {
	words := strings.Fields(cd.insert)
	if !cd.kw || len(words) < 2 {
		return c.start
	}
	for n := len(words) - 1; n >= 1; n-- {
		s := c.start
		for k := n - 1; k >= 0; k-- {
			e := s
			for e > 0 && (text[e-1] == ' ' || text[e-1] == '\t') {
				e--
			}
			w := words[k]
			if e == s || e < len(w) || !strings.EqualFold(text[e-len(w):e], w) || e > len(w) && sqlkit.IdentByte(text[e-len(w)-1]) {
				s = -1
				break
			}
			s = e - len(w)
		}
		if s >= 0 {
			return s
		}
	}
	return c.start
}

// atWordStart is where pattern matches s with its first character at a
// word start (§9.7), given fzf's best match at pos; nil when it can't.
// fzf's match starts elsewhere when a run of consecutive characters
// outscores the word start, and pos[0] may be a later term's when a space
// splits the pattern (a value typed, 'in p'): the rest is matched again
// from each word start.
func atWordStart(pattern, s string, pos []int) []int {
	rs := []rune(s)
	if len(pos) == 0 || !strings.Contains(pattern, " ") && wordStart(rs, pos[0]) {
		return pos
	}
	first, n := utf8.DecodeRuneInString(pattern)
	for i, r := range rs {
		if !wordStart(rs, i) || unicode.ToLower(r) != first {
			continue
		}
		if ms := ui.Filter(pattern[n:], []string{string(rs[i+1:])}); len(ms) > 0 {
			out := []int{i}
			for _, p := range ms[0].Pos {
				out = append(out, i+1+p)
			}
			return out
		}
	}
	return nil
}

// wordStart reports whether rune i of rs starts a word: the first, one
// after _ . - or $, or an upper case after a lower case.
func wordStart(rs []rune, i int) bool {
	return i == 0 || strings.ContainsRune("_.-$", rs[i-1]) || unicode.IsLower(rs[i-1]) && unicode.IsUpper(rs[i])
}

// table is c's table name of schema, as PG folds what isn't quoted.
func (c *Conn) table(schema, name string) (db.Table, bool) {
	for _, t := range c.Tables {
		if strings.EqualFold(t.Schema, schema) && strings.EqualFold(t.Name, name) {
			return t, true
		}
	}
	return db.Table{}, false
}

// sqlNames tells the names in SQL text on c apart for their colors
// (§7.3): a table is one of c's, any schema's; a column is one of the
// tables its statement names, of schema when unqualified, or with cols
// given, one of those (a WHERE's table's). missing is the statements'
// tables whose columns the cache lacks.
// ponytail: by the tokens alone: a CTE or an alias named as a table is
// colored as one, as is a column; a table's name wins over a column's
func (a *App) sqlNames(c *Conn, text, schema string, cols *db.Columns) (names ui.SQLNames, missing []db.Table) {
	// PG folds a name's ASCII letters unless it is "quoted"
	// (downcase_identifier), its catalog's as they are; MySQL's go by any
	// case on both sides, `quoted` too
	// ponytail: but its tables on Linux, which go by case
	fold := func(s string) string { return s }
	if c.eng.Dialect() == sqlkit.MySQL {
		fold = strings.ToLower
	}
	tables := map[string]bool{}
	for _, t := range c.Tables {
		tables[fold(t.Name)] = true
	}
	type span struct {
		end  int
		cols map[string]bool
	}
	var spans []span // by statement, in order
	add := func(end int, cs db.Columns) {
		if len(spans) == 0 || spans[len(spans)-1].end != end {
			spans = append(spans, span{end, map[string]bool{}})
		}
		for _, c := range cs.Cols {
			spans[len(spans)-1].cols[fold(c.Name)] = true
		}
	}
	var stmts []sqlkit.Stmt
	if cols != nil {
		add(math.MaxInt, *cols)
	} else {
		stmts = sqlkit.Statements(text, c.eng.Dialect())
	}
	for _, st := range stmts {
		add(st.End, db.Columns{})
		cc := sqlkit.CompletionContext(text[st.Start:st.End], 0, c.eng.Dialect())
		for _, r := range cc.Tables {
			if r.Schema == "" && slices.ContainsFunc(cc.CTEs, func(n string) bool { return strings.EqualFold(n, r.Name) }) {
				continue
			}
			t, ok := c.table(cmp.Or(r.Schema, schema), r.Name)
			cs, cached := c.cols[idOf(t)]
			if ok && !cached {
				missing = append(missing, t)
			}
			add(st.End, cs)
		}
	}
	schemas := map[string]bool{}
	for _, s := range c.Schemas {
		schemas[fold(s)] = true
	}
	return func(at int, word string) ui.SQLName {
		name, q := word, word[:1]
		if q == `"` || q == "`" {
			name = strings.ReplaceAll(strings.TrimSuffix(word[1:], q), q+q, q)
		}
		if q != `"` {
			name = strings.Map(func(r rune) rune {
				if 'A' <= r && r <= 'Z' {
					r += 'a' - 'A'
				}
				return r
			}, name)
		}
		name = fold(name)
		i := sort.Search(len(spans), func(i int) bool { return spans[i].end > at })
		switch {
		case schemas[name] && strings.HasPrefix(text[min(at+len(word), len(text)):], "."): // s of s.t: a schema's, whatever table shares its name
			return ui.OtherName
		case tables[name]:
			return ui.TableName
		case i < len(spans) && spans[i].cols[name]:
			return ui.ColumnName
		}
		return ui.OtherName
	}, missing
}

// wantCols asks for the columns of the tables the consoles on screen name
// that the cache lacks, and the palette's previewed view does, for their
// names' colors (§7.3, F4.2): each once, till the cache is dropped.
// ponytail: the consoles are scanned after every message, as they are
// drawn; keep the tables a console names per change if a big file lags
func (a *App) wantCols() tea.Cmd {
	var cmds []tea.Cmd
	want := func(c *Conn, text, schema string) {
		_, missing := a.sqlNames(c, text, schema, nil)
		for _, t := range missing {
			if !c.colsAsked[idOf(t)] {
				c.colsAsked[idOf(t)] = true
				cmds = append(cmds, a.fetchCols(c, t))
			}
		}
	}
	for id := range a.layout() {
		if t := consoleOf(a.win().pane(id)); t != nil {
			want(t.conn, strings.Join(t.ed.Lines(), "\n"), cmp.Or(t.schema, t.conn.Schema))
		}
	}
	if c, text, schema := a.previewedView(); text != "" {
		want(c, text, schema)
	}
	return tea.Batch(cmds...)
}

// sqlComplete is the candidates for the cursor at pos in sql on conn, a
// console's or the quick SQL's (§9.7): by what CompletionContext says goes
// there, tables of schema (the console's; the connection's for the quick
// SQL, §8.6), of
// X. after a schema X, a table's columns
// after its name or alias, else the statement's tables' columns, those at
// the cursor's depth first, the other tables and keywords. It opens with a
// word typed, or right after a qualifier that resolves; manual (C-n) with
// nothing. The columns the cache lacks are fetched, a table once per asked.
// ponytail: names go in bare, as the catalog has them; one that wants
// quotes (upper case, a space) needs them typed.
func (a *App) sqlComplete(conn *Conn, sql string, pos int, schema string, asked map[tableID]bool, manual bool) (*completion, []tea.Cmd) {
	c := sqlkit.CompletionContext(sql, pos, conn.eng.Dialect())
	if !c.OK {
		return nil, nil
	}
	var cmds []tea.Cmd
	table := func(in, name string) (db.Table, bool) { return conn.table(cmp.Or(in, schema), name) }
	columns := func(t db.Table) (out []candidate) {
		cs, ok := conn.cols[idOf(t)]
		if !ok && !asked[idOf(t)] {
			asked[idOf(t)] = true
			cmds = append(cmds, a.fetchCols(conn, t))
		}
		for _, col := range cs.Cols {
			out = append(out, candidate{label: col.Name, insert: col.Name, note: col.Type + " · " + t.Name, space: true})
		}
		return out
	}
	tables := func(in string, skip map[tableID]bool) (out []candidate) {
		for _, t := range conn.Tables {
			if t.Schema == in && !skip[idOf(t)] {
				note := i18n.T("表")
				if t.View() {
					note = i18n.T("视图")
				}
				out = append(out, candidate{label: t.Name, insert: t.Name, note: note, space: true})
			}
		}
		return out
	}
	var ctes []candidate
	isCTE := map[string]bool{}
	for _, n := range c.CTEs {
		ctes, isCTE[strings.ToLower(n)] = append(ctes, candidate{label: n, insert: n, note: "CTE", space: true}), true
	}
	var groups [][]candidate
	resolved := false
	switch c.Kind {
	case sqlkit.CompColumns: // an alias or a table named, a CTE (no columns), a table, a schema
		q := c.Qualifier
		named := func(is func(sqlkit.TableRef) bool) (sqlkit.TableRef, bool) {
			for i := len(c.Tables) - 1; i >= 0; i-- { // at the cursor's depth or out of it, the later first (lazysql's resolveAliases)
				if r := c.Tables[i]; r.Depth <= c.Depth && is(r) {
					return r, true
				}
			}
			return sqlkit.TableRef{}, false
		}
		r, ok := named(func(r sqlkit.TableRef) bool { return strings.EqualFold(r.Alias, q) })
		if !ok {
			r, ok = named(func(r sqlkit.TableRef) bool { return strings.EqualFold(r.Name, q) })
		}
		t, found := table(r.Schema, r.Name)
		switch {
		case ok && r.Schema == "" && isCTE[strings.ToLower(r.Name)], !ok && isCTE[strings.ToLower(q)]:
		case ok && found:
			groups, resolved = [][]candidate{columns(t)}, true
		default: // what the statement names is not a table, or it names none: a table, else a schema (from agentable.)
			if t, ok := table("", q); ok {
				groups, resolved = [][]candidate{columns(t)}, true
			} else if j := slices.IndexFunc(conn.Schemas, func(s string) bool { return strings.EqualFold(s, q) }); j >= 0 {
				groups, resolved = [][]candidate{tables(conn.Schemas[j], nil)}, true
			}
		}
	case sqlkit.CompTables:
		groups = [][]candidate{append(ctes, tables(schema, nil)...)}
	case sqlkit.CompInsertCols: // what the parentheses lack, and all of it at once (§9.7)
		resolved = true
		t, found := table(c.Into.Schema, c.Into.Name)
		if !found {
			break
		}
		columns(t) // fetches them when the cache lacks them
		var rest []candidate
		for _, col := range conn.cols[idOf(t)].Cols {
			if !col.Generated && !slices.ContainsFunc(c.Listed, func(l string) bool { return strings.EqualFold(l, col.Name) }) {
				rest = append(rest, candidate{label: col.Name, insert: col.Name, note: col.Type})
			}
		}
		if len(rest) > 1 && c.Prefix == "" {
			names := make([]string, len(rest))
			for i, r := range rest {
				names[i] = r.label
			}
			all := strings.Join(names, ", ")
			label := all
			if r := []rune(all); len(r) > 40 {
				label = string(r[:39]) + "…"
			}
			rest = append([]candidate{{label: label, insert: all, note: i18n.T("全部 %d 列", len(rest))}}, rest...)
		}
		groups = [][]candidate{rest}
	default:
		var near, far []candidate
		named := map[tableID]bool{}
		for _, r := range c.Tables {
			t, ok := table(r.Schema, r.Name)
			if !ok || named[idOf(t)] || r.Schema == "" && isCTE[strings.ToLower(r.Name)] {
				continue
			}
			named[idOf(t)] = true
			if r.Depth == c.Depth {
				near = append(near, columns(t)...)
			} else {
				far = append(far, columns(t)...)
			}
		}
		var kws []candidate
		for _, k := range sqlkit.Common(conn.eng.Dialect()) {
			kws = append(kws, candidate{label: k, insert: k, note: i18n.T("关键字"), kw: true, space: sqlkit.SpaceAfter(k)})
		}
		groups = [][]candidate{near, far, append(ctes, tables(schema, named)...), kws}
	}
	if c.Prefix == "" && !manual && !resolved {
		return nil, cmds
	}
	return ranked(sql, pos, c.Prefix, c.Start, groups...), cmds
}

// completing is the candidate list that is up, if any: the palette's quick
// SQL's, else the focused console's or the WHERE's being typed.
func (a *App) completing() *completion {
	if a.palette != nil {
		return a.palette.comp
	}
	if t := a.focusedConsole(); t != nil {
		return t.comp
	}
	if t := a.typingTab(); t != nil {
		return t.comp
	}
	return nil
}

// acceptCompletion puts the selected candidate in place of what it
// completes and closes the list. ↵ with the list up only ever does this,
// whether or not the text changes (§9.7, M8 F8.1). A console's change is
// saved as typing is (cmd).
func (a *App) acceptCompletion() tea.Cmd {
	if p := a.palette; p != nil {
		p.comp.accept(&p.input)
		p.comp = nil
		return nil
	}
	if t := a.focusedConsole(); t != nil {
		return a.consoleAccept(t)
	}
	a.whereAccept(a.typingTab())
	return nil
}

// whereAccept is accept in the WHERE's vim, as typing it would (F3.39).
func (a *App) whereAccept(t *dataTab) {
	c, in := t.comp, t.where
	t.comp = nil
	c.plan(in.Text, in.Pos).put(t.ed)
	t.syncWhere()
}

// accepted is what taking a candidate does: ins goes in place of the text
// from start to the cursor, and drop bytes after the cursor go.
type accepted struct {
	start, drop int
	ins         string
}

// plan is what taking the selected candidate does to text with the cursor at
// pos (§9.7): what is typed in the candidate's case stays as it is, a word
// like NULL is not made null; a word gets a space after it, or the cursor
// moves past the whitespace that is there. The closing quote after the
// cursor, autopairs' or typed, goes when the insert has its own (§7.9).
func (c *completion) plan(text string, pos int) accepted {
	cd := c.items[c.sel]
	p := accepted{start: c.startOf(text, cd), ins: cd.insert}
	if typed := text[p.start:pos]; strings.EqualFold(typed, p.ins) {
		p.ins = typed
	}
	after := text[pos:]
	if n := len(p.ins); n > 0 && strings.ContainsRune("'\"`", rune(p.ins[n-1])) && strings.HasPrefix(after, p.ins[n-1:]) {
		p.drop = 1
	}
	if cd.space {
		if after != "" && (after[0] == ' ' || after[0] == '\t') {
			p.ins, p.drop = p.ins+after[:1], 1
		} else {
			p.ins += " "
		}
	}
	return p
}

// put does p in e, an INSERT, as typing it would.
func (p accepted) put(e *editor.Editor) editor.Effect {
	eff := e.Complete(p.start, p.ins)
	for range p.drop {
		eff = e.Feed("<Del>")
	}
	return eff
}

// accept takes the selected candidate into in.
func (c *completion) accept(in *ui.Input) {
	p := c.plan(in.Text, in.Pos)
	in.Text, in.Pos = in.Text[:p.start]+p.ins+in.Text[in.Pos+p.drop:], p.start+len(p.ins)
}

// move moves the selection by d, around the ends (§9.7).
func (c *completion) move(d int) {
	n := len(c.items)
	c.sel = ((c.sel+d)%n + n) % n
}

// startWhere gives the keys to t's WHERE input, its vim in INSERT at the
// end (F3.39); made anew, so its undo starts here and goes with it.
func (a *App) startWhere(t *dataTab) {
	t.typing, t.comp, t.hist = "where", nil, nil
	t.ed = editor.New(t.where.Text)
	t.ed.TabWidth, t.ed.AutoPairs, t.ed.OneLine = a.tabWidth, a.autoPairs, true
	if t.ed.SetPlain(a.plain); a.plain {
		t.ed.Goto(editor.Pos{Col: len(t.where.Text)})
	} else {
		t.ed.Feed("A")
	}
	t.syncWhere()
}

// syncWhere shows the WHERE's vim in the input: its line and cursor.
func (t *dataTab) syncWhere() {
	t.where = ui.Input{Text: t.ed.Lines()[0], Pos: t.ed.Cursor().Col}
}

// whereKey gives key k to the WHERE's vim (F3.39): in INSERT ↵ takes the
// selected candidate when the list is up, else runs the WHERE (§9.7),
// and esc leaves INSERT, the list closing with it, as a console's.
func (a *App) whereKey(t *dataTab, k keymap.Key) tea.Cmd {
	if m := t.ed.Mode(); k == "<CR>" && (m == editor.Insert || m == editor.Replace) {
		if t.comp != nil {
			a.whereAccept(t)
			return nil
		}
		return a.runWhere(t)
	}
	if k == keymap.Esc && t.ed.Plain() { // there is no mode to leave: the list, else the input (§6.9)
		if t.comp != nil {
			t.comp = nil
		} else {
			t.stopTyping()
		}
		return nil
	}
	return a.whereDid(t, t.ed.Feed(string(k)))
}

// whereDid acts on what the WHERE's vim did: the input shows it; in INSERT
// the candidates follow the cursor, or the history list filters by the
// text once typed into (§9.7); the clipboard and a yank's flash as a
// console's. gq has nothing to lay out.
func (a *App) whereDid(t *dataTab, eff editor.Effect) tea.Cmd {
	was := t.where.Text
	t.syncWhere()
	switch {
	case t.hist != nil: // in INSERT or REPLACE, as where.history puts it
		if t.where.Text != was {
			t.hist.typed, t.hist.sel = true, 0
		}
	case t.ed.Mode() == editor.Insert:
		a.complete(t)
	default:
		t.comp = nil
	}
	ed := t.ed
	return tea.Batch(a.vimCmds(eff, func(text string, p editor.ClipPut) tea.Cmd {
		if t.typing != "where" || t.ed != ed { // the edit is over
			return nil
		}
		return a.whereDid(t, ed.PutClip(text, p))
	})...)
}

// whereClick puts the WHERE's cursor where p is on pane pn's input.
// ponytail: the column is the input's, where a tab is one cell, and the
// editor's tabs go to the tabstop: past a pasted tab a click lands short;
// map through the byte offset if tabs ever show up in a WHERE
func (a *App) whereClick(pn *Pane, t *dataTab, p uv.Position) {
	t.ed.Click(0, a.whereCol(pn, t, p))
	t.syncWhere()
	t.comp = nil
	if t.ed.Plain() { // dragging from there selects (§6.9)
		a.dragWhere = pn.ID
	}
}

// whereDrag is the pointer dragged to p with the button down from the WHERE
// of pane id: the selection's far end follows it.
func (a *App) whereDrag(id int, p uv.Position) {
	if pn := a.win().pane(id); pn != nil {
		if t := dataOf(pn); t != nil && t.typing == "where" {
			t.ed.Drag(0, a.whereCol(pn, t, p))
			t.syncWhere()
		}
	}
}

// whereCol is the display column of the WHERE's text that p, a place on
// pane pn's input, is at.
func (a *App) whereCol(pn *Pane, t *dataTab, p uv.Position) int {
	r := a.queryBar(pn, t).InputRect(bodyRect(a.layout()[pn.ID]))
	shown := t.where.Text[:t.where.Start(r.Dx())]
	return ui.Width(ui.Printable(shown)) + p.X - r.Min.X
}

// editPaired is editInput with autopairs, the quick SQL's (§7.9): an
// opening half gets its other, a closing one after the cursor is stepped
// over, and BS takes an empty pair.
func (a *App) editPaired(in *ui.Input, k keymap.Key) bool {
	before, after := in.Text[:in.Pos], in.Text[in.Pos:]
	t := keymap.Text(k)
	switch r, n := utf8.DecodeRuneInString(t); {
	case !a.autoPairs:
	case k == "<BS>" && editor.EmptyPair(before, after):
		in.Text, in.Pos = before[:len(before)-1]+after[1:], in.Pos-1
		return true
	case n > 0 && n == len(t):
		switch shut, skip := editor.Pair(before, after, r); {
		case skip:
			in.Pos += n
			return false
		case shut != "":
			in.Insert(t + shut)
			in.Pos -= len(shut)
			return true
		}
	}
	return editInput(in, k)
}

// whereAt is where pane p's WHERE input starts: lists open under it.
func (a *App) whereAt(p *Pane, t *dataTab) uv.Position {
	return a.queryBar(p, t).InputRect(bodyRect(a.layout()[p.ID])).Min
}

// completeView is c as it opens under the input cell at, where what it
// completes starts.
func (a *App) completeView(c *completion, at uv.Position) (ui.Complete, uv.Rectangle, int) {
	v := ui.Complete{Sel: c.sel}
	w := 20
	for _, cd := range c.items {
		v.Items = append(v.Items, ui.CompleteItem{Text: cd.label, Pos: cd.pos, Note: cd.note})
		w = max(w, ui.Width(cd.label+"  "+cd.note)+4)
	}
	box, rows := ui.CompleteBox(a.window(), at, w, len(v.Items))
	v.Top = max(0, v.Sel-rows+1)
	return v, box, rows
}

// histMenu is the WHERE history and favorites list (Q-02, §9.7), all of
// it as it opens, filtered by the WHERE input once typed into.
type histMenu struct {
	sel   int
	typed bool
}

// histEntry is one of its rows: a favorite, or a history entry.
type histEntry struct {
	q   config.Query
	fav bool
}

// tableState is t's history and favorites, made on first use.
func (a *App) tableState(t *dataTab) *config.TableState {
	key := t.conn.Spec.Name + "/" + t.table.Schema + "." + t.table.Name
	if a.state.Tables == nil {
		a.state.Tables = map[string]*config.TableState{}
	}
	if a.state.Tables[key] == nil {
		a.state.Tables[key] = &config.TableState{}
	}
	return a.state.Tables[key]
}

// histEntries is the favorites, then the history, that the WHERE input
// matches once typed into, newest first in each.
func (a *App) histEntries(t *dataTab) (entries []histEntry, pos [][]int) {
	st, pattern := a.tableState(t), ""
	if t.hist != nil && t.hist.typed {
		pattern = t.where.Text
	}
	for _, g := range []struct {
		qs  []config.Query
		fav bool
	}{{st.Favorites, true}, {st.History, false}} {
		wheres := make([]string, len(g.qs))
		for i, q := range g.qs {
			wheres[i] = q.Where
		}
		ms := ui.Filter(pattern, wheres)
		slices.SortFunc(ms, func(x, y ui.Match) int { return x.Index - y.Index })
		for _, m := range ms {
			entries, pos = append(entries, histEntry{g.qs[m.Index], g.fav}), append(pos, m.Pos)
		}
	}
	return entries, pos
}

// histMove moves the selection by d around the ends; 0 keeps it in the
// list, which got shorter.
func (a *App) histMove(t *dataTab, d int) {
	es, _ := a.histEntries(t)
	if n := len(es); d != 0 && n > 0 {
		t.hist.sel = ((t.hist.sel+d)%n + n) % n
	}
	t.hist.sel = max(min(t.hist.sel, len(es)-1), 0)
}

// histApply runs the selected entry: its WHERE, ORDER and LIMIT, from the
// first page.
func (a *App) histApply(t *dataTab, i int) tea.Cmd {
	es, _ := a.histEntries(t)
	t.hist = nil
	if i >= len(es) {
		return nil
	}
	q := es[i].q
	t.where.Text, t.order, t.desc, t.limit = q.Where, q.Order, q.Desc, max(q.Limit, limits[0])
	return a.runWhere(t)
}

// histStar makes the selected entry a favorite, or no longer one.
func (a *App) histStar(t *dataTab) tea.Cmd {
	es, _ := a.histEntries(t)
	if t.hist.sel >= len(es) {
		return nil
	}
	e, st := es[t.hist.sel], a.tableState(t)
	same := func(q config.Query) bool { return sameQuery(q, e.q) }
	if e.fav {
		st.Favorites = slices.DeleteFunc(st.Favorites, same)
	} else if !slices.ContainsFunc(st.Favorites, same) {
		st.Favorites = slices.Insert(st.Favorites, 0, e.q)
	}
	a.histMove(t, 0)
	return a.saveState()
}

func sameQuery(x, y config.Query) bool {
	return x.Where == y.Where && x.Order == y.Order && x.Desc == y.Desc && x.Limit == y.Limit
}

// whereRows is how many runs a table's WHERE history keeps (§9.7).
const whereRows = 100

// runWhere runs the WHERE typed, from the first page, counting again; a
// condition goes into the history, a repeat moving up to the top (§9.7).
func (a *App) runWhere(t *dataTab) tea.Cmd {
	t.applied = t.where.Text
	t.stopTyping()
	t.pageNo = 0
	fetch := a.fetch(t, true)
	cmds := []tea.Cmd{fetch}
	if strings.TrimSpace(t.applied) != "" && fetch != nil { // not one fetch refuses
		st := a.tableState(t)
		q := config.Query{Where: t.applied, Order: t.order, Desc: t.desc, At: time.Now()}
		if t.limit != limits[0] {
			q.Limit = t.limit
		}
		st.History = slices.Insert(slices.DeleteFunc(st.History, func(h config.Query) bool { return sameQuery(h, q) }), 0, q)
		st.History = st.History[:min(len(st.History), whereRows)]
		cmds = append(cmds, a.saveState())
	}
	return tea.Batch(cmds...)
}

func (a *App) histView(p *Pane, t *dataTab) (ui.Complete, uv.Rectangle, int) {
	es, pos := a.histEntries(t)
	v := ui.Complete{Sel: t.hist.sel}
	w := 40
	for i, e := range es { // an icon each, no group titles (F3.31)
		note, icon := e.q.At.Local().Format("01-02 15:04"), a.icons.History
		if e.fav {
			note, icon = queryNote(e.q), a.icons.Star
			icon.Fg = cmp.Or(icon.Fg, a.theme.Warn)
		}
		v.Items = append(v.Items, ui.CompleteItem{Icon: icon, Text: e.q.Where, Pos: pos[i], Note: note})
		w = max(w, ui.Width(icon.Text+" "+e.q.Where+"  "+note)+4)
	}
	box, rows := ui.CompleteBox(a.window(), a.whereAt(p, t), w, len(v.Items))
	v.Top = max(0, v.Sel-rows+1)
	return v, box, rows
}

// queryNote is what a favorite ran with besides its WHERE, when it isn't
// the default: "status ↓ · 500".
func queryNote(q config.Query) string {
	var parts []string
	if q.Order != "" {
		parts = append(parts, sortedBy(q.Order, q.Desc))
	}
	if q.Limit != 0 {
		parts = append(parts, strconv.Itoa(q.Limit))
	}
	return strings.Join(parts, " · ")
}

// saveState writes the state out (§14): snapshot here, in Update, written
// on the Cmd's goroutine.
func (a *App) saveState() tea.Cmd {
	write := config.Snapshot(a.state)
	return func() tea.Msg {
		if err := write(); err != nil {
			return stateErr{err}
		}
		return nil
	}
}

type stateErr struct{ err error }
