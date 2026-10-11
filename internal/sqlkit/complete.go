package sqlkit

import "slices"

// CompKind is what goes where the cursor is (§9.7).
type CompKind int

const (
	CompAny        CompKind = iota // the statement's tables' columns, tables, keywords
	CompTables                     // after FROM, JOIN, UPDATE, INTO, TABLE, or a comma of FROM's list
	CompColumns                    // after Qualifier.: its columns, or its tables when it is a schema
	CompInsertCols                 // in INSERT INTO t (…): Into's columns not in Listed
)

// TableRef is a table a statement names, as Schema.Name, and the alias it
// gets there; Depth is the parentheses it is in.
type TableRef struct {
	Schema, Name, Alias string
	Depth               int
}

// Completion is what the cursor is at, for completion (§9.7).
type Completion struct {
	OK        bool // a place to complete: not in a string, a comment or a quoted name
	Kind      CompKind
	Prefix    string // the word the cursor is at the end of, maybe ""
	Start     int    // where Prefix starts in the text
	AtStart   bool   // no token of the statement is before Prefix: it begins one (F12.6)
	Qualifier string // the X of X.Prefix, quotes off
	Depth     int    // the parentheses the cursor is in
	Tables    []TableRef
	CTEs      []string
	Into      TableRef // CompInsertCols: the table inserted into
	Listed    []string // CompInsertCols: the columns the parentheses name already, quotes off
	// WantsColumn: nothing is typed, the cursor is after a blank, and a
	// column is what comes next: after SELECT, WHERE, AND, OR, ON, SET,
	// HAVING, BY, or the comma of a select, set or order by list (M8 F8.4)
	WantsColumn bool
}

// CompletionContext tells what the cursor at pos in text is at (§9.7):
// within the statement it is in, by the ; around it, the words before it,
// the tables the statement names with their aliases, and its WITH's names.
// It reads tokens only, after lazysql's components/sql_context.go: no
// grammar, so a guess past what it knows.
func CompletionContext(text string, pos int, d Dialect) Completion {
	all := Scan(text, d)
	for _, t := range all {
		if t.Kind != String && t.Kind != Comment && t.Kind != Quoted || t.Start >= pos || pos > t.End {
			continue
		}
		// at its end the cursor is in it when it is open, what is typed
		// there joining it: a line comment, a string the text ends in
		if pos < t.End || Scan(text[t.Start:t.End]+" ", d)[0].End > t.End-t.Start {
			return Completion{}
		}
	}
	ts, depth := meaningful(text, all)
	// the statement: between the ; before pos and the one after it
	lo, hi := 0, len(ts)
	for i, t := range ts {
		if t.Kind == Punct && text[t.Start] == ';' && depth[i] == 0 {
			if t.End <= pos {
				lo = i + 1
			} else if hi == len(ts) {
				hi = i
			}
		}
	}
	ts, depth = ts[lo:hi], depth[lo:hi]
	c := Completion{OK: true, Start: pos}
	before := len(ts) // the tokens before the word at the cursor: ts[:before]
	for i, t := range ts {
		if t.Start < pos && t.End >= pos && (t.Kind == Ident || t.Kind == Keyword) { // the word the cursor is in, or at the end of
			c.Prefix, c.Start, before = text[t.Start:pos], t.Start, i
			break
		}
		if t.Start >= pos {
			before = i
			break
		}
	}
	c.AtStart = before == 0
	for _, t := range ts[:before] {
		switch text[t.Start:t.End] {
		case "(":
			c.Depth++
		case ")":
			c.Depth = max(c.Depth-1, 0)
		}
	}
	word := func(i int) string { return lower(text, ts, i) }
	// UPDATE names a table at the start of a statement, not after DO (PG's
	// ON CONFLICT DO UPDATE), KEY (MySQL's ON DUPLICATE KEY UPDATE) or FOR
	// (FOR UPDATE, FOR NO KEY UPDATE), where it is a clause
	update := func(i int) bool {
		prev := word(i - 1)
		return word(i) == "update" && prev != "do" && prev != "key" && prev != "for"
	}
	switch p := before - 1; {
	case word(p) == "." && p > 0 && ts[p-1].End == ts[p].Start && ts[p-1].Kind != Punct:
		c.Kind, c.Qualifier = CompColumns, unquote(text[ts[p-1].Start:ts[p-1].End])
	case word(p) == "from" || word(p) == "join" || update(p) || word(p) == "into" || word(p) == "table":
		c.Kind = CompTables
	case word(p) == ",":
		for j := p - 1; j >= 0; j-- { // a FROM list's comma: its clause is FROM
			if depth[j] != depth[p] {
				continue
			}
			// the stop words of a FROM list, but not the ( ) and AS in it: a ) ends a call
			// the list goes on past, an AS is an alias in it
			if w := word(j); fromEnds[w] && w != "(" && w != ")" && w != "as" || w == "select" || w == "from" || w == "by" {
				if w == "from" {
					c.Kind = CompTables
				}
				c.WantsColumn = w == "select" || w == "set" || w == "by"
				break
			}
		}
	default:
		c.WantsColumn = columnFollows[word(p)]
	}
	c.WantsColumn = c.WantsColumn && spaceBefore(all, pos)
	c.readInsert(text, ts, depth, before)
	for i := 0; i < len(ts); i++ {
		switch w := word(i); {
		case w == "from" || w == "join" || update(i) || w == "into":
			i = c.readTables(text, ts, depth, i+1) - 1
		case w == "with" && depth[i] == 0: // the names; the FROMs in them are read on
			c.readCTEs(text, ts, i+1)
		}
	}
	return c
}

