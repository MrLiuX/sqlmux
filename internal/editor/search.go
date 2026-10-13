package editor

import (
	"regexp"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/kackerx/sqlmux/internal/i18n"
)

// Search and the : line (§11): patterns are Go's RE2, which reads close to
// vim's \v; ignorecase with smartcase, and searches wrap around the end.

// cmdline is the / ? or : line being typed, drawn on the query's last
// row.
type cmdline struct {
	prompt string // "/", "?" or ":"
	text   string
	pos    int  // the cursor, a byte offset into text
	c      cmd  // the count, and the operator / or ? is the motion of
	back   Mode // the mode to go back to: NORMAL, or VISUAL for a search
}

// CmdLine is the line being typed after / ? or :, and the cursor in it;
// ok is false when none is.
func (e *Editor) CmdLine() (prompt, text string, pos int, ok bool) {
	if e.cl == nil {
		return "", "", 0, false
	}
	return e.cl.prompt, e.cl.text, e.cl.pos, true
}

// openCmdline opens the / ? or : line. : in VISUAL is an operator in vim:
// the cursor goes to the start of the selection, the line to '<,'>; a
// count before : is that many lines from the cursor's (nv_colon).
func (e *Editor) openCmdline(prompt string, c cmd) {
	back := Normal
	switch {
	case prompt != ":":
		if e.visual() {
			back = e.mode
		}
	case e.visual():
		s := e.selection(":")
		e.endVisual()
		e.cur, c.arg = s.start, "'<,'>"
	case c.count > 1:
		c.arg = ".,.+" + strconv.Itoa(c.count-1)
	case c.count == 1:
		c.arg = "."
	}
	e.cl = &cmdline{prompt: prompt, text: c.arg, pos: len(c.arg), c: c, back: back}
	e.mode = Command
}

func (e *Editor) cmdKey(k string) {
	cl := e.cl
	t, p := cl.text, cl.pos
	switch k {
	case "<Esc>":
		e.closeCmdline()
	case "<CR>":
		e.closeCmdline()
		if cl.prompt == ":" {
			e.ex(cl.text)
		} else {
			e.searchCmd(cl)
		}
	case "<BS>", "<C-h>":
		if t == "" {
			e.closeCmdline()
			return
		}
		if p > 0 {
			q := prev(t, p)
			cl.text, cl.pos = t[:q]+t[p:], q
		}
	case "<C-w>", "<M-BS>": // M-BS is C-w in all typing (§7.9)
		q := WordStart(t, p)
		cl.text, cl.pos = t[:q]+t[p:], q
	case "<C-u>":
		cl.text, cl.pos = t[p:], 0
	case "<Left>":
		cl.pos = prev(t, p)
	case "<Right>":
		cl.pos = next(t, p)
	default:
		if s := keyText(k); s != "" {
			cl.text, cl.pos = t[:p]+s+t[p:], p+len(s)
		}
	}
}

// WordStart is where the command line's C-w deletes back to from p in s
// (ex_getln.c): past the blanks before p, then a run of one class; the
// inputs' C-w too (§7.9).
func WordStart(s string, p int) int {
	if p == 0 {
		return 0
	}
	q := prev(s, p)
	for q > 0 && isSpace(rune(s[q])) {
		q = prev(s, q)
	}
	c := classAt(s, q)
	for q > 0 && classAt(s, q) == c {
		q = prev(s, q)
	}
	if classAt(s, q) != c {
		q = next(s, q)
	}
	return q
}

func (e *Editor) closeCmdline() {
	e.mode, e.cl = e.cl.back, nil
}

// search is what n and N repeat: the pattern, whether it ignores case (by
// 'smartcase' when it was typed, always for * and #) and its direction.
type search struct {
	pattern   string
	ignore    bool
	backwards bool
}

var noPattern = i18n.N("没有上一个模式") // vim's E35; shown through i18n.T where it is set

// hasUpper is whether a pattern has an upper case letter for 'smartcase';
// what a backslash escapes does not count: \D is not (search.c
// pat_has_uppercase).
func hasUpper(p string) bool {
	for i := 0; i < len(p); {
		r, n := utf8.DecodeRuneInString(p[i:])
		switch {
		case r == '\\' && i+2 < len(p) && (p[i+1] == '_' || p[i+1] == '%'):
			i += 3
		case r == '\\':
			i += 2
		case unicode.ToLower(r) != r:
			return true
		default:
			i += n
		}
	}
	return false
}

