package app

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// keyTexts are the spellings the UI shows keys by (§6.3): C-p, SPC, esc, ↵,
// Tab. A string literal that has one is a key written into the code, which
// the keymap should have been asked for (§6.7); <C-p> in angle brackets is a
// key read, as the code compares it with what came in.
var keyTexts = regexp.MustCompile(`(^|[^A-Za-z<])[CMSD]-[A-Za-z]|\bSPC\b|\besc\b|↵|\bTab\b`)

// keyTextOK are the literals that may spell a key, by file and the text, with
// why. A new entry is for a reason a key cannot be asked for.
var keyTextOK = map[string]string{
	"action.go: 命令面板：Tab": "the Tab scope of the palette, not a key",
	"palette.go: Tab":     "the Tab scope's label and a tab row's tag, not a key",
	"grid.go: ↵":          "a line break in a cell, drawn as ↵ (§7.6)",
}

// Every key the screen shows comes from the keymap, so a key that is
// rebound shows as it was rebound (§6.7, F6.1): no string literal of the
// UI's code spells one, but the allow-list's.
func TestNoKeyTextInCode(t *testing.T) {
	files, _ := filepath.Glob("*.go")
	ui, _ := filepath.Glob("../ui/*.go")
	used := map[string]bool{}
	for _, name := range append(files, ui...) {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(token.NewFileSet(), name, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(f, func(n ast.Node) bool {
			lit, ok := n.(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				return true
			}
			s, _ := strconv.Unquote(lit.Value)
			if !keyTexts.MatchString(s) {
				return true
			}
			for k := range keyTextOK {
				if file, text, _ := strings.Cut(k, ": "); filepath.Base(name) == file && strings.Contains(s, text) {
					used[k] = true
					return true
				}
			}
			t.Errorf("%s: %q spells a key", name, s)
			return true
		})
	}
	for k := range keyTextOK {
		if !used[k] {
			t.Errorf("%q is on the allow-list, and nowhere in the code", k)
		}
	}
}
