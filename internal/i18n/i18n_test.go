package i18n

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
)

var han = regexp.MustCompile(`\p{Han}`)

// stays is where a Chinese text is shown as it is, whatever language: what
// is read before `language` is, and the command line (§14「界面语言」).
var stays = map[string]string{
	"internal/config/config.go": "config.toml and keymaps.toml's own errors: the language is one of the things being read",
	"internal/ui/theme.go":      "theme files' errors, read with config.toml",
	"cmd/sqlmux/main.go":        "usage and flag errors",
}

// calls is every i18n.T and i18n.N call of the program's code, and the
// Chinese literals outside both.
func scan(t *testing.T) (keys map[string]bool, outside []string) {
	t.Helper()
	keys = map[string]bool{}
	root := filepath.Join("..", "..")
	filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		rel, _ := filepath.Rel(root, path)
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") ||
			strings.HasPrefix(rel, "internal/i18n") || strings.HasPrefix(rel, "e2e") {
			return nil
		}
		f, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		var walk func(n ast.Node, inT bool)
		walk = func(n ast.Node, inT bool) {
			ast.Inspect(n, func(c ast.Node) bool {
				switch x := c.(type) {
				case *ast.CallExpr:
					if sel, ok := x.Fun.(*ast.SelectorExpr); ok {
						if id, ok := sel.X.(*ast.Ident); ok && id.Name == "i18n" && (sel.Sel.Name == "T" || sel.Sel.Name == "N") {
							if lit, ok := x.Args[0].(*ast.BasicLit); ok && lit.Kind == token.STRING {
								s, _ := strconv.Unquote(lit.Value)
								if han.MatchString(s) {
									keys[s] = true
								}
							}
							for _, a := range x.Args {
								walk(a, true)
							}
							return false
						}
					}
				case *ast.BasicLit:
					if s, _ := strconv.Unquote(x.Value); x.Kind == token.STRING && han.MatchString(s) && !inT && stays[rel] == "" {
						outside = append(outside, rel+": "+s)
					}
				}
				return true
			})
		}
		walk(f, false)
		return nil
	})
	return keys, outside
}

// Every Chinese text the program shows goes through T or N, bar what stays
// (§14「界面语言」).
func TestNoChineseOutsideT(t *testing.T) {
	_, outside := scan(t)
	for _, s := range outside {
		t.Errorf("not translated: %s", s)
	}
}

// The English table has every text T and N are given, and nothing else, each
// one taking the arguments its Chinese does.
func TestEnglishTableCoversAll(t *testing.T) {
	keys, _ := scan(t)
	for k := range keys {
		if en[k] == "" {
			t.Errorf("no English for %q", k)
		}
	}
	for k, v := range en {
		if !keys[k] {
			t.Errorf("English for %q, which nothing shows", k)
		}
		if !slices.Equal(Verbs(k), Verbs(v)) {
			t.Errorf("%q: verbs %v, English %v", k, Verbs(k), Verbs(v))
		}
	}
}

func TestT(t *testing.T) {
	en["测试 %d"] = "test %d"
	defer delete(en, "测试 %d")
	defer Set("zh")
	if got := T("测试 %d", 3); got != "测试 3" {
		t.Errorf("zh: %q", got)
	}
	Set("en")
	if got := T("测试 %d", 3); got != "test 3" {
		t.Errorf("en: %q", got)
	}
	if got := T("没有这条"); got != "没有这条" {
		t.Errorf("missing: %q", got)
	}
	if err := Set("fr"); err == nil || Lang() != "en" {
		t.Errorf("a language that is not: %v, %s", err, Lang())
	}
	if got := Verbs("%s 有 %d %% 个 %-5.2f %v"); !slices.Equal(got, []string{"%s", "%d", "%-5.2f", "%v"}) {
		t.Errorf("verbs: %v", got)
	}
}
