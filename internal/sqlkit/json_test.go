package sqlkit

import (
	"slices"
	"testing"
)

// JSON is cut into keys, strings, numbers, keywords and punctuation; a key
// is a string before a colon, an escaped quote does not end a string, a text
// half typed has what it has (F12.2).
func TestScanJSON(t *testing.T) {
	kinds := map[JSONKind]string{JSONOther: "_", JSONKey: "key", JSONString: "str", JSONNumber: "num", JSONKeyword: "kw", JSONPunct: "p"}
	for in, want := range map[string][]string{
		`{"a": 1, "b": [true, null, "x\"y"], "c": -2.5e3}`: {"p{", `key"a"`, "p:", "num1", "p,", `key"b"`, "p:", "p[", "kwtrue", "p,", "kwnull", "p,", `str"x\"y"`, "p]", "p,", `key"c"`, "p:", "num-2.5e3", "p}"},
		`{"k" : "v"}`:        {"p{", `key"k"`, "p:", `str"v"`, "p}"},
		`[1, 2`:              {"p[", "num1", "p,", "num2"},
		`{"open`:             {"p{", `str"open`},
		`tru`:                {"_tru"},
		`nulls`:              {"_nulls"},
		`@`:                  {"_@"},
		`[false, 1E+2]`:      {"p[", "kwfalse", "p,", "num1E+2", "p]"},
		"{\"k\"\n\t:\r\n1}":  {"p{", `key"k"`, "p:", "num1", "p}"},             // a key and its colon on other lines
		"{\"open\n\"b\": 1}": {"p{", `str"open`, `key"b"`, "p:", "num1", "p}"}, // a quote left open ends with its line
	} {
		var got []string
		for _, tok := range ScanJSON(in) {
			got = append(got, kinds[tok.Kind]+in[tok.Start:tok.End])
		}
		if !slices.Equal(got, want) {
			t.Errorf("%s:\n got %q\nwant %q", in, got, want)
		}
	}
}
