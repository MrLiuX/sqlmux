package editor

import (
	"slices"
	"testing"
)

// A ReadOnly editor changes no text: every command that would is not run and
// says so; moving, searching, VISUAL and yanking go on (F12.2).
func TestReadOnly(t *testing.T) {
	text := []string{"one two", "three four", "five"}
	feed := func(e *Editor, ks string) (eff Effect) {
		t.Helper()
		for _, k := range keys(t, ks) {
			if f := e.Feed(k); f.ReadOnly || f.Changed || f.Yank != nil || f.Error != "" {
				eff = f
			}
		}
		return eff
	}
	for _, ks := range []string{"x", "X", "dd", "dw", "D", "C", "s", "S", "cc", "cw", "i", "a", "I", "A", "o", "O", "R", "p", "P", "J", "~", "rx",
		"<<", ">>", "gUU", "guu", "g~~", "gcc", "gqq", "u", "<C-r>", "U", "vd", "vx", "vc", "vs", "v>", "v<", "vJ", "vu", "vU", "v~", "vgq", "Vd", "<C-v>d", "<C-v>Ix<Esc>", "<C-v>rx", "3x"} {
		e := New("one two\nthree four\nfive")
		e.ReadOnly = true
		eff := feed(e, ks)
		if !eff.ReadOnly || !slices.Equal(e.Lines(), text) || eff.Changed || e.Mode() != Normal && e.Mode() != Visual && e.Mode() != VisualLine && e.Mode() != VisualBlock {
			t.Errorf("%s: read only %v, text %q, mode %v", ks, eff.ReadOnly, e.Lines(), e.Mode())
		}
		feed(e, "<Esc><Esc>")
		if !slices.Equal(e.Lines(), text) {
			t.Errorf("%s: text %q", ks, e.Lines())
		}
	}
	e := New("one two\nthree four\nfive")
	e.ReadOnly = true
	if feed(e, "wjl"); e.Cursor() != (Pos{1, 5}) {
		t.Errorf("moving: %v", e.Cursor())
	}
	if eff := feed(e, "yy"); eff.ReadOnly || eff.Yank == nil {
		t.Errorf("yy: %+v", eff)
	}
	if eff := feed(e, "/five<CR>"); eff.ReadOnly || e.Cursor().Line != 2 {
		t.Errorf("search: %+v at %v", eff, e.Cursor())
	}
	if feed(e, "ggvjy"); e.Mode() != Normal || e.Cursor().Line != 0 {
		t.Errorf("VISUAL y: %v at %v", e.Mode(), e.Cursor())
	}
	if feed(e, "ggVjo<Esc>"); e.Mode() != Normal {
		t.Errorf("VISUAL o: %v", e.Mode())
	}
	if eff := e.Paste("XX"); !eff.ReadOnly || e.Lines()[0] != "one two" {
		t.Errorf("a paste: %+v %q", eff, e.Lines())
	}
	if eff := feed(e, ":%s/one/ZZ/<CR>"); !eff.ReadOnly || e.Lines()[0] != "one two" {
		t.Errorf(":s: %+v %q", eff, e.Lines())
	}
	e.ReadOnly = false
	if feed(e, "x"); e.Lines()[0] != "ne two" {
		t.Errorf("not read only: %q", e.Lines())
	}
}

// Without modes, a ReadOnly editor takes the keys that move and nothing that
// types or deletes (F12.2).
func TestReadOnlyPlain(t *testing.T) {
	e := New("one two")
	e.SetPlain(true)
	e.ReadOnly = true
	var seen bool
	for _, k := range []string{"x", "<BS>", "<Del>", "<CR>", "<C-w>", "<Tab>", "<C-z>"} {
		if eff := e.Feed(k); !eff.ReadOnly || eff.Changed || e.Lines()[0] != "one two" {
			t.Errorf("%s: %+v %q", k, eff, e.Lines())
		}
		seen = true
	}
	for _, k := range []string{"<Right>", "<Right>", "<C-e>", "<Left>", "<C-a>", "<S-Right>"} {
		if eff := e.Feed(k); eff.ReadOnly {
			t.Errorf("%s: read only", k)
		}
	}
	if !seen || e.Lines()[0] != "one two" {
		t.Errorf("text %q", e.Lines())
	}
}