// matcher is a pattern compiled: RE2, and vim's \< and \> at its ends,
// which RE2 has not (* and # put them there).
type matcher struct {
	re       *regexp.Regexp
	bow, eow bool
}

func compile(pattern string, ignore bool) (matcher, error) {
	var m matcher
	pattern, m.bow = strings.CutPrefix(pattern, `\<`)
	if strings.HasSuffix(pattern, `\>`) && backslashes(pattern, len(pattern)-1)%2 == 1 { // not a\\>
		pattern, m.eow = pattern[:len(pattern)-2], true
	}
	re, err := regexp.Compile(pattern) // first without (?i), for the error
	if err == nil && ignore {
		re, err = regexp.Compile("(?i)" + pattern)
	}
	m.re = re
	return m, err
}

// find is m's matches in l, as FindAllStringSubmatchIndex gives them. \<
// and \> are where the character class changes to or from a word class
// (regexp_bt.c BOW, EOW).
func (m matcher) find(l string) [][]int {
	all := m.re.FindAllStringSubmatchIndex(l, -1)
	ms := all[:0]
	for _, x := range all {
		a, b := x[0], x[1]
		if m.bow && (a >= len(l) || classAt(l, a) < 2 || a > 0 && classAt(l, prev(l, a)) == classAt(l, a)) {
			continue
		}
		if m.eow && (b == 0 || classAt(l, prev(l, b)) < 2 || b < len(l) && classAt(l, b) == classAt(l, prev(l, b))) {
			continue
		}
		ms = append(ms, x)
	}
	return ms
}

// searchCmd runs / or ? with what cl holds: the pattern ends at a / (or
// ?) not escaped, and what follows, vim's search offset, is not done. An
// empty pattern is the last one.
func (e *Editor) searchCmd(cl *cmdline) {
	pattern, _ := splitPattern(cl.text, cl.prompt[0])
	s := search{pattern: pattern, ignore: !hasUpper(pattern), backwards: cl.prompt == "?"}
	if s.pattern == "" {
		if e.lastSearch.pattern == "" {
			e.eff.Error = i18n.T(noPattern)
			return
		}
		s.pattern, s.ignore = e.lastSearch.pattern, e.lastSearch.ignore
	}
	e.lastSearch = s
	c := cl.c
	c.name = "n"
	if c.op != "" {
		e.operate(c)
	} else {
		e.run(c)
	}
}

// findNext is n and N (reverse): the last search again, count times.
func (e *Editor) findNext(c cmd, reverse bool) target {
	return e.searchNext(e.cur, c.n(), e.lastSearch.backwards != reverse)
}

// searchNext looks for the last search from p, count times.
func (e *Editor) searchNext(p Pos, count int, back bool) target {
	s := e.lastSearch
	fail := target{to: e.cur}
	e.hl = true
	if s.pattern == "" {
		e.eff.Error = i18n.T(noPattern)
		return fail
	}
	m, err := compile(s.pattern, s.ignore)
	if err != nil {
		e.eff.Error = i18n.T("正则有误：%s", err)
		return fail
	}
	wrapped := false
	for ; count > 0; count-- {
		q, ok := e.searchFrom(m, p, back)
		if !ok {
			e.eff.Error = i18n.T("找不到：%s", s.pattern)
			return fail
		}
		wrapped = wrapped || back && !q.less(p) || !back && !p.less(q)
		p = q
	}
	e.stat = e.statOf(m, p, back, wrapped)
	return target{to: p, ok: true}
}

// searchStat is the line a search leaves under the text: the pattern as it
// was searched, and the count at its right end (F15.9).
type searchStat struct{ left, right string }

// searchMaxCount is how far the count goes: beyond it ">99" (search.c
// SEARCH_STAT_DEF_MAX_COUNT).
const searchMaxCount = 99

