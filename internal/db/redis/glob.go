package redis

import "regexp"

// Match is whether s matches the glob pattern as MATCH of SCAN reads it: * is
// any run, ? any character, [a-c] one of a set, [^a] and [!a] one that is not, and
// \ makes the next character itself.
func Match(pattern, s string) bool {
	re, err := regexp.Compile(globRegexp(pattern))
	return err == nil && re.MatchString(s)
}

// globRegexp is pattern as an anchored regular expression.
func globRegexp(pattern string) string {
	out := []rune(`(?s)^`)
	rs := []rune(pattern)
	for i := 0; i < len(rs); i++ {
		switch c := rs[i]; c {
		case '*':
			out = append(out, '.', '*')
		case '?':
			out = append(out, '.')
		case '\\':
			if i+1 < len(rs) {
				i++
			}
			out = append(out, []rune(regexp.QuoteMeta(string(rs[i])))...)
		case '[':
			j := i + 1
			if j < len(rs) && (rs[j] == '^' || rs[j] == '!') {
				j++
			}
			for j < len(rs) && rs[j] != ']' {
				if rs[j] == '\\' {
					j++
				}
				j++
			}
			if j >= len(rs) { // no ] : a [ of its own
				out = append(out, []rune(`\[`)...)
				continue
			}
			set := []rune{'['}
			k := i + 1
			if rs[k] == '^' || rs[k] == '!' {
				set, k = append(set, '^'), k+1
			}
			for ; k < j; k++ {
				if rs[k] == '\\' && k+1 < j {
					k++
					set = append(set, []rune(regexp.QuoteMeta(string(rs[k])))...)
					continue
				}
				if rs[k] == '[' || rs[k] == ']' || rs[k] == '^' && k > i+1 {
					set = append(set, '\\')
				}
				set = append(set, rs[k])
			}
			out = append(append(out, set...), ']')
			i = j
		default:
			out = append(out, []rune(regexp.QuoteMeta(string(c)))...)
		}
	}
	return string(append(out, '$'))
}
