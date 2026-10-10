package app

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/charmbracelet/x/exp/golden"

	"github.com/kackerx/sqlmux/internal/config"
	"github.com/kackerx/sqlmux/internal/ui"
)

// groupedFile is five connections of two groups and two with none, in the
// order of the file: the tree and the manager show doraemon, misc, then prod's
// stage and live, then dev's ops (F10.6).
const groupedFile = `[[connection]]
name = "doraemon"
engine = "postgres"
dsn = "postgres://u@h:5433/db?sslmode=disable"

[[connection]]
name = "stage"
group = "prod"
engine = "mysql"
dsn = "u:p@tcp(m:3306)/s"

[[connection]]
name = "ops"
group = "dev"
engine = "postgres"
dsn = "postgres://u@h/ops"

[[connection]]
name = "live"
group = "prod"
engine = "postgres"
dsn = "postgres://u@h/live"

[[connection]]
name = "misc"
engine = "postgres"
dsn = "postgres://u@h/misc"
`

// grouped is managed over groupedFile, the connections of the session in the
// order the tree has them.
func grouped(t *testing.T) *App {
	t.Helper()
	a := managed(t)
	if err := os.WriteFile(filepath.Join(config.Dir(), "connections.toml"), []byte(groupedFile), 0o644); err != nil {
		t.Fatal(err)
	}
	specs, _, err := config.LoadConnections("")
	if err != nil {
		t.Fatal(err)
	}
	conns := []*Conn{a.sess.Conns[0]}
	conns[0].Spec = specs[0]
	for _, sp := range specs[1:] {
		conns = append(conns, newConn(sp))
	}
	a.sess.Conns = conns
	groupOrder(a.sess.Conns)
	return a
}

func connOrder(a *App) string {
	var names []string
	for _, c := range a.sess.Conns {
		names = append(names, c.Spec.Name)
	}
	return strings.Join(names, " ")
}

// Connections with no group first, then each group in the order its first one
// comes in the file, the connections of each in the file's order (F10.6).
func TestGroupOrder(t *testing.T) {
	a := grouped(t)
	if got := connOrder(a); got != "doraemon misc stage live ops" {
		t.Errorf("order: %s", got)
	}
	groupOrder(a.sess.Conns) // again: as it is
	if got := connOrder(a); got != "doraemon misc stage live ops" {
		t.Errorf("again: %s", got)
	}
	if got := strings.Join(a.groupNames(), " "); got != "prod dev" {
		t.Errorf("groups: %s", got)
	}
}

// The tree draws a rule with the group's name where a group begins and one
// with none before the workspace; the cursor skips them, j and k counting
// the nodes, a click is not on one, and a filter draws none (F10.6).
func TestTreeGroups(t *testing.T) {
	a := grouped(t)
	a.sess.open["conn:doraemon"] = false
	roots := func() string {
		var out []string
		ns, _ := a.treeNodes()
		for _, n := range ns {
			switch n.kind {
			case nodeConn:
				out = append(out, n.Text)
			case nodeSep:
				out = append(out, "── "+n.Text)
			case nodeSchema, nodeGroup, nodeTable, nodeColumn, nodeWorkspace, nodeWindow, nodeTab, nodeConsoles, nodeConsoleFile:
			}
		}
		return strings.Join(out, "|")
	}
	if got := roots(); got != "doraemon|misc|── prod|stage|live|── dev|ops|── " {
		t.Fatalf("roots: %s", got)
	}
	cursor := func() string { n, _ := a.treeNode(); return n.Text }
	a.win().focus(0)
	ns0, _ := a.treeNodes()
	a.treeGo(slices.IndexFunc(ns0, func(n node) bool { return n.kind == nodeSep && n.Text == "prod" }))
	if cursor() != "stage" {
		t.Errorf("a rule is not where the cursor rests: %q", cursor())
	}
	treeTo(t, a, "live")
	if feed(t, a, "j"); cursor() != "ops" {
		t.Errorf("j past the rule: %q", cursor())
	}
	if feed(t, a, "k"); cursor() != "live" {
		t.Errorf("k: %q", cursor())
	}
	if feed(t, a, "2k"); cursor() != "misc" {
		t.Errorf("2k from live: %q", cursor())
	}
	if feed(t, a, "gg"); cursor() != "doraemon" {
		t.Errorf("gg: %q", cursor())
	}
	feed(t, a, "G")
	if n, _ := a.treeNode(); n.kind == nodeSep {
		t.Errorf("G on a rule")
	}
	treeTo(t, a, "ops")
	if feed(t, a, "j"); cursor() != "工作区" {
		t.Errorf("j from the last connection skips the rule before the workspace: %q", cursor())
	}
	a.View()
	ns, _ := a.treeNodes()
	for _, h := range a.hits {
		if h.Target.Kind == ui.KindNode && ns[h.Target.I].kind == nodeSep {
			t.Errorf("a rule is a region to click: %+v", h.Target)
		}
	}
	feed(t, a, "/t_")
	if got := roots(); strings.Contains(got, "──") {
		t.Errorf("a filter draws no rule: %s", got)
	}
}

