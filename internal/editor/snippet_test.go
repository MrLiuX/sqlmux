package editor

import (
	"strings"
	"testing"
)

const alterText = "alter table t add column <name> <type>;"

// snippetOf is an editor on text with its <...> as tabstops, the snippet over
// all of it.
func snippetOf(text string, plain ...bool) *Editor {
	e := New(text)
	e.SetPlain(len(plain) > 0)
	e.SetHeight(10)
	e.SetWidth(80)
	var stops []Range
	for i, l := range e.Lines() {
		for from := 0; ; {
			a := strings.Index(l[from:], "<")
			if a < 0 {
				break
			}
			b := strings.Index(l[from+a:], ">") + 1
			stops = append(stops, Range{Pos{i, from + a}, Pos{i, from + a + b}})
			from += a + b
		}
	}
	last := len(e.Lines()) - 1
	e.Snippet(Pos{}, Pos{last, len(e.Lines()[last])}, stops)
	return e
}

func typed(e *Editor, ks ...string) {
	for _, k := range ks {
		e.Feed(k)
	}
}

func tabstopText(e *Editor) string {
	from, to, ok := e.Selection()
	if !ok {
		return ""
	}
	return strings.Join(e.Lines(), "\n")[e.offset(from) : e.offset(to)+1]
}

// A snippet selects its first tabstop in SELECT; what is typed takes its
// place and goes on in INSERT, Tab selects the next, and Tab on the last ends
// the snippet with no tab typed, the cursor at the end of its text (F15.8;
// measured on nvim 0.12.4's vim.snippet).
func TestSnippetTypeAndTab(t *testing.T) {
	e := snippetOf(alterText)
	if e.Mode() != Select || tabstopText(e) != "<name>" {
		t.Fatalf("start: %v %q", e.Mode(), tabstopText(e))
	}
	typed(e, "i", "d")
	if e.Mode() != Insert || e.Lines()[0] != "alter table t add column id <type>;" {
		t.Fatalf("typed: %v %q", e.Mode(), e.Lines()[0])
	}
	typed(e, "<Tab>")
	if e.Mode() != Select || tabstopText(e) != "<type>" {
		t.Fatalf("Tab: %v %q", e.Mode(), tabstopText(e))
	}
	typed(e, "i", "n", "t")
	typed(e, "<Tab>") // nvim's $0: INSERT at the end of the snippet's text, no tab typed
	if e.snip != nil || e.Mode() != Insert || e.Lines()[0] != "alter table t add column id int;" || e.Cursor() != (Pos{0, 39 - len("<name> <type>") + len("id int")}) {
		t.Errorf("the last Tab: snippet %v, %v, %q, cursor %v", e.snip != nil, e.Mode(), e.Lines()[0], e.Cursor())
	}
	typed(e, "<Tab>") // no snippet: a tab stop
	if !strings.Contains(e.Lines()[0], ";  ") && !strings.Contains(e.Lines()[0], "; ") {
		t.Errorf("Tab with no snippet: %q", e.Lines()[0])
	}
}

// S-Tab goes back, the places kept right through what is typed and deleted:
// a first tabstop made longer or shorter leaves the next one where it is; on
// the first it does nothing (F15.8).
func TestSnippetShiftTab(t *testing.T) {
	e := snippetOf(alterText)
	typed(e, "<S-Tab>")
	if e.Mode() != Select || tabstopText(e) != "<name>" {
		t.Fatalf("S-Tab on the first: %v %q", e.Mode(), tabstopText(e))
	}
	for _, word := range []string{"a_much_longer_name", "x", ""} {
		e = snippetOf(alterText)
		for _, r := range word {
			typed(e, string(r))
		}
		if word == "" {
			typed(e, "<BS>") // deleted: NORMAL, nothing selected, as nvim's Select does
			if e.Mode() != Normal {
				t.Fatalf("BS: %v", e.Mode())
			}
			typed(e, "a")
		}
		typed(e, "<Tab>")
		if tabstopText(e) != "<type>" {
			t.Fatalf("after %q: Tab selects %q in %q", word, tabstopText(e), e.Lines()[0])
		}
		typed(e, "<S-Tab>")
		if tabstopText(e) != word && word != "" {
			t.Errorf("after %q: S-Tab selects %q in %q", word, tabstopText(e), e.Lines()[0])
		}
	}
}

