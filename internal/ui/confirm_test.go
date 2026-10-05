package ui

import (
	"strings"
	"testing"

	uv "github.com/charmbracelet/ultraviolet"
)

// A line the screen cannot hold goes on the next, whole: the question
// is read to its end (§10.5).
func TestConfirmWraps(t *testing.T) {
	const text = "doraemon, play, far has an uncommitted transaction; quitting would roll it back."
	f := NewFrame(40, 12, TokyonightStorm)
	Confirm{Text: text, YesKey: "y", Yes: "Quit", NoKey: "n", No: "Cancel"}.Draw(f, uv.Rect(0, 0, 40, 12))
	var words []string
	for _, l := range strings.Split(f.String(), "\n") {
		words = append(words, strings.Fields(strings.Trim(strings.TrimSpace(l), "│┌┐└┘─ "))...)
	}
	if got := strings.Join(words, " "); !strings.Contains(got, strings.Join(strings.Fields(text), " ")) {
		t.Errorf("the box says %q", got)
	}
}
