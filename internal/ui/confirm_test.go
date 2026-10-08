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

// A danger confirm has its title, the first line however it wraps, in the
// error color, and no other line (§13).
func TestConfirmDangerTitle(t *testing.T) {
	th := TokyonightStorm
	// the rows of the box with a cell in the error color, and what they say
	errRows := func(danger bool) (rows []string) {
		f := NewFrame(30, 12, th)
		Confirm{Text: "Drop table public.t_user, which cannot be undone:\n  DROP TABLE t", Danger: danger, YesKey: "y", Yes: "Drop", NoKey: "n", No: "Cancel"}.Draw(f, uv.Rect(0, 0, 30, 12))
		for y, line := range strings.Split(f.String(), "\n") {
			for x := range 30 {
				if c := f.Buf.CellAt(x, y); c.Style.Fg == th.Error && strings.TrimSpace(c.String()) != "" {
					rows = append(rows, strings.TrimSpace(strings.Trim(line, "│ ")))
					break
				}
			}
		}
		return rows
	}
	if got := errRows(true); len(got) != 2 || !strings.HasPrefix(got[0], "Drop table") || !strings.HasSuffix(got[1], "undone:") {
		t.Errorf("the title: %q", got)
	}
	if got := errRows(false); len(got) != 0 {
		t.Errorf("no danger: %q", got)
	}
}
