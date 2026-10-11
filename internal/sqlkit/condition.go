package sqlkit

import "strings"

// AddCondition joins cond to the WHERE where with AND (§7.6「按单元值过滤」),
// the way a person would write it: an OR at the top of where needs it in
// parentheses; a comment at the end of the line stays at the end, after cond.
// where is trimmed, as t.applied is. It is where and false when one of the conditions of where, as AND cuts it,
// is cond already, blanks and the case of words aside.
// ponytail: BETWEEN … AND is told by the word before it only; a CASE … END
// with an AND at the top of it, outside parentheses, is cut in two.
func AddCondition(where, cond string, d Dialect) (string, bool) {
	all := Scan(where, d)
	end := len(where) // the code ends where the comments and blanks that close it begin
	for i := len(all) - 1; i >= 0 && (all[i].Kind == Space || all[i].Kind == Comment); i-- {
		end = all[i].Start
	}
	code := strings.TrimRight(where[:end], " \t\r\n")
	tail := where[len(code):]
	if code == "" {
		return strings.TrimRight(cond+" "+tail, " "), true
	}
	ts, depth := words(code, d)
	want := normal(Scan(cond, d), cond)
	hasOr, between, from := false, false, 0
	for i := 0; i <= len(ts); i++ {
		var w string
		if i < len(ts) && depth[i] == 0 && (ts[i].Kind == Ident || ts[i].Kind == Keyword) {
			w = strings.ToLower(code[ts[i].Start:ts[i].End])
		}
		switch {
		case w == "between":
			between = true
		case w == "and" && between:
			between = false
		case w == "or":
			hasOr = true
		case w == "and" || i == len(ts): // a condition ends
			stop := len(code)
			if i < len(ts) {
				stop = ts[i].Start
			}
			if normal(Scan(code[from:stop], d), code[from:stop]) == want {
				return where, false
			}
			if i < len(ts) {
				from = ts[i].End
			}
		}
	}
	if hasOr {
		code = "(" + code + ")"
	}
	return code + " AND " + cond + tail, true
}

// normal is s as its tokens joined by a blank each, the words in lower case:
// what two spellings of one condition share.
func normal(ts []Token, s string) string {
	var out []string
	for _, t := range ts {
		switch t.Kind {
		case Space, Comment:
		case Ident, Keyword:
			out = append(out, strings.ToLower(s[t.Start:t.End]))
		default:
			out = append(out, s[t.Start:t.End])
		}
	}
	return strings.Join(out, " ")
}