// The manager's list has the same rules, one above the row that makes a
// connection when there are groups; the selection stays in sight through them
// (F10.6).
func TestManagerGroups(t *testing.T) {
	a := grouped(t)
	a.run("conn.manage", 0)
	v := a.managerView()
	var got []string
	for _, r := range v.Rows {
		if r.Rule {
			got = append(got, "── "+r.Group)
		}
		got = append(got, r.Name)
	}
	if want := "doraemon|misc|── prod|stage|live|── dev|ops|── |新建"; strings.Join(got, "|") != want {
		t.Errorf("rows: %s", strings.Join(got, "|"))
	}
	golden.RequireEqual(t, a.render().String())
	// the list is shorter than its rows and rules: moving down keeps the selection showing
	for range 7 {
		feed(t, a, "j")
	}
	v = a.managerView()
	lines := 0
	for _, r := range v.Rows[a.form.top : a.form.cur+1] {
		if lines++; r.Rule {
			lines++
		}
	}
	if lines > v.ListRows(a.w) {
		t.Errorf("the selection is out of sight: top %d, cur %d, %d lines in %d", a.form.top, a.form.cur, lines, v.ListRows(a.w))
	}
}

// A connection moved into a group takes the place the file gives it among
// that group's, not the end of it, and the form follows it (F10.6).
func TestRegroupFileOrder(t *testing.T) {
	a := grouped(t)
	a.run("conn.manage", 0)
	a.selectConn(slices.IndexFunc(a.sess.Conns, func(c *Conn) bool { return c.Spec.Name == "ops" }))
	feed(t, a, "<Tab><Tab><C-u>prod<Esc><C-s>") // ops is the file's second and live its third
	if got := connOrder(a); got != "doraemon misc stage ops live" || a.sess.Conns[a.form.cur].Spec.Name != "ops" {
		t.Errorf("order %s, form on %q", got, a.sess.Conns[a.form.cur].Spec.Name)
	}
}

// The form has a group, written under the name in connections.toml; an edit
// that changes it moves the connection to where the file puts it, the form
// following it; clearing it takes the line out; a new connection's group is
// written too and it lands at the end of its group (F10.6).
func TestFormGroup(t *testing.T) {
	a := grouped(t)
	a.run("conn.manage", 0)
	a.selectConn(slices.IndexFunc(a.sess.Conns, func(c *Conn) bool { return c.Spec.Name == "stage" }))
	if fieldText(a, fieldGroup) != "prod" {
		t.Fatalf("the group of stage: %q", fieldText(a, fieldGroup))
	}
	feed(t, a, "<Tab><Tab><C-u>dev<Esc><C-s>") // esc closes the list of groups
	if a.form == nil {
		t.Fatalf("closed by esc")
	}
	if got := connOrder(a); got != "doraemon misc stage ops live" {
		t.Errorf("stage is in dev, before ops, which is after it in the file: %s", got)
	}
	if !strings.Contains(readConn(t), "name = \"stage\"\ngroup = \"dev\"\n") || a.sess.Conns[a.form.cur].Spec.Name != "stage" {
		t.Errorf("file:\n%s\nform on %q", readConn(t), a.sess.Conns[a.form.cur].Spec.Name)
	}
	feed(t, a, "<Tab><Tab><C-u><Esc><C-s>")
	if strings.Contains(readConn(t), "name = \"stage\"\ngroup") || connOrder(a) != "doraemon stage misc ops live" {
		t.Errorf("cleared: %s\n%s", connOrder(a), readConn(t))
	}
	feed(t, a, "<Esc><Esc><C-p>conn.new<CR>fresh<Tab>d<Tab><Tab><Tab>h<Tab><Tab>u<Tab><Tab>d<CR>")
	if a.form != nil || !strings.Contains(readConn(t), "name = \"fresh\"\ngroup = \"dev\"\n") || connOrder(a) != "doraemon stage misc ops fresh live" {
		t.Errorf("new: form %v, order %s\n%s", a.form, connOrder(a), readConn(t))
	}
	feed(t, a, "<C-p>conn.new<CR>bad<Tab>a/b<Tab><Tab>h<Tab><Tab>u<Tab><Tab>d<CR>")
	if a.form == nil || a.form.err == "" || !a.form.fields[fieldGroup].Bad {
		t.Errorf("a slash in a group: %+v", a.form)
	}
}

