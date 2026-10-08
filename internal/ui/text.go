package ui

import "github.com/charmbracelet/x/ansi"

// Width is the display width of s in cells.
func Width(s string) int {
	if printableASCII(s) {
		return len(s)
	}
	return ansi.StringWidth(s)
}

// printableASCII is whether every byte of s is a printable ASCII one: one
// cell each, and nothing the width of which depends on what comes next.
// Most of what a grid shows is, and the grapheme tables are slow for it
// (F8.20).
func printableASCII(s string) bool {
	for i := range len(s) {
		if s[i] < 0x20 || s[i] >= 0x7f {
			return false
		}
	}
	return true
}

// firstGrapheme is the first grapheme cluster of s and its width. A
// printable ASCII byte followed by another ASCII byte, or by nothing, is one
// by itself: a mark or a joiner that would join it is not ASCII.
func firstGrapheme(s string) (string, int) {
	if s != "" && s[0] >= 0x20 && s[0] < 0x7f && (len(s) == 1 || s[1] < 0x80) {
		return s[:1], 1
	}
	return ansi.FirstGraphemeCluster(s, ansi.GraphemeWidth)
}

// Truncate cuts s to at most w cells, ending in … when cut.
func Truncate(s string, w int) string { return ansi.Truncate(s, max(w, 0), "…") }

// TruncateMatch is Truncate for s with match positions pos (rune offsets,
// as TextMatch takes them): a match cut off does not light the … up.
func TruncateMatch(s string, pos []int, w int) (string, []int) {
	t := Truncate(s, w)
	if t == s {
		return s, pos
	}
	kept := len([]rune(t)) - 1 // the runes before the …
	var in []int
	for _, p := range pos {
		if p < kept {
			in = append(in, p)
		}
	}
	return t, in
}

// DropLastGrapheme removes the last grapheme cluster, as backspace does in
// nvim: é typed as e + U+0301 or 👍🏽 goes whole, not a code point at a time.
func DropLastGrapheme(s string) string {
	last := 0
	for i := 0; i < len(s); {
		gr, _ := ansi.FirstGraphemeCluster(s[i:], ansi.GraphemeWidth)
		last, i = i, i+len(gr)
	}
	return s[:last]
}
