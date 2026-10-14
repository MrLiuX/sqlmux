package app

import (
	"image/color"

	"github.com/kackerx/sqlmux/internal/sqlkit"
	"github.com/kackerx/sqlmux/internal/ui"
)

// lang is what the text of a query tab is for its connection (§5「语言缝」):
// the statements it is made of, and what the run needs to know of each.
// sqlLang is SQL's; Redis's is a command a line.
type lang interface {
	Stmts(text string) []sqlkit.Stmt
	// At is the statement of stmts the cursor at offset pos of text is in, -1
	// for none.
	At(text string, stmts []sqlkit.Stmt, pos int) int
	// Colors is the color of each byte of text (§7.3, §9.8); names, for SQL's,
	// tell its tables and columns apart.
	Colors(th *ui.Theme, text string, names ui.SQLNames) []color.Color
	// Writes is whether a read-only connection keeps stmt back, Dangerous
	// whether a run of it is asked about first (§13).
	Writes(stmt string) bool
	Dangerous(stmt string) bool
	// Reruns is whether a request of stmt that was lost after it was sent runs
	// again on the connection dialed anew: when stmt is known to have no effect
	// to repeat (§8.2, §8.7).
	Reruns(stmt string) bool
	// Page is stmt as it asks for the rows from offset, limit of them, and
	// whether it can be asked for again so (§11).
	Page(stmt string, limit, offset int) (text string, pageable bool)
	// Resets is whether what stmt leaves as the schema is not known, DDL
	// whether the catalog is loaded again after it, EndsTx "commit" or
	// "rollback" for a statement that ends the transaction, "" for the rest,
	// NoBegin whether the manual mode begins none before it (§8.6, §5).
	Resets(stmt string) bool
	DDL(stmt string) bool
	EndsTx(stmt string) string
	NoBegin(stmt string) bool
}

// sqlLang is the statements of one SQL dialect.
type sqlLang struct{ d sqlkit.Dialect }

func (l sqlLang) Stmts(text string) []sqlkit.Stmt { return sqlkit.Statements(text, l.d) }
func (l sqlLang) At(text string, stmts []sqlkit.Stmt, pos int) int {
	return sqlkit.StmtAt(text, stmts, pos)
}
func (l sqlLang) Colors(th *ui.Theme, text string, names ui.SQLNames) []color.Color {
	return ui.SQLColors(th, text, l.d, names)
}
func (l sqlLang) Writes(stmt string) bool    { return sqlkit.BreaksReadOnly(stmt, l.d) }
func (l sqlLang) Dangerous(stmt string) bool { return sqlkit.Dangerous(stmt, l.d) }
func (sqlLang) Reruns(string) bool           { return false } // a select may call a function that writes (§8.2)
func (l sqlLang) Page(stmt string, limit, offset int) (string, bool) {
	return sqlkit.AutoLimit(stmt, l.d, limit, offset)
}
func (l sqlLang) Resets(stmt string) bool {
	switch sqlkit.FirstWord(stmt, l.d) {
	case "set", "reset", "discard", "use":
		return true
	}
	return false
}
func (l sqlLang) DDL(stmt string) bool {
	switch sqlkit.FirstWord(stmt, l.d) {
	case "create", "alter", "drop", "rename": // MySQL's RENAME TABLE
		return true
	}
	return false
}
func (l sqlLang) EndsTx(stmt string) string { return sqlkit.EndsTx(stmt, l.d) }
func (l sqlLang) NoBegin(stmt string) bool  { return sqlkit.NoBegin(stmt, l.d) }

// lang is the language of c's queries.
func (c *Conn) lang() lang {
	if c.eng == nil {
		return redisLang{c.cmds}
	}
	return sqlLang{c.eng.Dialect()}
}
