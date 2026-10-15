package editor

import (
	"strings"
	"testing"
)

// "+ and "* are the system clipboard (F3.38): a yank or a delete into it
// gives the query its text, the unnamed register too, as vim's; a plain
// one does not. "+p waits for the clipboard, then puts it as a register
// charwise, or linewise with a newline at its end, nothing when it is
// empty; the unnamed register is kept, but for one a "+ yank filled, which
// stands for "+ as in vim. With other registers the whole command goes;
// "" is the unnamed.
func TestClipboardRegister(t *testing.T) {
	e := New("one two\nthree")
	var eff Effect
	feed := func(ks string) {
		t.Helper()
		for _, k := range keys(t, ks) {
			if f := e.Feed(k); f.Clip != nil || f.Paste != nil {
				eff = f
			}
		}
	}
	if feed(`"+yy`); eff.Clip == nil || *eff.Clip != "one two\n" {
		t.Fatalf(`"+yy: %+v`, eff)
	}
	if feed(`wve"+y`); eff.Clip == nil || *eff.Clip != "two" || e.Mode() != Normal {
		t.Fatalf(`VISUAL "+y: %+v`, eff)
	}
	eff = Effect{}
	if feed("0yw"); eff.Clip != nil {
		t.Error("yw: the clipboard")
	}
	if feed(`w"*2x`); eff.Clip == nil || *eff.Clip != "tw" || e.Lines()[0] != "one o" {
		t.Fatalf(`"*2x: %+v %q`, eff, e.Lines())
	}
	eff = Effect{}
	if feed(`0yl"+p`); eff.Paste == nil || e.Lines()[0] != "one o" {
		t.Fatalf(`"+p: %+v`, eff)
	}
	if e.PutClip("CLIP", *eff.Paste); e.Lines()[0] != "oCLIPne o" {
		t.Errorf("charwise: %q", e.Lines())
	}
	eff = Effect{}
	feed(`"+P`)
	if e.PutClip("new\n", *eff.Paste); strings.Join(e.Lines(), "|") != "new|oCLIPne o|three" {
		t.Errorf("linewise: %q", e.Lines())
	}
	if feed("jp"); e.Lines()[1] != "ooCLIPne o" {
		t.Errorf("the unnamed register kept, yl's: %q", e.Lines())
	}
	eff = Effect{}
	feed(`"+p`)
	if f := e.PutClip("", *eff.Paste); f.Changed || e.Lines()[1] != "ooCLIPne o" {
		t.Errorf("an empty clipboard puts: %q", e.Lines())
	}
	before := strings.Join(e.Lines(), "|")
	if feed(`"add`); strings.Join(e.Lines(), "|") != before || e.Mode() != Normal {
		t.Errorf(`"a: %q in %v`, e.Lines(), e.Mode())
	}
	if feed(`""yyp`); len(e.Lines()) != 4 {
		t.Errorf(`"" is the unnamed: %q`, e.Lines())
	}
	e = New("one\ntwo")
	eff = Effect{}
	feed(`"+yy"+p`)
	e.PutClip("X", *eff.Paste)
	if feed("p"); strings.Join(e.Lines(), "|") != "oXXne|two" {
		t.Errorf(`the unnamed register after "+yy is "+'s: %q`, e.Lines())
	}
}

// Y in VISUAL yanks the lines, a block as y does (v_visop).
func TestVisualY(t *testing.T) {
	for ks, want := range map[string]string{`wvl"+Y`: "one two\n", `Vj"+Y`: "one two\nthree\n", `0<C-v>jl"+Y`: "on\nth"} {
		e := New("one two\nthree\nfour")
		var clip *string
		for _, k := range keys(t, ks) {
			if f := e.Feed(k); f.Clip != nil {
				clip = f.Clip
			}
		}
		if clip == nil || *clip != want || e.Mode() != Normal {
			t.Errorf("%s: %v, mode %v", ks, clip, e.Mode())
		}
	}
	for ks, mode := range map[string]Mode{"wvjY": Visual, "Vj\"+Y": VisualLine, "w<C-v>jlY": VisualBlock} { // gv comes back as it was
		e := New("one two\nthree\nfour")
		for _, k := range keys(t, ks+"gv") {
			e.Feed(k)
		}
		if e.Mode() != mode {
			t.Errorf("%sgv: mode %v, want %v", ks, e.Mode(), mode)
		}
	}
}
