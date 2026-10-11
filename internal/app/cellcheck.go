package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/kackerx/sqlmux/internal/i18n"
	"github.com/kackerx/sqlmux/internal/ui"
)

// intBits are the integer types' sizes, by format_type's names (§10.7).
var intBits = map[string]int{"smallint": 16, "integer": 32, "bigint": 64}

// pgInteger and pgDecimal are integer's and numeric's text past a sign as
// PG 17 reads them: digits with a _ between two, or 0x, 0o and 0b ones;
// numeric's also with a fraction and an exponent.
var (
	pgInteger = regexp.MustCompile(`^(\d+(_\d+)*|0[xX](_?[0-9a-fA-F])+|0[oO](_?[0-7])+|0[bB](_?[01])+)$`)
	pgDecimal = regexp.MustCompile(`^(\d+(_\d+)*(\.(\d+(_\d+)*)?)?|\.\d+(_\d+)*)([eE][+-]?\d+(_\d+)*)?$`)
)

// cellCheck is what is wrong with text as a value of catalog type typ, as
// the edit's hint says it (§10.7); "" when nothing is, or typ goes
// unchecked. Valid is what PG 17's input functions take, spaces around
// it and all; the database has the last word.
func cellCheck(typ, text string) string {
	s := strings.Trim(text, " \t\n\v\f\r") // C's isspace, as PG trims: not U+00A0 nor U+3000
	switch t := baseType(typ); {
	case intBits[t] > 0:
		n := strings.TrimLeft(s, "+-")
		if len(s)-len(n) > 1 || !pgInteger.MatchString(n) {
			return i18n.T("不是有效的整数")
		}
		sign, base, n := s[:len(s)-len(n)], 10, strings.ReplaceAll(n, "_", "")
		if len(n) > 1 && strings.ContainsAny(n[1:2], "xXoObB") { // 010 is ten, as in PG: base 0 only past a prefix
			base = 0
		}
		if _, err := strconv.ParseInt(sign+n, base, intBits[t]); err != nil {
			return i18n.T("超出 int%d 的范围", intBits[t]/8)
		}
	case t == "numeric", t == "real", t == "double precision":
		n := strings.TrimLeft(s, "+-")
		w := strings.ToLower(n)
		special := w == "nan" || w == "inf" || w == "infinity"
		switch sign := s[:len(s)-len(n)]; {
		case len(sign) > 1, t == "numeric" && w == "nan" && sign != "": // numeric's NaN takes no sign, a float's may
			return i18n.T("不是有效的数字")
		case t == "numeric":
			var p, sc int
			switch k, _ := fmt.Sscanf(typ, "numeric(%d,%d)", &p, &sc); {
			case !special && !pgDecimal.MatchString(n) && !pgInteger.MatchString(n):
				return i18n.T("不是有效的数字")
			case k == 2 && w != "nan" && (special || !numericFits(n, p, sc)):
				return i18n.T("超出 %s 的范围", typ)
			}
		case special:
		default: // strtod's: ParseFloat's, but for a hex float's exponent, _ and underflow
			f := s
			mant, _, _ := strings.Cut(w, "e")
			if strings.HasPrefix(w, "0x") { // e a digit, p its exponent, which strtod doesn't need
				var exp bool
				if mant, _, exp = strings.Cut(w[2:], "p"); !exp {
					f += "p0"
				}
			}
			bits := map[string]int{"real": 32, "double precision": 64}[t]
			switch v, err := strconv.ParseFloat(f, bits); {
			case errors.Is(err, strconv.ErrRange), err == nil && v == 0 && strings.Trim(mant, "0.") != "": // so small it is 0: PG's out of range too
				return i18n.T("超出 float%d 的范围", bits/8)
			case err != nil, strings.Contains(s, "_"): // ParseFloat takes 1_000, strtod doesn't
				return i18n.T("不是有效的数字")
			}
		}
	case t == "boolean": // a prefix of one of PG's words, on and off by two letters
		w := strings.ToLower(s)
		for _, word := range []string{"true", "false", "yes", "no", "on", "off", "1", "0"} {
			if strings.HasPrefix(word, w) && (len(w) > 1 || word[0] != 'o') && w != "" {
				return ""
			}
		}
		return i18n.T("不是有效的布尔值")
	case t == "uuid":
		if !validUUID(text) {
			return i18n.T("不是有效的 UUID")
		}
	case t == "json" || t == "jsonb":
		if !json.Valid([]byte(text)) {
			return i18n.T("不是有效的 JSON")
		}
	case timeKind(typ) != ui.NotTime:
		k := timeKind(typ)
		words := []string{"now", "today", "tomorrow", "yesterday", "epoch", "infinity", "-infinity", "+infinity"}
		if k == ui.Time || k == ui.TimeTZ {
			words = []string{"now", "allballs"}
		}
		if !slices.Contains(words, strings.ToLower(s)) && !isoTimeOK(isoTime[k], s) {
			return i18n.T("不是有效的日期 / 时间")
		}
	}
	return ""
}

