package sqlkit

import "testing"

// A name is quoted when read bare it would be another: not a word, folded by
// PG, reserved.
func TestNeedsQuote(t *testing.T) {
	for _, c := range []struct {
		name string
		d    Dialect
		want bool
	}{
		{"status", PG, false}, {"t_order2", PG, false}, {"a$b", PG, false}, {"_x", PG, false}, {"中文", PG, false}, {"名前x", PG, false},
		{"Status", PG, true}, {"createdAt", PG, true}, {"a b", PG, true}, {"a-b", PG, true}, {"1a", PG, true}, {"", PG, true}, {"a.b", PG, true}, {`a"b`, PG, true},
		{"user", PG, true}, {"order", PG, true}, {"ORDER", PG, true}, {"left", PG, true}, {"verbose", PG, true}, {"system_user", PG, true},
		{"name", PG, false}, {"type", PG, false}, {"comment", PG, false}, {"key", PG, false}, {"first", PG, false}, {"index", PG, false},
		{"Status", MySQL, false}, {"createdAt", SQLite, false}, {"a b", MySQL, true}, {"1a", SQLite, true},
		{"order", MySQL, true}, {"key", MySQL, true}, {"Order", SQLite, true}, {"status", MySQL, false},
	} {
		if got := c.d.NeedsQuote(c.name); got != c.want {
			t.Errorf("%q in %v: %v", c.name, c.d, got)
		}
	}
}
