package sqlkit

import (
	"strings"
	"testing"
)

type whereCase struct {
	in          string // | is the cursor
	prefix, col string
	ok          bool
}

func TestWhereContext(t *testing.T) {
	check := func(d Dialect, cases []whereCase) {
		for _, c := range cases {
			pos := len(c.in) - len("|")
			for i := range c.in {
				if c.in[i] == '|' {
					pos = i
				}
			}
			s := c.in[:pos] + c.in[pos+1:]
			w := WhereContext(s, pos, d)
			if w.Prefix != c.prefix || w.Column != c.col || w.OK != c.ok || w.OK && w.Start != pos-len(c.prefix) {
				t.Errorf("%q: %+v, want prefix %q column %q ok %v", c.in, w, c.prefix, c.col, c.ok)
			}
		}
	}
	check(PG, []whereCase{
		{"sta|", "sta", "", true},
		{"id > 5 and st|", "st", "", true},
		{"|", "", "", true},
		{"status = |", "", "status", true},
		{"status = 'do|", "'do", "status", true},
		{"status <> d|", "d", "status", true},
		{"paid = tr|", "tr", "paid", true},
		{"status in (|", "", "status", true},
		{"status IN ('done', 'fa|", "'fa", "status", true},
		{"status in ('done', |", "", "status", true},
		{"status in ('don''t', |", "", "status", true},
		{"note like 'a|", "", "", false}, // a string, but not a column's value
		{"x = 1 -- sta|", "", "", false}, // in a comment
		{`"weird col|`, "", "", false},   // a quoted identifier
		{"a = 1 and b = |", "", "b", true},
		{"f(x) = |", "", "", true}, // not a plain column
		{"id >= |", "", "", true},
		{"名前 = |", "", "名前", true},
	})
	check(MySQL, []whereCase{ // its strings and names
		{`note = 'it\'s' and st|`, "st", "", true},
		{`note = "pa|`, `"pa`, "note", true},
		{"`weird col|", "", "", false},
		{"x = 1 # sta|", "", "", false},
		{`note like "a|`, "", "", false},
	})
}

// After AND or OR and a blank, with nothing typed, a column comes next; an
// empty input, after a word, or right after the and, does not (M8 F8.4).
func TestWhereWantsColumn(t *testing.T) {
	for in, want := range map[string]bool{
		"id > 5 and |":         true,
		"id > 5 OR |":          true,
		"a = 1 and b = 2 or |": true,
		"|":                    false, // ↵ in an empty input clears the filter
		"id > |":               false,
		"id > 5 and|":          false,
		"id > 5 and s|":        false,
		"id > 5 |":             false,
		"a in (1, |":           false,
	} {
		pos := strings.Index(in, "|")
		if got := WhereContext(strings.Replace(in, "|", "", 1), pos, PG).WantsColumn; got != want {
			t.Errorf("%q: %v", in, got)
		}
	}
}
