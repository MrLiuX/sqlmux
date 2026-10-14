package redis

import (
	"slices"
	"strings"
	"testing"

	"github.com/kackerx/sqlmux/internal/i18n"
)

// A line is split as redis-cli's sdssplitargs does (§9.8): blanks, the escapes
// of "…", only \' of '…', a quote that is closed and followed by more, one that
// is not closed, a # comment (F16.1, F16.4).
func TestSplitLine(t *testing.T) {
	for _, c := range []struct {
		line string
		want []string
		err  string
	}{
		{"", nil, ""},
		{"   \t ", nil, ""},
		{"# a comment SET a b", nil, ""},
		{"  # indented", nil, ""},
		{"GET key", []string{"GET", "key"}, ""},
		{"  SET   a    b  ", []string{"SET", "a", "b"}, ""},
		{`SET k "a b"`, []string{"SET", "k", "a b"}, ""},
		{`SET k 'a b'`, []string{"SET", "k", "a b"}, ""},
		{`SET k ""`, []string{"SET", "k", ""}, ""},
		{`SET k "a\nb\tc\rd\be\af"`, []string{"SET", "k", "a\nb\tc\rd\be\af"}, ""},
		{`SET k "\x41\x42\xff"`, []string{"SET", "k", "AB\xff"}, ""},
		{`SET k "q\"uote"`, []string{"SET", "k", `q"uote`}, ""},
		{`SET k "\q\\"`, []string{"SET", "k", `q\`}, ""},
		{`SET k '\n \' x'`, []string{"SET", "k", `\n ' x`}, ""},
		{`SET k "\xZZ"`, []string{"SET", "k", "xZZ"}, ""},
		{`SET k "\x4"`, []string{"SET", "k", "x4"}, ""},
		{`SET a"b c"d e`, nil, "引号后有字符"},
		{`SET k "abc"x`, nil, "引号后有字符"},
		{`SET k 'abc'x`, nil, "引号后有字符"},
		{`SET k "abc`, nil, "引号没有闭合"},
		{`SET k 'abc`, nil, "引号没有闭合"},
		{`SET k "abc\`, nil, "引号没有闭合"},
		{`SET k "a" "b"`, []string{"SET", "k", "a", "b"}, ""},
		{`SET k#1 v`, []string{"SET", "k#1", "v"}, ""},
		{"SET k \"日本語\"", []string{"SET", "k", "日本語"}, ""},
	} {
		got, err := SplitLine(c.line)
		switch {
		case c.err != "" && (err == nil || !strings.Contains(err.Error(), i18n.T(c.err))):
			t.Errorf("%q: error %v, want %q", c.line, err, c.err)
		case c.err == "" && (err != nil || !slices.Equal(got, c.want)):
			t.Errorf("%q: %q %v, want %q", c.line, got, err, c.want)
		}
	}
}