// numericFits reports whether numeric text n, with no sign, fits
// numeric(p,s): rounded to s places it is under 10^(p-s), its digits
// from the first to the s-th past the point p at most.
func numericFits(n string, p, s int) bool {
	n = strings.ToLower(strings.ReplaceAll(n, "_", ""))
	if len(n) > 1 && strings.ContainsAny(n[1:2], "xob") { // a whole number in another base
		x, _ := new(big.Int).SetString(n, 0)
		n = x.String()
	}
	mant, exp, _ := strings.Cut(n, "e")
	e, _ := strconv.Atoi(exp) // past int's range, its end: clamped below
	whole, frac, _ := strings.Cut(mant, ".")
	digits := strings.TrimLeft(whole+frac, "0")
	keep := len(digits) - len(frac) + max(min(e, 1<<20), -1<<20) + s // the rounded value's digits
	switch {
	case digits == "" || keep < 0: // 0, or rounded to it
		return true
	case keep < len(digits) && digits[keep] >= '5' && strings.Trim(digits[:keep], "9") == "": // rounded up a digit: 9.995 to 10.00
		keep++
	}
	return keep <= p
}

// isoTime is what cellCheck takes as each kind's text besides its words:
// ISO dates and times, wider than the parts a time steps (§10.2), a zone
// on any, which PG drops for a type without one; isoTimeOK checks the
// fields' ranges.
// ponytail: PG takes much more (2026/09/20, month names); add them when
// someone is kept from one
var isoTime = func() map[ui.TimeKind]*regexp.Regexp {
	date := `(?P<y>\d{4,})-(?P<mo>\d{1,2})-(?P<d>\d{1,2})`
	tm := `(?P<h>\d{1,2}):(?P<mi>\d{2})(:(?P<s>\d{2})(?P<f>\.\d+)?)?`
	zone := `( ?([+-](?P<zh>\d{1,2})(:?(?P<zm>\d{2})(:(?P<zsec>\d{2}))?)?|[zZ]))?` // +08:05:43 as PG puts 1900 in Shanghai
	bc := `( (?P<bc>(?i:bc)))?`
	at := `^` + date + `([ Tt]` + tm + zone + `)?` + bc + `$`
	return map[ui.TimeKind]*regexp.Regexp{
		ui.Date: regexp.MustCompile(`^` + date + bc + `$`), ui.Timestamp: regexp.MustCompile(at), ui.TimestampTZ: regexp.MustCompile(at),
		ui.Time: regexp.MustCompile(`^` + tm + zone + `$`), ui.TimeTZ: regexp.MustCompile(`^` + tm + zone + `$`),
	}
}()

// isoTimeOK reports whether s matches re with its fields in range, as PG
// checks them: from year 1, the month's days in the proleptic Gregorian
// calendar (1 BC its year 0, a leap year), 24:00:00 the latest time, a
// leap second, a zone within 15:59:59.
func isoTimeOK(re *regexp.Regexp, s string) bool {
	m := re.FindStringSubmatch(s)
	if m == nil {
		return false
	}
	field := func(name string) string {
		if i := re.SubexpIndex(name); i > 0 {
			return m[i]
		}
		return ""
	}
	num := func(name string) int { n, _ := strconv.Atoi(field(name)); return n } // "" is 0; past int's range, its end
	if y, mo, d := num("y"), num("mo"), num("d"); field("y") != "" {
		ay := y // the astronomical year: 1 BC is 0
		if field("bc") != "" {
			ay = 1 - y
		}
		if y < 1 || mo < 1 || mo > 12 || d < 1 || d > time.Date(ay, time.Month(mo+1), 0, 0, 0, 0, 0, time.UTC).Day() {
			return false
		}
	}
	h, mi, sec := num("h"), num("mi"), num("s")
	return h <= 24 && mi <= 59 && sec <= 60 && (h < 24 || mi == 0 && sec == 0 && strings.Trim(field("f"), ".0") == "") &&
		num("zh") <= 15 && num("zm") <= 59 && num("zsec") <= 59
}

// validUUID is uuid_in's rule: 32 hex digits, a - after any four of them
// but the last, in braces or not; no spaces.
func validUUID(s string) bool {
	if strings.HasPrefix(s, "{") != strings.HasSuffix(s, "}") {
		return false
	}
	s = strings.TrimSuffix(strings.TrimPrefix(s, "{"), "}")
	n := 0
	for i := range len(s) {
		switch c := s[i]; {
		case '0' <= c && c <= '9', 'a' <= c && c <= 'f', 'A' <= c && c <= 'F':
			n++
		case c != '-' || n == 0 || n%4 != 0 || n == 32 || i+1 == len(s) || s[i+1] == '-':
			return false
		}
	}
	return n == 32
}