// BS and Del delete the selection and leave SELECT for NORMAL, as nvim 0.12.4
// does; esc leaves SELECT, the snippet stays; the arrow keys move the end of
// the selection; what is deleted is in the register (F15.8).
func TestSnippetSelectKeys(t *testing.T) {
	for _, k := range []string{"<BS>", "<C-h>", "<Del>"} {
		e := snippetOf(alterText)
		typed(e, k)
		if e.Mode() != Normal || e.Lines()[0] != "alter table t add column  <type>;" {
			t.Errorf("%s: %v %q", k, e.Mode(), e.Lines()[0])
		}
		if e.reg.text != "<name>" {
			t.Errorf("%s: register %q", k, e.reg.text)
		}
	}
	e := snippetOf(alterText)
	typed(e, "<Esc>")
	if e.Mode() != Normal || e.Lines()[0] != alterText || e.snip == nil || len(e.SnippetStops()) != 1 {
		t.Errorf("esc: %v, snippet %v, %d stops to come", e.Mode(), e.snip != nil, len(e.SnippetStops()))
	}
	typed(e, "a", "<Tab>") // a snippet esc left is still one
	if e.Mode() != Select || tabstopText(e) != "<type>" {
		t.Errorf("Tab after esc: %v %q", e.Mode(), tabstopText(e))
	}
	e = snippetOf(alterText)
	typed(e, "<Left>")
	if e.Mode() != Select || tabstopText(e) != "<name" {
		t.Errorf("Left: %v %q", e.Mode(), tabstopText(e))
	}
	typed(e, "<Right>", "<Right>")
	if tabstopText(e) != "<name> " {
		t.Errorf("Right: %q", tabstopText(e))
	}
	typed(e, "<C-c>")
	if e.Mode() != Normal {
		t.Errorf("C-c: %v", e.Mode())
	}
	e = snippetOf(alterText)
	typed(e, "<CR>")
	if e.Mode() != Insert || e.Lines()[0] != "alter table t add column " || e.Lines()[1] != "<type>;" {
		t.Errorf("↵: %v %q", e.Mode(), e.Lines())
	}
	e = snippetOf(alterText)
	e.Paste("pasted")
	if e.Mode() != Insert || e.Lines()[0] != "alter table t add column pasted <type>;" {
		t.Errorf("a paste: %v %q", e.Mode(), e.Lines()[0])
	}
	typed(e, "<Tab>")
	if tabstopText(e) != "<type>" {
		t.Errorf("Tab after a paste: %q", tabstopText(e))
	}
}

// The tabstops still to come are told to the view, not the one being filled in
// nor one with no width (F15.8).
func TestSnippetStopsToCome(t *testing.T) {
	e := snippetOf(alterText)
	if got := e.SnippetStops(); len(got) != 1 || got[0] != (Hit{Line: 0, From: 32, To: 38}) {
		t.Errorf("at the start: %+v", got)
	}
	typed(e, "x", "<Tab>") // the one typed in stays one: S-Tab goes back to it
	if got := e.SnippetStops(); len(got) != 1 || got[0] != (Hit{Line: 0, From: 25, To: 26}) {
		t.Errorf("at the last: %+v", got)
	}
	e = snippetOf("insert into t () values ();\n")
	if len(e.SnippetStops()) != 0 {
		t.Error("no stops of any text here")
	}
}

// A snippet ends when the cursor leaves its text, and when its text is
// replaced whole; Esc does not end it (F15.8).
func TestSnippetEnds(t *testing.T) {
	e := New("select 1;\n" + alterText)
	e.SetHeight(10)
	e.Snippet(Pos{1, 0}, Pos{1, len(alterText)}, []Range{{Pos{1, 26}, Pos{1, 32}}, {Pos{1, 33}, Pos{1, 39}}})
	typed(e, "<Esc>", "k") // out of it, on the line above
	if e.snipAlive() != nil {
		t.Error("the cursor left it")
	}
	typed(e, "i", "<Tab>")
	if !strings.Contains(e.Lines()[0], "  ") {
		t.Errorf("Tab with no snippet: %q", e.Lines()[0])
	}
	e = snippetOf(alterText)
	e.Load("anything\nelse")
	if e.snip != nil {
		t.Error("the text replaced")
	}
	e = snippetOf(alterText)
	e.Replace(Pos{0, 0}, Pos{0, len(alterText)}, "formatted")
	if e.snipAlive() != nil {
		t.Error("the text formatted whole")
	}
}

// A completion or a paste in the tabstop made longer moves the next one, the
// same as typing does (F15.8).
func TestSnippetPlacesFollowEdits(t *testing.T) {
	e := snippetOf("select <a>, <b> from <c>")
	e.Complete(0, "") // nothing
	typed(e, "x", "y", "z")
	e.Complete(7, "column_one") // INSERT: "xyz" replaced by a completion
	typed(e, "<Tab>")
	if tabstopText(e) != "<b>" {
		t.Errorf("%q in %q", tabstopText(e), e.Lines()[0])
	}
	typed(e, "<Tab>")
	if tabstopText(e) != "<c>" {
		t.Errorf("%q in %q", tabstopText(e), e.Lines()[0])
	}
}

