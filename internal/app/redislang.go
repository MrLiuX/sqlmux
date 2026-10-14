package app

import (
	"image/color"
	"strconv"
	"strings"

	"github.com/kackerx/sqlmux/internal/db/redis"
	"github.com/kackerx/sqlmux/internal/sqlkit"
	"github.com/kackerx/sqlmux/internal/ui"
)

// redisLang is the console's language (§5「语言缝」, §9.8): a command a line, one
// that is blank or starts with # none; what writes and what is dangerous is what
// the server's own list of commands says, which is nil when it would not.
// ponytail: SELECT n in the console changes the db of that one Main only: the
// title and the tree stay on the db the query is on; pick the db in the title
// to have both
type redisLang struct{ cmds redis.Commands }

func (redisLang) Stmts(text string) (out []sqlkit.Stmt) {
	off := 0
	for line := range strings.SplitSeq(text, "\n") {
		start := len(line) - len(strings.TrimLeft(line, " \t\r"))
		if end := off + len(strings.TrimRight(line, " \t\r")); start < len(strings.TrimRight(line, " \t\r")) && line[start] != '#' {
			out = append(out, sqlkit.Stmt{Start: off + start, End: end, Lead: off + start})
		}
		off += len(line) + 1
	}
	return out
}

// At is the command the cursor at pos is on the line of; -1 on a line with none.
func (redisLang) At(text string, stmts []sqlkit.Stmt, pos int) int {
	from := strings.LastIndexByte(text[:pos], '\n') + 1
	to := len(text)
	if i := strings.IndexByte(text[pos:], '\n'); i >= 0 {
		to = pos + i
	}
	for i, s := range stmts {
		if s.Start >= from && s.Start < to {
			return i
		}
	}
	return -1
}

func (l redisLang) Writes(stmt string) bool    { return l.cmds.Writes(stmt) }
func (l redisLang) Dangerous(stmt string) bool { return l.cmds.Dangerous(stmt) }

// Page: a reply has no cursor to go on from, it is held whole and paged in memory.
func (redisLang) Page(stmt string, _, _ int) (string, bool) { return stmt, false }
func (redisLang) Resets(string) bool                        { return false }
func (redisLang) DDL(string) bool                           { return false }
func (redisLang) EndsTx(string) string                      { return "" }
func (redisLang) NoBegin(string) bool                       { return true }

// Colors is the command names in the keyword's color, the ones the server lists, with
// the subcommand of one that has them (CONFIG GET); quoted strings, numbers and # lines
// in theirs; a name the server did not list, or any when it said none, in none (§9.8).
func (l redisLang) Colors(th *ui.Theme, text string, _ ui.SQLNames) []color.Color {
	out := make([]color.Color, len(text))
	for i := range out {
		out[i] = th.Fg
	}
	paint := func(from, to int, c color.Color) {
		for i := from; i < to; i++ {
			out[i] = c
		}
	}
	off := 0
	for line := range strings.SplitSeq(text, "\n") {
		var first string // the command's name, once it is read
		for i, word := 0, 0; i < len(line); word++ {
			for i < len(line) && isBlank(line[i]) {
				i++
			}
			if i == len(line) {
				break
			}
			start := i
			switch q := line[i]; {
			case q == '#' && word == 0:
				paint(off+i, off+len(line), th.Comment)
				i = len(line)
			case q == '"' || q == '\'':
				for i++; i < len(line) && line[i] != q; i++ {
					if line[i] == '\\' && i+1 < len(line) {
						i++
					}
				}
				i = min(i+1, len(line))
				paint(off+start, off+i, th.SQLString)
			default:
				for i < len(line) && !isBlank(line[i]) {
					i++
				}
				w := strings.ToLower(line[start:i])
				_, listed := l.cmds[w]
				_, sub := l.cmds[first+"|"+w]
				switch {
				case word == 0 && listed:
					first = w
					paint(off+start, off+i, th.Keyword)
				case word == 1 && sub:
					paint(off+start, off+i, th.Keyword)
				default:
					if _, err := strconv.ParseFloat(w, 64); err == nil {
						paint(off+start, off+i, th.Number)
					}
				}
			}
		}
		off += len(line) + 1
	}
	return out
}

func isBlank(c byte) bool { return c == ' ' || c == '\t' || c == '\r' }
