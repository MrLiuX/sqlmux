package redis

import (
	"strings"
	"testing"
	"time"

	"github.com/kackerx/sqlmux/internal/i18n"
)

// MATCH's glob: * ? [a-c] [^a] \x, and a [ with no ] is itself (F16.3).
func TestMatch(t *testing.T) {
	for _, c := range []struct {
		pattern, s string
		want       bool
	}{
		{"", "", true}, {"", "a", false}, {"*", "", true}, {"*", "anything:at all", true},
		{"a*", "abc", true}, {"a*", "bac", false}, {"*c", "abc", true}, {"a*c", "ac", true}, {"a*c", "abd", false},
		{"a?c", "abc", true}, {"a?c", "ac", false}, {"a?c", "abbc", false},
		{"[abc]x", "bx", true}, {"[abc]x", "dx", false}, {"[a-c]x", "bx", true}, {"[a-c]x", "dx", false},
		{"[^a]x", "bx", true}, {"[^a]x", "ax", false}, {"[!a]x", "bx", false}, {"[!a]x", "ax", true}, {"[!a]x", "!x", true},
		{`a\*b`, "a*b", true}, {`a\*b`, "axb", false}, {`a\\b`, `a\b`, true}, {`\[x`, "[x", true},
		{"[x", "[x", true}, {"a.b", "a.b", true}, {"a.b", "axb", false}, {"a(b)", "a(b)", true},
		{"日*", "日本", true}, {"a*\n", "ab\n", true}, {"a*", "ab\ncd", true},
		{"[a\\]]x", "]x", true},
	} {
		if got := Match(c.pattern, c.s); got != c.want {
			t.Errorf("Match(%q, %q) = %v", c.pattern, c.s, got)
		}
	}
}

// A time to live as a person reads it, and a key with none (F16.3).
func TestHumanTTL(t *testing.T) {
	for d, want := range map[time.Duration]string{
		-1: i18n.T("不过期"), -2: "-", 0: "0ms", 500 * time.Millisecond: "500ms", 59 * time.Second: "59s", 90 * time.Second: "1m30s", 12*time.Minute + 3*time.Second: "12m03s",
		time.Hour + 100*time.Second: "1h01m40s", 26*time.Hour + 5*time.Minute: "1d02h05m", 3650 * 24 * time.Hour: "3650d00h00m",
	} {
		if got := HumanTTL(d); got != want {
			t.Errorf("HumanTTL(%v) = %q, want %q", d, got, want)
		}
	}
	for n, want := range map[int64]string{0: "0B", 1023: "1023B", 1024: "1KB", 1536: "1.5KB", 1 << 20: "1MB", 5 << 30: "5GB"} {
		if got := humanBytes(n); got != want {
			t.Errorf("humanBytes(%d) = %q, want %q", n, got, want)
		}
	}
}

// A word is bare unless a line would split or quote it, and quoted it reads
// back as it was (F16.3).
func TestQuote(t *testing.T) {
	for in, want := range map[string]string{
		"plain": "plain", "": `""`, "two words": `"two words"`, `say "hi"`: `"say \"hi\""`, "a\nb\tc": `"a\nb\tc"`, `back\slash`: `"back\\slash"`,
		"#tag": `"#tag"`, "\x00\xff": `"\x00\xff"`, "日本": "日本", "it's": `"it's"`,
	} {
		got := Quote(in)
		if got != want {
			t.Errorf("Quote(%q) = %q, want %q", in, got, want)
		}
		if words, err := SplitLine("X " + got); err != nil || len(words) != 2 || words[1] != in {
			t.Errorf("Quote(%q) reads back as %q %v", in, words, err)
		}
	}
}

// A long text with quotes in it and multi-byte characters (a JSON of Chinese) is quoted
// in time that follows its length, not the square of it.
func TestQuoteLongText(t *testing.T) {
	in := `{"name":"` + strings.Repeat("你好", 100_000) + `"}`
	start := time.Now()
	got := Quote(in)
	if d := time.Since(start); d > 2*time.Second {
		t.Errorf("%d bytes took %v", len(in), d)
	}
	if words, err := SplitLine("X " + got); err != nil || len(words) != 2 || words[1] != in {
		t.Errorf("read back: %v", err)
	}
}
