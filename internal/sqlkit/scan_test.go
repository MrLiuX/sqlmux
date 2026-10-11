package sqlkit

import (
	"slices"
	"strings"
	"testing"
)

// kinds is s's tokens as a string, one letter a kind: _ space, i ident,
// k keyword, q quoted, s string, n number, o op, p punct, c comment.
func kinds(s string, d Dialect) string {
	var b strings.Builder
	for _, t := range Scan(s, d) {
		b.WriteByte("_ikqsnopc"[t.Kind])
	}
	return b.String()
}

func TestScan(t *testing.T) {
	for _, c := range []struct {
		d      Dialect
		in     string
		kinds  string
		tokens []string // the tokens that are not blanks, when set
	}{
		{PG, `a='it''s'--x` + "\n" + `/*c*/"q""q">=1`, "iosc_cqon", nil},
		{PG, "select $$a;b$$, $f$x $$ y$f$, $1", "k_sp_sp_on", []string{"select", "$$a;b$$", ",", "$f$x $$ y$f$", ",", "$", "1"}},
		{PG, "$tag$ open", "s", nil},
		{PG, `E'a\'b' e'\\' 'a\'`, "s_s_s", []string{`E'a\'b'`, `e'\\'`, `'a\'`}}, // \ is just a character in '…'
		{PG, "name'x'", "is", nil}, // no E string inside a word
		{PG, "/* a /* b */ c */x", "ci", nil},
		{PG, "a::int->>'k' #>> '{a}'", "ioios_o_s", nil},
		{PG, "1.5e-3 .5 0x1F 1_000 1e", "n_n_n_n_ni", []string{"1.5e-3", ".5", "0x1F", "1_000", "1", "e"}},
		{PG, "a$b $c", "i_oi", nil},
		{PG, "a -- \"x\"\nb", "i_c_i", nil},
		{MySQL, "`a``b` \"x\\\"y\" 'it''s' # c\nd", "q_s_s_c_i", nil},
		{MySQL, "a--b -- c\n--", "iooi_c_c", []string{"a", "-", "-", "b", "-- c", "--"}}, // a - -b; a -- at the end is one
		{MySQL, "/* a /* b */ c */", "c_i_o", nil},                                       // no nesting: c */ is left
		{MySQL, `"a\'`, "s", nil},
		{PG, "a\u3000b", "i", nil}, // past ASCII, a name's, as scan.l's ident_cont: none may spin
		{PG, "id = \uff11", "i_o_i", nil},
		{PG, "a\u00a0b", "i", nil},
		{PG, "x = \u0663", "i_o_i", nil},
		{PG, "select 1 as ✓$a$; set x = 1; select ✓$a$", "k_n_k_ip_k_i_o_np_k_i", nil}, // no $a$ string swallowing the SET
		{MySQL, "a\u3000b", "i", nil},
		// /*! … */ is code but for its marks; a version is 5 digits, 6 with a blank after
		{MySQL, "SELECT 5 /*! +1 */", "k_n_c_on_c", nil},
		{MySQL, "/*!40101 SET NAMES utf8 */;", "c_k_i_i_cp", []string{"/*!40101", "SET", "NAMES", "utf8", "*/", ";"}},
		{MySQL, "/*!5+1*/ /*!080400 1*/", "cnonc_c_nc", []string{"/*!", "5", "+", "1", "*/", "/*!080400", "1", "*/"}},
		{MySQL, "/*!0804001*/", "cnc", []string{"/*!08040", "01", "*/"}},
		{MySQL, "/*! a /*! b */ 2**/", "c_i_c_i_c_no", nil}, // one in it is code too, the first */ ending both
		{PG, "/*! a */", "c", nil},
	} {
		if got := kinds(c.in, c.d); got != c.kinds {
			t.Errorf("%q: %s, want %s", c.in, got, c.kinds)
		}
		if c.tokens == nil {
			continue
		}
		var got []string
		for _, tk := range Scan(c.in, c.d) {
			if tk.Kind != Space {
				got = append(got, c.in[tk.Start:tk.End])
			}
		}
		if strings.Join(got, "|") != strings.Join(c.tokens, "|") {
			t.Errorf("%q: %q, want %q", c.in, got, c.tokens)
		}
	}
}

func TestScanLines(t *testing.T) {
	var lines []int
	for _, tk := range Scan("a /* x\ny */ b\n'c\nd' e", PG) {
		if tk.Kind != Space {
			lines = append(lines, tk.Line)
		}
	}
	if want := []int{0, 0, 1, 2, 3}; !slices.Equal(lines, want) {
		t.Errorf("lines %v, want %v", lines, want)
	}
}

// A line comment's end is found without copying the rest of the text: a text of
// 4000 comment lines scans in a few dozen allocations, not one a comment, which
// is what made it quadratic (F13.8).
func TestScanLineCommentsDoNotCopy(t *testing.T) {
	for _, d := range []Dialect{PG, MySQL} {
		s := strings.Repeat("select a from t; -- note\n# mysql too\n", 2000)
		if n := testing.AllocsPerRun(3, func() { Scan(s, d) }); n > 100 {
			t.Errorf("dialect %d: %v allocations", d, n)
		}
	}
}
