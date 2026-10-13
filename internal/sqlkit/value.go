package sqlkit

import "slices"

// ValueKind is what a value typed at the cursor is for (§11「签名提示与值补全」).
type ValueKind int

const (
	ValueInsert ValueKind = iota + 1 // the Index'th value of a VALUES row: Cols[Index], or the Index'th column of Tables[0], the table inserted into, when Cols is empty
	ValueUpdate                      // UPDATE … SET Cols[0] =
	ValueWhere                       // WHERE, ON, HAVING: Cols[0] = …, Cols[0] in ( …
	ValueCall                        // the Index'th argument of the call Func
)

// Value is what a value typed at the cursor is for. The column of an
// update's or a condition's is Cols[0], with Qualifier, the table or alias it
// is written with, "" when it is not.
type Value struct {
	OK        bool
	Kind      ValueKind
	Tables    []TableRef // those the statement names
	Cols      []string   // quotes off
	Index     int
	Depth     int    // the parentheses the cursor is in
	Prefix    string // the word, number or string typed at the cursor, a string's quote in, from Start; Start is the cursor's own place when there is none
	Start     int
	Begins    bool // nothing of the value is before Prefix: it follows a ( , = <> or !=, where a value begins
	Qualifier string
	Func      TableRef // a call's: the function, Schema.Name
	// Outer is what the call is an argument of, the call around it or the
	// statement's own place: a function the catalog does not know leaves it.
	Outer *Value
}

// callStatements are the statements whose parentheses can be a call's.
var callStatements = map[string]bool{"select": true, "insert": true, "replace": true, "update": true, "delete": true, "with": true, "call": true, "values": true, "table": true}

// ValueContext tells what a value typed at the cursor at pos in text is for:
// the statement it is in by the ; around it, read from its tokens as
// CompletionContext does, and a string is a value's place as well. In a
// comment or a quoted name there is none.
// ponytail: INSERT with no INTO, behind a WITH, ON CONFLICT … SET and
// UPDATE's SET (a, b) = are not read.
func ValueContext(text string, pos int, d Dialect) Value {
	all := Scan(text, d)
	if inside(text, all, pos, d, Comment, Quoted) {
		return Value{}
	}
	ts, depth := statementAt(text, all, pos)
	n := 0 // the tokens that start before the cursor: the word it is at the end of too
	for n < len(ts) && ts[n].Start < pos {
		n++
	}
	w := func(i int) string { return lower(text, ts, i) }
	name := func(i int) bool {
		return i >= 0 && i < len(ts) && (ts[i].Kind == Ident || ts[i].Kind == Quoted || ts[i].Kind == Keyword)
	}
	var open []int // the ( the cursor is in, outermost first
	for i := range n {
		switch w(i) {
		case "(":
			open = append(open, i)
		case ")":
			if len(open) > 0 {
				open = open[:len(open)-1]
			}
		}
	}
	commas := func(p int) (n2 int) { // the commas of the parentheses opening at p
		for j := p + 1; j < n; j++ {
			if depth[j] == depth[p]+1 && w(j) == "," {
				n2++
			}
		}
		return n2
	}
	prefix, start := "", pos
	for _, t := range all {
		if t.Start < pos && pos <= t.End && (t.Kind == Ident || t.Kind == Keyword || t.Kind == Number || t.Kind == String && inside(text, all, pos, d, String)) {
			prefix, start = text[t.Start:pos], t.Start
		}
	}
	begins := false
	for i := n - 1; i >= 0; i-- { // the token before what is typed
		if ts[i].End <= start {
			begins = slices.Contains([]string{"(", ",", "=", "<>", "!="}, w(i))
			break
		}
	}
	var c Completion
	c.readNames(text, ts, depth)
	at, cols, isInsert := insertValues(text, ts, depth)
	var chain []Value // the innermost call first, the statement's own place last
	for k := len(open) - 1; k >= 0 && callStatements[w(0)]; k-- {
		p := open[k]
		if p == 0 || ts[p-1].Kind != Ident && ts[p-1].Kind != Quoted || isInsert && p == at+1 { // MySQL's VALUE (…) is no call
			continue
		}
		f, from := TableRef{Name: unquote(text[ts[p-1].Start:ts[p-1].End])}, p-1
		if w(p-2) == "." && name(p-3) {
			f.Schema, from = unquote(text[ts[p-3].Start:ts[p-3].End]), p-3
		}
		if w(from-1) != "into" && w(from-1) != "table" { // INSERT INTO t (a, b): the columns, not a call
			chain = append(chain, Value{Kind: ValueCall, Func: f, Index: commas(p)})
		}
	}
	if isInsert && len(open) > 0 && at < open[0] {
		chain = append(chain, Value{Kind: ValueInsert, Cols: cols, Index: commas(open[0])})
	}
	if col, qual, ok := setColumn(text, ts, depth, n); ok {
		chain = append(chain, Value{Kind: ValueUpdate, Cols: []string{col}, Qualifier: qual})
	} else if wc := WhereContext(text, pos, d); wc.OK && wc.Column != "" && inCondition(text, ts, depth, n, len(open)) {
		chain = append(chain, Value{Kind: ValueWhere, Cols: []string{wc.Column}, Qualifier: wc.Qualifier})
	}
	for i := range chain {
		chain[i].OK, chain[i].Tables, chain[i].Depth, chain[i].Prefix, chain[i].Start, chain[i].Begins = true, c.Tables, len(open), prefix, start, begins
		if i+1 < len(chain) {
			chain[i].Outer = &chain[i+1]
		}
	}
	if len(chain) == 0 {
		return Value{}
	}
	return chain[0]
}

