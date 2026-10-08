package ui

import (
	"math/rand/v2"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

// The ASCII fast paths of Width, firstGrapheme and Printable agree with what
// they stand in for, on the controls (DEL among them) and on what joins an
// ASCII byte to the one after it (F8.20).
func TestTextFastPaths(t *testing.T) {
	pieces := []string{"a", "Z", "1", " ", "~", "\t", "\n", "\r", "\x00", "\x1b", "\x7f", "\x1b[31m",
		"\u0301", "\u200d", "\ufe0f", "\ufe0e", "\u20e3", "表", "ｆ", "👍", "🏽", "🇨", "🇳", "é", "\u200b", "#", "*"}
	cases := []string{"", "a", "\x7f", "a\x7f", "a\tb", "a\nb", "\r\n", "a\r\nb", "1\ufe0f\u20e3", "x\u200dy", "a\x1b[31mb", "~\u0300"}
	rng := rand.New(rand.NewPCG(9, 9))
	for range 5000 {
		var b strings.Builder
		for range rng.IntN(8) {
			b.WriteString(pieces[rng.IntN(len(pieces))])
		}
		cases = append(cases, b.String())
	}
	for _, s := range cases {
		if got, want := Width(s), ansi.StringWidth(s); got != want {
			t.Errorf("Width(%q) = %d, ansi says %d", s, got, want)
		}
		if s != "" {
			g, w := firstGrapheme(s)
			if wg, ww := ansi.FirstGraphemeCluster(s, ansi.GraphemeWidth); g != wg || w != ww {
				t.Errorf("firstGrapheme(%q) = %q, %d; ansi says %q, %d", s, g, w, wg, ww)
			}
		}
	}
	for s, want := range map[string]string{"a\x7f": "a", "a\tb": "a b", "a\nb": "a↵b", "\r\n": "↵", "a\x1bb": "ab"} {
		if got := Printable(s); got != want {
			t.Errorf("Printable(%q) = %q, want %q", s, got, want)
		}
	}
}
