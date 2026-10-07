package sqlkit

import "strings"

// Where is what the cursor of a WHERE input is at (§9.7): Prefix, from
// Start, is the word or the value being typed that a completion replaces;
// Column is set where a value of that column goes (col =, col <>,
// col in ( … ,). OK is false where nothing completes: in a comment or a
// quoted identifier, or a string outside a value's place. WantsColumn is
// set with nothing typed, after AND or OR and a blank: a column comes next
// (M8 F8.4); an empty input is not it, ↵ there clears the filter.
type Where struct {
	Prefix      string
	Start       int
	Column      string
	OK          bool
	WantsColumn bool
}

// WhereContext reads s, in d, up to its byte offset pos.
func WhereContext(s string, pos int, d Dialect) Where {
	ts := Scan(s[:pos], d)
	w, str := Where{Start: pos, OK: true}, false
	if n := len(ts); n > 0 && ts[n-1].End == pos {
		switch last := ts[n-1]; last.Kind {
		case Ident, Keyword, String, Number:
			w.Prefix, w.Start, ts, str = s[last.Start:pos], last.Start, ts[:n-1], last.Kind == String
		case Comment, Quoted:
			return Where{}
		case Space, Op, Punct: // end the word: nothing to complete
		}
	}
	// the meaningful tokens before the prefix, last first
	var back []Token
	for i := len(ts) - 1; i >= 0; i-- {
		if ts[i].Kind != Space && ts[i].Kind != Comment {
			back = append(back, ts[i])
		}
	}
	text := func(i int) string {
		if i < len(back) {
			return strings.ToLower(s[back[i].Start:back[i].End])
		}
		return ""
	}
	word := func(i int) bool { return i < len(back) && (back[i].Kind == Ident || back[i].Kind == Keyword) }
	w.WantsColumn = w.Prefix == "" && pos > 0 && isBlank(s[pos-1]) && (text(0) == "and" || text(0) == "or")
	switch {
	case (text(0) == "=" || text(0) == "<>" || text(0) == "!=") && word(1):
		w.Column = s[back[1].Start:back[1].End]
	default: // in ( a, b, … : back to the (
		i := 0
		for text(i) == "," && i+1 < len(back) && back[i+1].Kind != Punct && back[i+1].Kind != Op {
			i += 2
		}
		if text(i) == "(" && text(i+1) == "in" && word(i+2) {
			w.Column = s[back[i+2].Start:back[i+2].End]
		}
	}
	if w.Column == "" && str { // a string that is no value of a column
		return Where{}
	}
	return w
}