// statOf is the line for a match at p: [cur/total], W before it when the
// search wrapped around the end of the text (search.c update_search_stat,
// cmdline_search_stat). The count is of the matches RE2 finds in each line,
// one that overlaps the one before not among them.
func (e *Editor) statOf(m matcher, p Pos, back, wrapped bool) *searchStat {
	cur, total := 0, 0
count:
	for n, l := range e.lines {
		for _, x := range m.find(l) {
			col := x[0]
			if col >= len(l) {
				col = last(l)
			}
			if total++; !p.less(Pos{n, col}) { // the matches up to p
				cur = total
			}
			if total > searchMaxCount {
				break count
			}
		}
	}
	over := func(n int) string {
		if n > searchMaxCount {
			return ">" + strconv.Itoa(searchMaxCount)
		}
		return strconv.Itoa(n)
	}
	st := &searchStat{left: "/" + e.lastSearch.pattern, right: "[" + over(cur) + "/" + over(total) + "]"}
	if back {
		st.left = "?" + e.lastSearch.pattern
	}
	if wrapped {
		st.right = "W " + st.right
	}
	return st
}

// Stat is the line left under the text by a search, n or N, until the next
// key: the pattern as / or ? would show it, and the count at its end; ok is
// false when there is none (F15.9).
func (e *Editor) Stat() (left, right string, ok bool) {
	if e.stat == nil {
		return "", "", false
	}
	return e.stat.left, e.stat.right, true
}

// Hit is a match of the last search to light, bytes [From, To) of Line;
// Current is the one the cursor is in (F15.9).
type Hit struct {
	Line, From, To int
	Current        bool
}

// Hits are the matches of the last search on the lines in view, while they
// are lit: from a search until esc in NORMAL.
func (e *Editor) Hits() []Hit {
	if !e.hl || e.lastSearch.pattern == "" || e.height <= 0 {
		return nil
	}
	m, err := compile(e.lastSearch.pattern, e.lastSearch.ignore)
	if err != nil {
		return nil
	}
	var hits []Hit
	for n := e.top; n < min(e.top+e.height, len(e.lines)); n++ {
		for _, x := range m.find(e.lines[n]) {
			if x[1] > x[0] {
				hits = append(hits, Hit{n, x[0], x[1], e.cur.Line == n && e.cur.Col >= x[0] && e.cur.Col < x[1]})
			}
		}
	}
	return hits
}

// searchFrom finds the next match of m after p (before it, back),
// wrapping around the end of the text. A match at the end of a line (/$)
// counts as on its last character (search.c searchit).
func (e *Editor) searchFrom(m matcher, p Pos, back bool) (Pos, bool) {
	n := len(e.lines)
	for i := 0; i <= n; i++ {
		line := p.Line + i
		if back {
			line = p.Line - i
		}
		line = (line%n + n) % n
		l := e.lines[line]
		// on the cursor's line only past the cursor, and back on it after
		// going all the way round only up to it
		ok := func(col int) bool {
			switch {
			case i == 0 && back:
				return col < p.Col
			case i == 0:
				return col > p.Col
			case i == n && back:
				return col >= p.Col
			case i == n:
				return col <= p.Col
			}
			return true
		}
		ms := m.find(l)
		for j := range ms {
			if back {
				j = len(ms) - 1 - j
			}
			col := ms[j][0]
			if col >= len(l) {
				col = last(l)
			}
			if ok(col) {
				return Pos{line, col}, true
			}
		}
	}
	return p, false
}

// star is * and # (normal.c nv_ident, find_ident_at_pos): the keyword
// under or after the cursor as a whole word, else the run of non-blanks
// there, searched from its start; case is ignored, not smart. A keyword is
// a run of one word class, so 名字 in a名字 is not one.
func star(back bool) motion {
	return func(e *Editor, c cmd, _ string) target {
		l := e.line()
		start := e.cur.Col
		for start < len(l) && classAt(l, start) < 2 {
			start = next(l, start)
		}
		word := start < len(l)
		if !word { // any non-blank
			start = e.cur.Col
			for start < len(l) && classAt(l, start) == 0 {
				start = next(l, start)
			}
		}
		if start >= len(l) {
			return target{to: e.cur}
		}
		cls := classAt(l, start)
		for start > 0 && classAt(l, prev(l, start)) == cls {
			start = prev(l, start)
		}
		end := start
		for end < len(l) && (word && classAt(l, end) == cls || !word && classAt(l, end) != 0) {
			end = next(l, end)
		}
		pattern := regexp.QuoteMeta(l[start:end])
		if classAt(l, start) >= 2 {
			pattern = `\<` + pattern
		}
		if classAt(l, prev(l, end)) >= 2 {
			pattern += `\>`
		}
		e.lastSearch = search{pattern: pattern, ignore: true, backwards: back}
		return e.searchNext(Pos{e.cur.Line, start}, c.n(), back)
	}
}