// Tabstops over several lines and an empty one: a tabstop with no text is a
// place to type in INSERT, not a selection (F15.8).
func TestSnippetEmptyAndMultiline(t *testing.T) {
	e := New("insert into t () values ();")
	e.SetHeight(10)
	e.Snippet(Pos{}, Pos{0, 26}, []Range{{Pos{0, 15}, Pos{0, 15}}, {Pos{0, 25}, Pos{0, 25}}})
	if e.Mode() != Insert || e.Cursor() != (Pos{0, 15}) {
		t.Fatalf("an empty tabstop: %v %v", e.Mode(), e.Cursor())
	}
	typed(e, "a", ",", "b", "<Tab>", "1")
	if e.Lines()[0] != "insert into t (a,b) values (1);" {
		t.Errorf("%q", e.Lines()[0])
	}
	m := New("a\n<x>\nb\n<y>")
	m.SetHeight(10)
	m.Snippet(Pos{}, Pos{3, 3}, []Range{{Pos{1, 0}, Pos{1, 3}}, {Pos{3, 0}, Pos{3, 3}}})
	typed(m, "1", "<CR>", "2", "<Tab>")
	if tabstopText(m) != "<y>" || strings.Join(m.Lines(), "|") != "a|1|2|b|<y>" {
		t.Errorf("%q in %q", tabstopText(m), m.Lines())
	}
}

// A read only text takes no snippet: nothing to type into (F15.8).
func TestSnippetNotReadOnly(t *testing.T) {
	e := New(alterText)
	e.ReadOnly = true
	e.Snippet(Pos{}, Pos{0, len(alterText)}, []Range{{Pos{0, 26}, Pos{0, 32}}})
	if e.snip != nil || e.Mode() != Normal {
		t.Error("a snippet in a read only editor")
	}
}

// Without modes the tabstop is the selection, typing writes over it, Tab and
// S-Tab go between them, and the last Tab ends it with no tab typed (F15.8).
func TestSnippetPlain(t *testing.T) {
	e := snippetOf(alterText, true)
	if !e.HasSelection() || tabstopText(e) != "<name>" {
		t.Fatalf("start: %q", tabstopText(e))
	}
	typed(e, "i", "d", "<Tab>")
	if tabstopText(e) != "<type>" || e.Lines()[0] != "alter table t add column id <type>;" {
		t.Fatalf("Tab: %q in %q", tabstopText(e), e.Lines()[0])
	}
	typed(e, "<S-Tab>")
	if tabstopText(e) != "id" {
		t.Errorf("S-Tab: %q", tabstopText(e))
	}
	typed(e, "<Tab>", "i", "n", "t", "<Tab>")
	if e.snip != nil || e.Lines()[0] != "alter table t add column id int;" || e.HasSelection() || e.Cursor() != (Pos{0, len("alter table t add column id int;")}) {
		t.Errorf("the last Tab: %q, snippet %v, cursor %v", e.Lines()[0], e.snip != nil, e.Cursor())
	}
}

// The snippet's typing is one undo step a tabstop (F15.8).
func TestSnippetUndo(t *testing.T) {
	e := snippetOf(alterText)
	typed(e, "i", "d", "<Tab>", "i", "n", "t", "<Esc>", "u")
	if got := e.Lines()[0]; got != "alter table t add column id <type>;" {
		t.Errorf("u: %q", got)
	}
	typed(e, "u")
	if got := e.Lines()[0]; got != alterText {
		t.Errorf("uu: %q", got)
	}
}

// An edit inside a tabstop that is not the one being filled in makes it longer
// (the whole of it is selected when it comes), and one right after its end
// leaves it as it was (F15.8).
func TestSnippetEditsOfOtherTabstops(t *testing.T) {
	e := snippetOf("select <a>, <b> from t")
	typed(e, "<Tab>", "<Esc>", "0", "f", "a", "i", "Q", "<Esc>") // <b> is the one filled in; Q goes into <a>
	typed(e, "f", ",", "i", "W", "<Esc>")                        // W goes right after <Qa>
	typed(e, "a", "<S-Tab>")
	if got := tabstopText(e); got != "<Qa>" || e.Lines()[0] != "select <Qa>W, <b> from t" {
		t.Errorf("S-Tab: %q in %q", got, e.Lines()[0])
	}
}
