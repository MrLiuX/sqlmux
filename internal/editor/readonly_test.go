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

// Without modes, a ReadOnly editor takes the keys that move, selecting and Esc
// among them, and nothing that types or deletes (F12.2).
func TestReadOnlyPlain(t *testing.T) {
	e := New("one two\nthree four\nfive")
	e.SetPlain(true)
	e.ReadOnly = true
	text := e.Lines()
	for _, k := range []string{"x", "<BS>", "<Del>", "<CR>", "<C-w>", "<Tab>", "<C-z>", "<Home>", "<End>"} {
		if eff := e.Feed(k); !eff.ReadOnly || eff.Changed || !slices.Equal(e.Lines(), text) {
			t.Errorf("%s: %+v %q", k, eff, e.Lines())
		}
	}
	// each key moves the cursor to the place, and is no toast
	for _, c := range []struct {
		k  string
		at Pos
	}{{"<Right>", Pos{0, 1}}, {"<Down>", Pos{1, 1}}, {"<Left>", Pos{1, 0}}, {"<Up>", Pos{0, 0}}, {"<C-e>", Pos{0, 7}}, {"<C-a>", Pos{0, 0}},
		{"<S-Right>", Pos{0, 1}}, {"<S-Left>", Pos{0, 0}}, {"<S-Down>", Pos{1, 0}}, {"<S-Up>", Pos{0, 0}}} {
		if eff := e.Feed(c.k); eff.ReadOnly || e.Cursor() != c.at {
			t.Errorf("%s: read only %v, at %v, want %v", c.k, eff.ReadOnly, e.Cursor(), c.at)
		}
	}
	e.Feed("<S-Right>")
	if eff := e.Feed("<Esc>"); eff.ReadOnly || !slices.Equal(e.Lines(), text) {
		t.Errorf("Esc: %+v", eff)
	}
}

// Moving the selection about, in each of VISUAL's three kinds, is no change
// and no toast (F12.2).
func TestReadOnlyVisualMoves(t *testing.T) {
	for ks, mode := range map[string]Mode{"vV": VisualLine, "v<C-v>": VisualBlock, "<C-v>v": Visual, "<C-v>O": VisualBlock, "vo": Visual, "vj<Esc>gv": Visual, "vj<Esc>Gvgv": Visual, "V<C-v>": VisualBlock, "v<C-q>": VisualBlock} {
		e := New("one two\nthree four\nfive")
		e.ReadOnly = true
		for _, k := range keys(t, ks) {
			if eff := e.Feed(k); eff.ReadOnly {
				t.Errorf("%s: %s is read only", ks, k)
			}
		}
		if e.Mode() != mode {
			t.Errorf("%s: mode %v, want %v", ks, e.Mode(), mode)
		}
	}
}

// The : and / line is no text of the buffer: a paste into it works in a read
// only editor (F12.2).
func TestReadOnlyPasteCommandLine(t *testing.T) {
	e := New("one")
	e.ReadOnly = true
	e.Feed("/")
	if eff := e.Paste("on"); eff.ReadOnly {
		t.Error("read only")
	}
	if _, text, _, _ := e.CmdLine(); text != "on" {
		t.Errorf("the line holds %q", text)
	}
	e.Feed("<Esc>")
	if eff := e.Paste("on"); !eff.ReadOnly {
		t.Error("the buffer takes a paste")
	}
}