// badRange is vim's E16, a range past the text.
var badRange = i18n.N("范围无效") // shown through i18n.T where it is set

// ex runs a : line: {n} goes to line n and s substitutes; the rest is the
// query's (:w, :q), handed on as Effect.Ex. Lines of the text are from 0
// here, so line 0 of an ex range is -1.
func (e *Editor) ex(line string) {
	line = strings.TrimSpace(line)
	from, to, given, rest, err := e.lineRange(line)
	last := len(e.lines) - 1
	switch {
	case err != "":
		e.eff.Error = err
	case rest == "" && given: // :{n}: past the end is the last line (ex_docmd.c)
		if to < -1 {
			e.eff.Error = i18n.T(badRange)
			return
		}
		e.cur.Line = min(max(to, 0), last)
		e.cur.Col = e.coladvance(e.cur.Line, e.want)
	case len(rest) >= 2 && rest[0] == 's' && strings.IndexByte(`\"| `, rest[1]) < 0 &&
		!unicode.IsLetter(rune(rest[1])) && !unicode.IsDigit(rune(rest[1])):
		if from < -1 || to > last { // invalid_range; line 0 is line 1 (correct_range)
			e.eff.Error = i18n.T(badRange)
			return
		}
		if e.ReadOnly {
			e.eff.ReadOnly = true
			return
		}
		e.substitute(max(from, 0), max(to, 0), rest[1:])
	case line != "":
		e.eff.Ex = line
	}
}

// lineRange reads the lines a : command is for (ex_docmd.c
// parse_cmd_range): % or addresses split by commas, each ., $, a number,
// '< or '>, with +n or -n after; one left out is the cursor's line. given
// is whether s has one; a backward range is turned round. err is what went
// wrong.
func (e *Editor) lineRange(s string) (from, to int, given bool, rest, err string) {
	if r, ok := strings.CutPrefix(s, "%"); ok {
		return 0, len(e.lines) - 1, true, r, ""
	}
	to = e.cur.Line
	for n := 0; ; n++ {
		from, to = to, e.cur.Line
		line, r, ok := e.address(s)
		if !ok {
			return 0, 0, false, "", i18n.T("没有选区") // vim's E20: '< and '> not set
		}
		if r != s {
			to, s, given = line, r, true
		}
		if n == 0 {
			from = to
		}
		if s, ok = strings.CutPrefix(s, ","); !ok {
			break
		}
		given = true
	}
	return min(from, to), max(from, to), given, s, ""
}

// address reads one line address off s; rest is s when there is none. ok
// is false for '< or '> with no VISUAL yet.
func (e *Editor) address(s string) (n int, rest string, ok bool) {
	number := func(s string) (int, string) {
		d := len(s) - len(strings.TrimLeft(s, "0123456789"))
		v, _ := strconv.Atoi(s[:d])
		return v, s[d:]
	}
	v := e.lastVisual
	switch {
	case s == "":
		return 0, s, true
	case s[0] == '.':
		n, s = e.cur.Line, s[1:]
	case s[0] == '$':
		n, s = len(e.lines)-1, s[1:]
	case strings.HasPrefix(s, "'<"), strings.HasPrefix(s, "'>"):
		if v.mode == Normal {
			return 0, s, false
		}
		n = min(v.start.Line, v.end.Line)
		if s[1] == '>' {
			n = max(v.start.Line, v.end.Line)
		}
		s = s[2:]
	case s[0] >= '0' && s[0] <= '9':
		n, s = number(s)
		n--
	case s[0] == '+' || s[0] == '-':
		n = e.cur.Line
	default:
		return 0, s, true
	}
	for len(s) > 0 && (s[0] == '+' || s[0] == '-') {
		sign := 1 - 2*strings.IndexByte("+-", s[0])
		d, r := number(s[1:])
		if r == s[1:] {
			d = 1
		}
		n, s = n+sign*d, r
	}
	return n, s, true
}