// insertValues reads "INSERT | REPLACE [IGNORE] INTO name [AS alias] [(cols)]
// VALUES | VALUE" from the start of ts: where VALUES is, and the columns the
// parentheses name, none when they are not there. INSERT … SELECT is not it.
func insertValues(text string, ts []Token, depth []int) (at int, cols []string, ok bool) {
	w := func(i int) string { return lower(text, ts, i) }
	name := func(i int) bool {
		return i < len(ts) && (ts[i].Kind == Ident || ts[i].Kind == Quoted || ts[i].Kind == Keyword)
	}
	if w(0) != "insert" && w(0) != "replace" {
		return 0, nil, false
	}
	i := 1
	if w(i) == "ignore" {
		i++
	}
	if w(i) != "into" || !name(i+1) {
		return 0, nil, false
	}
	i += 2
	if w(i) == "." && name(i+1) {
		i += 2
	}
	if w(i) == "as" && name(i+1) {
		i += 2
	}
	if w(i) == "(" {
		k := i
		for i++; i < len(ts) && depth[i] > depth[k]; i++ {
			if depth[i] == depth[k]+1 && name(i) {
				cols = append(cols, unquote(text[ts[i].Start:ts[i].End]))
			}
		}
		i++
	}
	return i, cols, w(i) == "values" || w(i) == "value"
}

// setColumn reads "UPDATE … SET [X.]col =" from the start of ts: the column
// of the assignment the cursor, with ts[:n] before it, is in, once its = is
// typed.
func setColumn(text string, ts []Token, depth []int, n int) (col, qual string, ok bool) {
	w := func(i int) string { return lower(text, ts, i) }
	name := func(i int) bool {
		return i >= 0 && i < n && (ts[i].Kind == Ident || ts[i].Kind == Quoted || ts[i].Kind == Keyword)
	}
	if !isUpdate(text, ts, 0) {
		return "", "", false
	}
	from := -1 // where the assignment the cursor is in starts
	for i := 1; i < n; i++ {
		if depth[i] != 0 {
			continue
		}
		switch {
		case from < 0 && w(i) == "set":
			from = i + 1
		case from < 0:
		case w(i) == ",":
			from = i + 1
		case w(i) == "where" || w(i) == "from" || w(i) == "returning" || w(i) == "order" || w(i) == "limit":
			return "", "", false
		}
	}
	if !name(from) {
		return "", "", false
	}
	i := from + 1
	if w(i) == "." && name(i+1) {
		qual, from, i = unquote(text[ts[from].Start:ts[from].End]), i+1, i+2
	}
	if i >= n || w(i) != "=" {
		return "", "", false
	}
	return unquote(text[ts[from].Start:ts[from].End]), qual, true
}

// inCondition is whether a condition goes on at the cursor, ts[:n] before it
// and level parentheses open: the clause it is in, back at its level, is
// WHERE, ON or HAVING. A group or an IN list lets it go on in the level
// outside, a call or a subquery's own clause does not.
func inCondition(text string, ts []Token, depth []int, n, level int) bool {
	for j := n - 1; j >= 0; j-- {
		switch w := lower(text, ts, j); {
		case depth[j] > level:
		case depth[j] < level: // the ( the level is in
			if j > 0 && (ts[j-1].Kind == Ident || ts[j-1].Kind == Quoted) {
				return false
			}
			level = depth[j]
		case w == "where" || w == "on" || w == "having":
			return true
		case w == "select" || w == "from" || w == "set" || w == "values" || w == "group" || w == "order" || w == "limit" || w == "offset" || w == "returning" || w == "union" || w == "intersect" || w == "except" || w == "using":
			return false
		}
	}
	return false
}