// readInsert makes c a CompInsertCols when the cursor, with ts[:before]
// ahead of it, is in the parentheses of "INSERT | REPLACE [IGNORE] INTO
// [schema.]name (", at that level: after the ( or a comma, with the word
// being typed maybe. Another parenthesis (VALUES, SELECT) or one inside the
// list is none.
func (c *Completion) readInsert(text string, ts []Token, depth []int, before int) {
	w := func(i int) string { return lower(text, ts, i) }
	name := func(i int) bool {
		return i < len(ts) && (ts[i].Kind == Ident || ts[i].Kind == Quoted || ts[i].Kind == Keyword)
	} // comment, key: names too
	for i := 1; i < before; i++ {
		if w(i) != "into" || w(i-1) != "insert" && w(i-1) != "replace" && w(i-1) != "ignore" || !name(i+1) {
			continue
		}
		into, k := TableRef{Name: unquote(text[ts[i+1].Start:ts[i+1].End])}, i+2
		if w(k) == "." && name(k+1) {
			into.Schema, into.Name, k = into.Name, unquote(text[ts[k+1].Start:ts[k+1].End]), k+2
		}
		if w(k) != "(" || k >= before || w(before-1) != "(" && w(before-1) != "," {
			continue
		}
		if c.Depth != depth[k]+1 || slices.ContainsFunc(depth[k+1:before], func(d int) bool { return d <= depth[k] }) {
			continue // in an expression of the list, or past its )
		}
		var listed []string
		for j := k + 1; j < len(ts) && depth[j] > depth[k]; j++ {
			if depth[j] == depth[k]+1 && (w(j-1) == "(" || w(j-1) == ",") && (name(j) || ts[j].Kind == Keyword) && !(j == before && c.Prefix != "") {
				listed = append(listed, unquote(text[ts[j].Start:ts[j].End]))
			}
		}
		c.Kind, c.Into, c.Listed = CompInsertCols, into, listed
		return
	}
}

// columnFollows are the words a column is typed after (F8.4).
var columnFollows = map[string]bool{"select": true, "distinct": true, "where": true, "and": true, "or": true, "on": true, "set": true, "having": true, "by": true}

// spaceBefore is whether the cursor at pos is right after white space, as
// the scanner reads it: a column is offered once a blank is typed, not at
// the word before it.
func spaceBefore(ts []Token, pos int) bool {
	return slices.ContainsFunc(ts, func(t Token) bool { return t.Kind == Space && t.Start < pos && pos <= t.End })
}

// fromEnds are the words a FROM list stops at.
var fromEnds = map[string]bool{
	"where": true, "group": true, "order": true, "having": true, "limit": true, "offset": true,
	"union": true, "intersect": true, "except": true, "returning": true, "values": true, "set": true,
	"on": true, "using": true, "join": true, "inner": true, "left": true, "right": true, "cross": true,
	"full": true, "outer": true, "natural": true, "lateral": true, "window": true, "for": true,
	"select": true, "(": true, ")": true, "as": true,
}

// readTables reads the table list from ts[i] on, "schema.name [as] alias"
// by commas, into c.Tables, and returns where it stopped.
func (c *Completion) readTables(text string, ts []Token, depth []int, i int) int {
	w := func(i int) string { return lower(text, ts, i) }
	for i < len(ts) {
		if w(i) == "only" || w(i) == "lateral" { // FROM ONLY t
			i++
		}
		if i >= len(ts) || ts[i].Kind == Punct || ts[i].Kind == Op || fromEnds[w(i)] {
			return i
		}
		ref := TableRef{Name: unquote(text[ts[i].Start:ts[i].End]), Depth: depth[i]}
		i++
		if w(i) == "." && i+1 < len(ts) {
			ref.Schema, ref.Name, i = ref.Name, unquote(text[ts[i+1].Start:ts[i+1].End]), i+2
		}
		if w(i) == "(" { // INTO's columns, or a function FROM calls: no alias, no more
			c.Tables = append(c.Tables, ref)
			return i
		}
		if w(i) == "as" {
			i++
		}
		if i < len(ts) && (ts[i].Kind == Ident || ts[i].Kind == Quoted) {
			ref.Alias, i = unquote(text[ts[i].Start:ts[i].End]), i+1
		}
		c.Tables = append(c.Tables, ref)
		if w(i) != "," {
			return i
		}
		i++
	}
	return i
}

// readCTEs reads WITH's "name [(cols)] AS [[NOT] MATERIALIZED] (…)" by
// commas from ts[i] on into c.CTEs; what is not that ends them, as
// lazysql's readCTEs: timestamp with time zone names no CTE.
func (c *Completion) readCTEs(text string, ts []Token, i int) {
	w := func(i int) string { return lower(text, ts, i) }
	skip := func(i int) int { // past the parentheses opening at i
		for n := 0; i < len(ts); i++ {
			switch w(i) {
			case "(":
				n++
			case ")":
				if n--; n == 0 {
					return i + 1
				}
			}
		}
		return i
	}
	if w(i) == "recursive" {
		i++
	}
	for i < len(ts) && (ts[i].Kind == Ident || ts[i].Kind == Quoted) {
		name := unquote(text[ts[i].Start:ts[i].End])
		if i++; w(i) == "(" {
			i = skip(i)
		}
		if w(i) != "as" {
			return
		}
		if i++; w(i) == "not" {
			i++
		}
		if w(i) == "materialized" {
			i++
		}
		if w(i) != "(" {
			return
		}
		c.CTEs = append(c.CTEs, name)
		if i = skip(i); w(i) != "," {
			return
		}
		i++
	}
}

// unquote is a name without the quotes around it.
func unquote(s string) string {
	if len(s) >= 2 && (s[0] == '"' || s[0] == '`') && s[len(s)-1] == s[0] {
		return s[1 : len(s)-1]
	}
	return s
}