// substitute is :s/pattern/replacement/flags over lines from to to: g for
// every match on a line, i to ignore case; & \1 to \9, \t and \r (a line
// break) in the replacement as in vim. The cursor ends on the last line
// changed; u and U come back to the start of the first (ex_cmds.c do_sub).
func (e *Editor) substitute(from, to int, arg string) {
	delim := arg[0]
	pattern, rest := splitPattern(arg[1:], delim)
	repl, flags := splitReplacement(rest, delim)
	ignore := !hasUpper(pattern)
	if pattern == "" {
		if e.lastSearch.pattern == "" {
			e.eff.Error = i18n.T(noPattern)
			return
		}
		pattern, ignore = e.lastSearch.pattern, e.lastSearch.ignore
	}
	e.lastSearch, e.hl = search{pattern: pattern, ignore: ignore, backwards: e.lastSearch.backwards}, true
	m, err := compile(pattern, ignore || strings.Contains(flags, "i"))
	if err != nil {
		e.eff.Error = i18n.T("正则有误：%s", err)
		return
	}
	template := replacement(repl)
	all, changed := strings.Contains(flags, "g"), -1
	for n := from; n <= to; n++ {
		l := e.lines[n]
		ms := m.find(l)
		if len(ms) > 1 && ms[len(ms)-1][0] == len(l) && ms[len(ms)-1][1] == len(l) {
			ms = ms[:len(ms)-1] // no empty match at the end after another (:s/a*/x/g)
		}
		if len(ms) == 0 {
			continue
		}
		if !all {
			ms = ms[:1]
		}
		var b []byte
		last := 0
		for _, x := range ms {
			b = append(b, l[last:x[0]]...)
			b = m.re.ExpandString(b, template, l, x)
			last = x[1]
		}
		if changed < 0 {
			// ponytail: after a \r U puts back this line; nvim's do_sub keeps
			// the line it split last for U. Follow it if that is missed.
			e.cur = Pos{n, 0}
			e.saveLine(n, l, e.cur)
		}
		parts := strings.Split(string(b)+l[last:], "\n") // \r broke the line
		e.setLine(n, parts[0])
		e.insertLines(n+1, parts[1:]...)
		n, to = n+len(parts)-1, to+len(parts)-1
		changed = n
	}
	if changed < 0 {
		e.eff.Error = i18n.T("找不到：%s", pattern)
		return
	}
	e.cur = Pos{changed, firstNonBlank(e.lines[changed])}
	e.want = wantUnset
	e.endChange() // U keeps the first line, whatever the lines after did
}

// splitPattern cuts s where a pattern delimited by delim ends (regexp.c
// skip_regexp): at the first delim not escaped nor in […]. A backslash
// stays: to RE2 \/ is / as it is to vim.
func splitPattern(s string, delim byte) (pattern, rest string) {
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case delim:
			return s[:i], s[i+1:]
		case '\\':
			i++
		case '[': // skip_anyof
			j := i + 1
			if j < len(s) && s[j] == '^' {
				j++
			}
			if j < len(s) && (s[j] == ']' || s[j] == '-') {
				j++
			}
			for j < len(s) && s[j] != ']' {
				switch {
				case s[j] == '\\':
					j++
				case strings.HasPrefix(s[j:], "[:"):
					if k := strings.Index(s[j:], ":]"); k > 0 {
						j += k + 1
					}
				}
				j++
			}
			if j >= len(s) {
				return s, ""
			}
			i = j
		}
	}
	return s, ""
}

// splitReplacement cuts s at the first delim not after a backslash.
func splitReplacement(s string, delim byte) (repl, rest string) {
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case delim:
			return s[:i], s[i+1:]
		case '\\':
			i++
		}
	}
	return s, ""
}

// replacement turns vim's replacement into regexp.Expand's template: & is
// the match, \1 a group, \t a tab, \r a line break, \& and \\ the
// characters themselves.
func replacement(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		switch c := s[i]; {
		case c == '&':
			b.WriteString("${0}")
		case c == '$':
			b.WriteString("$$")
		case c == '\\' && i+1 < len(s):
			i++
			switch d := s[i]; {
			case d >= '0' && d <= '9':
				b.WriteString("${" + string(d) + "}")
			case d == 't':
				b.WriteByte('\t')
			case d == 'r':
				b.WriteByte('\n')
			default:
				b.WriteByte(d)
			}
		default:
			b.WriteByte(c)
		}
	}
	return b.String()
}