func readConn(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(config.Dir(), "connections.toml"))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// The group field lists the groups there are: all of them before anything is
// typed, none selected, then those that start as what is typed does, the first
// selected; Tab and ↵ take the selected one and only that, with none selected
// they are what they are without the list; ↓ and ↑ pick, esc closes the list
// (F10.6).
func TestFormGroupList(t *testing.T) {
	a := grouped(t)
	feed(t, a, "<C-p>conn.new<CR>n<Tab>")
	cands := func() string { return strings.Join(a.groupCands(), " ") }
	pick := func() int { return a.form.groupPick(len(a.groupCands())) }
	if a.form.sel != fieldGroup || cands() != "prod dev" || pick() != -1 || a.context().Overlay != "groups" {
		t.Fatalf("empty: on %d, %q, pick %d, overlay %q", a.form.sel, cands(), pick(), a.context().Overlay)
	}
	if feed(t, a, "<Down>"); pick() != 0 {
		t.Errorf("↓: %d", pick())
	}
	if feed(t, a, "<Up>"); pick() != 1 {
		t.Errorf("↑ round: %d", pick())
	}
	feed(t, a, "<CR>")
	if fieldText(a, fieldGroup) != "dev" || a.form == nil || a.form.sel != fieldGroup || cands() != "" {
		t.Errorf("↵ takes dev and saves nothing: %q, form %v, %q", fieldText(a, fieldGroup), a.form != nil, cands())
	}
	feed(t, a, "<C-u>P")
	if cands() != "prod" || pick() != 0 {
		t.Errorf("a prefix, either case: %q pick %d", cands(), pick())
	}
	if feed(t, a, "<Tab>"); fieldText(a, fieldGroup) != "prod" || a.form.sel != fieldGroup {
		t.Errorf("Tab takes the first: %q on %d", fieldText(a, fieldGroup), a.form.sel)
	}
	feed(t, a, "<C-u>x")
	if cands() != "" {
		t.Errorf("no group starts with x: %q", cands())
	}
	if feed(t, a, "<Tab>"); a.form.sel != fieldEngine {
		t.Errorf("Tab with no list goes on: %d", a.form.sel)
	}
	feed(t, a, "<S-Tab><C-u>")
	if cands() != "prod dev" {
		t.Fatalf("empty again: %q", cands())
	}
	if feed(t, a, "<Tab>"); a.form.sel != fieldEngine || fieldText(a, fieldGroup) != "" {
		t.Errorf("Tab with none selected goes on: %d %q", a.form.sel, fieldText(a, fieldGroup))
	}
	feed(t, a, "<S-Tab><Esc>")
	if a.form == nil || cands() != "" || a.context().Overlay != "form" {
		t.Errorf("esc closes the list, not the form: %v %q", a.form != nil, cands())
	}
	feed(t, a, "p")
	if cands() != "prod" {
		t.Errorf("typing opens it again: %q", cands())
	}
	golden.RequireEqual(t, a.render().String())
}
