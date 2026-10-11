package sqlkit

// JSONKind is what a piece of JSON text is to a colorer (F12.2).
type JSONKind uint8

const (
	JSONOther   JSONKind = iota // what is no JSON, a space too
	JSONKey                     // a string that is followed by a colon
	JSONString                  // any other string
	JSONNumber                  // 12, -0.5, 1e9
	JSONKeyword                 // true, false, null
	JSONPunct                   // { } [ ] , :
)

// JSONToken is the text from Start to End, not in, of one JSONKind.
type JSONToken struct {
	Kind       JSONKind
	Start, End int
}

// ScanJSON cuts text into tokens for coloring: it reads what is there and
// does not check it, so a value half typed has its colors as far as it goes.
// Spaces are tokens of no kind.
func ScanJSON(text string) []JSONToken {
	var out []JSONToken
	for i := 0; i < len(text); {
		c := text[i]
		start := i
		kind := JSONOther
		switch {
		case c == '"':
			i++
			for i < len(text) && text[i] != '"' {
				if text[i] == '\\' {
					i++
				}
				i++
			}
			i = min(i+1, len(text)) // the closing quote
			kind = JSONString
			if j := skipSpace(text, i); j < len(text) && text[j] == ':' {
				kind = JSONKey
			}
		case c == '-' || c >= '0' && c <= '9':
			for i < len(text) && (text[i] >= '0' && text[i] <= '9' || text[i] == '-' || text[i] == '+' || text[i] == '.' || text[i] == 'e' || text[i] == 'E') {
				i++
			}
			kind = JSONNumber
		case c >= 'a' && c <= 'z':
			for i < len(text) && text[i] >= 'a' && text[i] <= 'z' {
				i++
			}
			if w := text[start:i]; w == "true" || w == "false" || w == "null" {
				kind = JSONKeyword
			}
		case c == '{' || c == '}' || c == '[' || c == ']' || c == ',' || c == ':':
			i++
			kind = JSONPunct
		default:
			i++
		}
		out = append(out, JSONToken{kind, start, i})
	}
	return out
}

func skipSpace(s string, i int) int {
	for i < len(s) && (s[i] == ' ' || s[i] == '\t' || s[i] == '\n' || s[i] == '\r') {
		i++
	}
	return i
}
