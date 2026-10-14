package redis

import (
	"testing"
	"time"
)

// A glob that matches a name alone, and the TTL a form takes (F16.2).
func TestGlobAndTTL(t *testing.T) {
	for in, want := range map[string]string{"plain": "plain", "a*b": `a\*b`, "a?b[c]d": `a\?b\[c\]d`, `a\b`: `a\\b`, "": ""} {
		if got := Glob(in); got != want {
			t.Errorf("Glob(%q) = %q, want %q", in, got, want)
		}
	}
	for _, c := range []struct {
		in   string
		ttl  time.Duration
		ok   bool
		what string
	}{
		{"", 0, true, "none"}, {"0", 0, true, "zero"}, {"-1", 0, true, "-1"}, {" 3600 ", time.Hour, true, "seconds"},
		{"90m", 90 * time.Minute, true, "a duration"}, {"1h30m", 90 * time.Minute, true, "two units"}, {"-5", 0, false, "negative"},
		{"-5m", 0, false, "negative duration"}, {"soon", 0, false, "words"}, {"1.5", 0, false, "a fraction of seconds"},
	} {
		if ttl, ok := ParseTTL(c.in); ok != c.ok || ok && ttl != c.ttl {
			t.Errorf("%s %q: %v %v, want %v %v", c.what, c.in, ttl, ok, c.ttl, c.ok)
		}
	}
}
