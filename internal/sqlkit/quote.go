package sqlkit

import "strings"

// pgReserved are the words PG's grammar does not take as a column's or a
// table's name: pg_get_keywords()'s categories R and T (PG 17).
var pgReserved = map[string]bool{}

func init() {
	for _, w := range strings.Fields(`all analyse analyze and any array as asc asymmetric both case cast check
		collate column constraint create current_catalog current_date current_role current_time
		current_timestamp current_user default deferrable desc distinct do else end except false fetch for
		foreign from grant group having in initially intersect into lateral leading limit localtime
		localtimestamp not null offset on only or order placing primary references returning select
		session_user some symmetric system_user table then to trailing true union unique user using variadic
		when where window with
		authorization binary collation concurrently cross current_schema freeze full ilike inner is isnull
		join left like natural notnull outer overlaps right similar tablesample verbose`) {
		pgReserved[w] = true
	}
}

// NeedsQuote is whether the name has to be quoted to be read as itself in d
// (§9.7, F15.11): one that is not a plain word, one PG folds to lower case, a
// word the dialect reserves.
// ponytail: MySQL's and SQLite's reserved words are not listed: any word of
// the keyword table is quoted, which is harmless where it is not reserved
func (d Dialect) NeedsQuote(name string) bool {
	if name == "" || !identStart(rune(name[0])) {
		return true
	}
	for i := range len(name) {
		if !IdentByte(name[i]) || d == PG && name[i] >= 'A' && name[i] <= 'Z' {
			return true
		}
	}
	if d == PG {
		return pgReserved[name]
	}
	return keywords[strings.ToLower(name)]
}
