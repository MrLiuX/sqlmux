package sqlkit

// Dangerous is whether statement s is one to confirm before it runs (§13
// 「危险语句先确认」): a DELETE or UPDATE with no WHERE at its own level, a
// TRUNCATE, a DROP of anything, an ALTER TABLE that drops a column; an
// EXPLAIN ANALYZE of one of them runs it. Only what the words say: what a
// function or a DO block does, or a WHERE that matches every row, is not
// seen. A guard against slips, not a permission (§13).
// ponytail: an ALTER TABLE … DROP of a form that is not in keeps counts as a
// column's; a new clause of the database's that is none wants its word there.
func Dangerous(s string, d Dialect) bool {
	ts, depth := words(s, d)
	return dangerous(s, ts, depth)
}

func dangerous(s string, ts []Token, depth []int) bool {
	i := first(s, ts)
	switch lower(s, ts, i) {
	case "truncate", "drop":
		return true
	case "delete", "update":
		return noWhere(s, ts, depth, i)
	case "alter":
		return lower(s, ts, i+1) == "table" && dropsColumn(s, ts, i)
	case "explain":
		analyze, j := explainOptions(s, ts[i+1:])
		j += i + 1
		return analyze && j < len(ts) && dangerous(s, ts[j:], depth[j:])
	case "with": // the statement after the CTEs, whose bodies are in parentheses
		for j := i + 1; j < len(ts); j++ {
			if depth[j] != depth[i] {
				continue
			}
			switch lower(s, ts, j) {
			case "delete", "update":
				return noWhere(s, ts, depth, j)
			case "select", "insert", "merge", "values", "table":
				return false
			}
		}
	}
	return false
}

// noWhere is whether the DELETE or UPDATE at ts[i] has no WHERE of its own:
// one in a subquery is deeper, and one past the closing parenthesis it is in
// is another statement's.
func noWhere(s string, ts []Token, depth []int, i int) bool {
	for j := i + 1; j < len(ts) && depth[j] >= depth[i]; j++ {
		if depth[j] == depth[i] && lower(s, ts, j) == "where" {
			return false
		}
	}
	return true
}

// keeps are what follows DROP in an ALTER TABLE that takes no column: a
// constraint, an index, a key, a default, a NOT NULL, a generation, an
// identity.
var keeps = map[string]bool{
	"constraint": true, "index": true, "key": true, "primary": true, "foreign": true, "check": true,
	"default": true, "not": true, "expression": true, "identity": true, "partition": true,
}

// dropsColumn is whether the ALTER TABLE at ts[i] has a DROP COLUMN, or the
// DROP of a column's name that PG takes with no COLUMN; MySQL's DROP
// [COLUMN] of a list too.
func dropsColumn(s string, ts []Token, i int) bool {
	for j := i + 2; j < len(ts); j++ {
		if lower(s, ts, j) != "drop" {
			continue
		}
		if next := lower(s, ts, j+1); next != "" && !keeps[next] {
			return true
		}
	}
	return false
}
