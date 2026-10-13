package ui

import (
	"strings"
	"testing"

	uv "github.com/charmbracelet/ultraviolet"
)

func seg(drop int, runs ...string) Segment {
	s := Segment{Drop: drop}
	for _, r := range runs {
		s.Runs = append(s.Runs, Run{Text: r})
	}
	return s
}

func shrinkable(runs ...string) Segment {
	s := seg(0, runs...)
	s.Runs[1].Shrink = true
	return s
}

// §7.8's order: info, then the connection, then other windows right to left,
// then the cursor position, and finally the session name is cut.
func TestStatusLineNarrowing(t *testing.T) {
	s := StatusLine{
		Left:  []Segment{shrinkable(" ", "doraemon", " ▾ "), seg(0, " 0: a* "), seg(2, " 1: b "), seg(2, " 2: c ")},
		Info:  "some extra info",
		Right: []Segment{seg(0, " C-p "), seg(0, " · "), seg(3, " 1,1 "), seg(1, " pg@host "), seg(0, " NORMAL ")},
	}
	// widths: left 12+7+6+6 = 31, right 5+3+5+9+8 = 30
	for _, c := range []struct {
		w                 int
		left, info, right string
	}{
		{80, " doraemon ▾  0: a*  1: b  2: c ", " some extra info ", " C-p  ·  1,1  pg@host  NORMAL "},
		{73, " doraemon ▾  0: a*  1: b  2: c ", " some extr… ", " C-p  ·  1,1  pg@host  NORMAL "},
		{72, " doraemon ▾  0: a*  1: b  2: c ", "", " C-p  ·  1,1  pg@host  NORMAL "}, // 11 spare: no info
		{60, " doraemon ▾  0: a*  1: b  2: c ", "", " C-p  ·  1,1  NORMAL "},          // the connection goes
		{46, " doraemon ▾  0: a*  1: b ", "", " C-p  ·  1,1  NORMAL "},                // then windows, right to left
		{40, " doraemon ▾  0: a* ", "", " C-p  ·  1,1  NORMAL "},
		{35, " doraemon ▾  0: a* ", "", " C-p  ·  NORMAL "}, // then the cursor
		{31, " dor… ▾  0: a* ", "", " C-p  ·  NORMAL "},     // then the session name is cut
	} {
		f := NewFrame(c.w, 1, TokyonightStorm)
		s.Draw(f, uv.Rect(0, 0, c.w, 1))
		row := f.String()
		row += strings.Repeat(" ", c.w-Width(row)) // String trims trailing blanks
		gap := c.w - Width(c.left+c.info+c.right)
		if gap < 0 {
			t.Fatalf("w=%d: bad case, parts are wider than the bar", c.w)
		}
		if want := c.left + strings.Repeat(" ", gap) + c.info + c.right; row != want {
			t.Errorf("w=%d:\n got %q\nwant %q", c.w, row, want)
		}
	}
	if s.Left[2].Runs[0].Text != " 1: b " || s.Left[0].Runs[1].Text != "doraemon" {
		t.Error("Draw must not change the caller's segments")
	}
}

// A tip is the first to go, before the connection, and the info stays whole
// for it: it is not cut for a tip, nor does the tip's going take it away (F15.4).
func TestStatusLineTip(t *testing.T) {
	tip := seg(1, " Tip: yl copies the cell ")
	tip.Tip = true
	s := StatusLine{
		Left:  []Segment{seg(0, " doraemon ▾ "), seg(0, " 0: a* ")},
		Info:  "-- editing WHERE --",
		Right: []Segment{tip, seg(0, " C-p "), seg(2, " pg@host "), seg(0, " NORMAL ")},
	}
	// widths: left 12+7 = 19; right 5+9+8 = 22 and the tip's 25; the info's 19+2
	for _, c := range []struct {
		w           int
		info, right string
		left        string
	}{
		{100, " -- editing WHERE -- ", " Tip: yl copies the cell  C-p  pg@host  NORMAL ", " doraemon ▾  0: a* "},
		{87, " -- editing WHERE -- ", " Tip: yl copies the cell  C-p  pg@host  NORMAL ", " doraemon ▾  0: a* "},
		{86, " -- editing WHERE -- ", " C-p  pg@host  NORMAL ", " doraemon ▾  0: a* "}, // the tip, not the info's room
		{62, " -- editing WHERE -- ", " C-p  pg@host  NORMAL ", " doraemon ▾  0: a* "},
		{61, " -- editing WHERE … ", " C-p  pg@host  NORMAL ", " doraemon ▾  0: a* "}, // what the info is cut for is the connection's, not the tip's
	} {
		f := NewFrame(c.w, 1, TokyonightStorm)
		s.Draw(f, uv.Rect(0, 0, c.w, 1))
		row := f.String()
		row += strings.Repeat(" ", c.w-Width(row))
		gap := c.w - Width(c.left+c.info+c.right)
		if want := c.left + strings.Repeat(" ", gap) + c.info + c.right; gap < 0 || row != want {
			t.Errorf("w=%d:\n got %q\nwant %q", c.w, row, want)
		}
	}
	// no info: the tip goes first, then the connection
	s.Info = ""
	for _, c := range []struct {
		w     int
		right string
	}{{66, " Tip: yl copies the cell  C-p  pg@host  NORMAL "}, {65, " C-p  pg@host  NORMAL "}, {40, " C-p  NORMAL "}} {
		f := NewFrame(c.w, 1, TokyonightStorm)
		s.Draw(f, uv.Rect(0, 0, c.w, 1))
		if row := f.String(); !strings.HasSuffix(row, strings.TrimRight(c.right, " ")) || strings.Contains(row, "Tip") != strings.Contains(c.right, "Tip") {
			t.Errorf("w=%d: %q does not end with %q", c.w, row, c.right)
		}
	}
	// a tip does not count among the segments that took the info away
	s.Info = "x extra info"
	f := NewFrame(95, 1, TokyonightStorm)
	s.Draw(f, uv.Rect(0, 0, 95, 1))
	if !strings.Contains(f.String(), "x extra info") || !strings.Contains(f.String(), "Tip") {
		t.Errorf("both fit: %q", f.String())
	}
}
