package redis

import (
	"errors"
	"strings"

	"github.com/kackerx/sqlmux/internal/i18n"
)

// SplitLine is the words of a line of the console, as redis-cli's sdssplitargs
// reads them (§9.8): blanks part them; inside "…" \n \r \t \b \a \xHH and \"
// mean what they say and any other \c is c; inside '…' only \' is an escape.
// A quote that is not closed, or a closing one with more after it that is no
// blank, is an error and the line is not sent. A line that holds only a #
// comment, or a blank one, has no words.
func SplitLine(line string) ([]string, error) {
	if t := strings.TrimLeft(line, " \t\r"); strings.HasPrefix(t, "#") {
		return nil, nil
	}
	var words []string
	for i := 0; ; {
		for i < len(line) && isBlank(line[i]) {
			i++
		}
		if i >= len(line) {
			return words, nil
		}
		var w []byte
		quote := byte(0) // the quote that is open: " or ', or none
		done := false
		for !done {
			var c byte
			if i < len(line) {
				c = line[i]
			}
			switch {
			case quote == '"':
				switch {
				case c == '\\' && i+3 < len(line) && line[i+1] == 'x' && isHex(line[i+2]) && isHex(line[i+3]):
					w = append(w, unhex(line[i+2])<<4|unhex(line[i+3]))
					i += 3
				case c == '\\' && i+1 < len(line):
					i++
					w = append(w, escapes(line[i]))
				case c == '"':
					if i+1 < len(line) && !isBlank(line[i+1]) { // "a"b
						return nil, errors.New(i18n.T("引号后有字符"))
					}
					done = true
				case i >= len(line):
					return nil, errors.New(i18n.T("引号没有闭合"))
				default:
					w = append(w, c)
				}
			case quote == '\'':
				switch {
				case c == '\\' && i+1 < len(line) && line[i+1] == '\'':
					i++
					w = append(w, '\'')
				case c == '\'':
					if i+1 < len(line) && !isBlank(line[i+1]) {
						return nil, errors.New(i18n.T("引号后有字符"))
					}
					done = true
				case i >= len(line):
					return nil, errors.New(i18n.T("引号没有闭合"))
				default:
					w = append(w, c)
				}
			default:
				switch {
				case i >= len(line) || isBlank(c):
					done = true
				case c == '"' || c == '\'':
					quote = c
				default:
					w = append(w, c)
				}
			}
			if i < len(line) {
				i++
			}
		}
		words = append(words, string(w))
	}
}

func isBlank(c byte) bool { return c == ' ' || c == '\t' || c == '\r' || c == '\n' }

func isHex(c byte) bool {
	return '0' <= c && c <= '9' || 'a' <= c && c <= 'f' || 'A' <= c && c <= 'F'
}

func unhex(c byte) byte {
	switch {
	case c >= 'a':
		return c - 'a' + 10
	case c >= 'A':
		return c - 'A' + 10
	}
	return c - '0'
}

func escapes(c byte) byte {
	switch c {
	case 'n':
		return '\n'
	case 'r':
		return '\r'
	case 't':
		return '\t'
	case 'b':
		return '\b'
	case 'a':
		return '\a'
	}
	return c
}

// Typed reads a line up to the cursor for what completing it needs: the words that
// are finished, and the one being typed, as the line has it (a quote and all), with
// whether it is in a quote still open. A comment is none (§9.8).
func Typed(before string) (words []string, partial string, open bool) {
	if strings.HasPrefix(strings.TrimLeft(before, " \t\r"), "#") {
		return nil, "", false
	}
	start, quote := -1, byte(0) // where the word being read began, and its open quote
	for i := 0; i < len(before); i++ {
		c := before[i]
		switch {
		case quote != 0:
			if c == '\\' && i+1 < len(before) {
				i++
			} else if c == quote {
				quote = 0
			}
		case isBlank(c):
			start = -1
		default:
			if start < 0 {
				start = i
			}
			if c == '"' || c == '\'' {
				quote = c
			}
		}
	}
	if start < 0 {
		start = len(before)
	}
	words, _ = SplitLine(before[:start])
	return words, before[start:], quote != 0
}
