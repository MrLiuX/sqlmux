package sqlkit

import "testing"

func TestLiteral(t *testing.T) {
	for _, c := range []struct {
		in   string
		d    Dialect
		want string
	}{
		{"done", PG, "'done'"},
		{"it's", PG, "'it''s'"},
		{`a\b`, PG, `'a\b'`}, // standard strings: a backslash is a backslash
		{"it's", MySQL, "'it''s'"},
		{`a\b'c`, MySQL, `'a\\b''c'`},
		{"", PG, "''"},
		{"两行\n中文", MySQL, "'两行\n中文'"},
	} {
		if got := Literal(c.in, c.d); got != c.want {
			t.Errorf("%q in %v: %s, want %s", c.in, c.d, got, c.want)
		}
	}
}
