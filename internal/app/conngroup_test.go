package app

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	uv "github.com/charmbracelet/ultraviolet"
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
func grouped(t *testing.T) *App { return groupedIn(t, groupedFile) }

// groupedIn is that over file.
func groupedIn(t *testing.T, file string) *App {
	t.Helper()
	a := managed(t)
	if err := os.WriteFile(filepath.Join(config.Dir(), "connections.toml"), []byte(file), 0o644); err != nil {
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

// preludeFile is groupedFile and a connection of a group that starts as prod
// does.
const preludeFile = groupedFile + `
[[connection]]
name = "pre"
group = "prelude"
engine = "postgres"
dsn = "postgres://u@h/pre"
`

// The group field lists the groups once something is typed and there is a
// match, those that start as what is typed does, the first selected, as a
// completion does; focus coming into the field opens none, so C-p is the
// palette's; Tab and ↵ take the selected one and only that, with no list they
// are what they are without it; ↓ and ↑ pick, esc closes the list (F10.6).
func TestFormGroupList(t *testing.T) {
	a := groupedIn(t, preludeFile)
	feed(t, a, "<C-p>conn.new<CR>n<Tab>")
	cands := func() string { return strings.Join(a.groupCands(), " ") }
	pick := func() int { return a.form.groupPick(len(a.groupCands())) }
	if a.form.sel != fieldGroup || cands() != "" || a.context().Overlay != "form" {
		t.Fatalf("empty: on %d, %q, overlay %q", a.form.sel, cands(), a.context().Overlay)
	}
	if feed(t, a, "<C-p>"); a.palette == nil {
		t.Errorf("C-p on the group field is the palette's")
	}
	feed(t, a, "<Esc>p")
	if cands() != "prod prelude" || pick() != 0 || a.context().Overlay != "groups" {
		t.Fatalf("p: %q, pick %d, overlay %q", cands(), pick(), a.context().Overlay)
	}
	if feed(t, a, "<Down>"); pick() != 1 {
		t.Errorf("↓: %d", pick())
	}
	if feed(t, a, "<Down>"); pick() != 0 {
		t.Errorf("↓ round: %d", pick())
	}
	if feed(t, a, "<Up>"); pick() != 1 {
		t.Errorf("↑ round: %d", pick())
	}
	feed(t, a, "<CR>")
	if fieldText(a, fieldGroup) != "prelude" || a.form == nil || a.form.sel != fieldGroup || cands() != "" {
		t.Errorf("↵ takes prelude and saves nothing: %q, form %v, %q", fieldText(a, fieldGroup), a.form != nil, cands())
	}
	feed(t, a, "<C-u>P")
	if cands() != "prod prelude" || pick() != 0 {
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
	if feed(t, a, "<S-Tab><C-u><Tab>"); a.form.sel != fieldEngine || fieldText(a, fieldGroup) != "" {
		t.Errorf("Tab with nothing typed goes on: %d %q", a.form.sel, fieldText(a, fieldGroup))
	}
	feed(t, a, "<S-Tab>p<Esc>")
	if a.form == nil || cands() != "" || a.context().Overlay != "form" {
		t.Errorf("esc closes the list, not the form: %v %q", a.form != nil, cands())
	}
	feed(t, a, "r")
	if cands() != "prod prelude" {
		t.Errorf("typing opens it again: %q", cands())
	}
	golden.RequireEqual(t, a.render().String())
}

// The list is the group field's: another field has none however its text
// starts (F10.6).
func TestFormGroupListOnlyOnTheField(t *testing.T) {
	a := grouped(t)
	feed(t, a, "<C-p>conn.new<CR>n<Tab>p<Esc><Tab>") // what is typed matches, and the focus is on the engine
	if a.form.sel != fieldEngine || len(a.groupCands()) != 0 {
		t.Errorf("a list on field %d: %q", a.form.sel, a.groupCands())
	}
}

// A click on a group of the list takes it, as ↵ does; the list scrolls to
// the group ↓ has picked past its rows; typing a group in the other case still
// offers it; the arrows that only move the cursor do not open the list again
// after esc closed it, a click on a field does (F10.6).
func TestFormGroupListMouseScrollCase(t *testing.T) {
	a := groupedIn(t, preludeFile)
	feed(t, a, "<C-p>conn.new<CR>n<Tab>p")
	click(a, find(t, a, ui.Target{Kind: ui.KindRow, I: 1, Action: "form.pick"}).Min)
	if fieldText(a, fieldGroup) != "prelude" || a.form.sel != fieldGroup || len(a.groupCands()) != 0 {
		t.Errorf("click: %q on %d", fieldText(a, fieldGroup), a.form.sel)
	}
	feed(t, a, "<C-u>Prod")
	if got := strings.Join(a.groupCands(), " "); got != "prod" {
		t.Errorf("Prod offers the group in its own case: %q", got)
	}
	feed(t, a, "<Esc><Left>")
	if len(a.groupCands()) != 0 {
		t.Errorf("a cursor move opens the list again")
	}
	click(a, find(t, a, ui.Target{Kind: ui.KindRow, I: fieldName}).Min)
	click(a, find(t, a, ui.Target{Kind: ui.KindRow, I: fieldGroup}).Min)
	if got := strings.Join(a.groupCands(), " "); got != "prod" {
		t.Errorf("back on the field by a click: %q", got)
	}

	var many strings.Builder
	for i := range 10 {
		fmt.Fprintf(&many, "[[connection]]\nname = \"c%d\"\ngroup = \"g%d\"\nengine = \"postgres\"\ndsn = \"postgres://u@h/d\"\n\n", i, i)
	}
	a = groupedIn(t, many.String())
	feed(t, a, "<C-p>conn.new<CR>n<Tab>g"+strings.Repeat("<Down>", 9))
	find(t, a, ui.Target{Kind: ui.KindRow, I: 9, Action: "form.pick"}) // the tenth is drawn
}

// With every connection in a group the tree begins with a rule: the cursor of
// a window with nothing done is not on it (F10.6).
func TestTreeCursorStartsOnANode(t *testing.T) {
	a := groupedIn(t, `[[connection]]
name = "stage"
group = "prod"
engine = "postgres"
dsn = "postgres://u@h/s"
`)
	a.win().focus(0)
	if n, ok := a.treeNode(); !ok || n.kind != nodeConn {
		t.Errorf("the cursor is on %+v", n)
	}
}

// freshOrder is the order of the connections a start over the file gives.
func freshOrder(t *testing.T) string {
	t.Helper()
	specs, _, err := config.LoadConnections("")
	if err != nil {
		t.Fatal(err)
	}
	var cs []*Conn
	for _, s := range specs {
		cs = append(cs, newConn(s))
	}
	groupOrder(cs)
	var names []string
	for _, c := range cs {
		names = append(names, c.Spec.Name)
	}
	return strings.Join(names, " ")
}

// Deleting the first connection of the group the file has first makes another
// group the first: the session follows the file as a start does (F10.6).
func TestGroupDeleteRegroups(t *testing.T) {
	a := grouped(t)
	a.run("conn.manage", 0)
	a.selectConn(slices.IndexFunc(a.sess.Conns, func(c *Conn) bool { return c.Spec.Name == "stage" }))
	a.form.list = true
	feed(t, a, "xy")
	if got := connOrder(a); got != "doraemon misc ops live" || got != freshOrder(t) {
		t.Errorf("order %q, a start's %q", got, freshOrder(t))
	}
}

// A group edited moves the connection in every session, not only the one the
// form is in; one that is connected keeps its state and its tabs and says
// nothing of the next connect, as only its group changed (F10.6).
func TestGroupEditEverySession(t *testing.T) {
	a := grouped(t)
	var conns []*Conn
	for _, c := range a.sess.Conns {
		conns = append(conns, newConn(c.Spec))
	}
	s2 := bareSession("s2", conns, conns[0])
	s2.Windows = []*Window{s2.blankWindow()}
	a.sessions = append(a.sessions, s2)
	doraemon, state := a.sess.Conns[0], a.sess.Conns[0].State
	a.toast = ""
	a.run("conn.manage", 0)
	a.selectConn(slices.IndexFunc(a.sess.Conns, func(c *Conn) bool { return c.Spec.Name == "ops" }))
	feed(t, a, "<Tab><Tab><C-u>prod<Esc><C-s>")
	var names []string
	for _, c := range s2.Conns {
		names = append(names, c.Spec.Name)
	}
	if got := strings.Join(names, " "); got != connOrder(a) || got != freshOrder(t) {
		t.Errorf("the other session %q, this %q, a start's %q", got, connOrder(a), freshOrder(t))
	}
	a.selectConn(0)
	feed(t, a, "<Tab><Tab><C-u>prod<Esc><C-s>")
	if i := slices.Index(a.sess.Conns, doraemon); i < 0 || doraemon.State != state || doraemon.Spec.Group != "prod" {
		t.Errorf("the connection is lost or changed: at %d, %v", i, doraemon.State)
	}
	if strings.Contains(a.toast, "下次连接时生效") {
		t.Errorf("a group alone is not for the next connect: %q", a.toast)
	}
}

// With every connection in a group the tree begins with a rule: G and then gg
// show the first one with it, k at the top has nowhere to go (F10.6).
func TestTreeTopShowsItsRule(t *testing.T) {
	var b strings.Builder
	for i := range 60 {
		fmt.Fprintf(&b, "[[connection]]\nname = \"c%02d\"\ngroup = \"g%d\"\nengine = \"postgres\"\ndsn = \"postgres://u@h/d\"\n\n", i, i/30)
	}
	a := groupedIn(t, b.String())
	a.win().focus(0)
	feed(t, a, "Ggg")
	ns, _ := a.treeNodes()
	if _, top := a.treeAt(ns); top != 0 {
		t.Errorf("the first group's rule is out of sight: top %d", top)
	}
}

// A list of more rows and rules than the manager shows keeps the selection
// drawn going down and up through them (F10.6).
func TestManagerGroupsScroll(t *testing.T) {
	var b strings.Builder
	for i := range 12 {
		fmt.Fprintf(&b, "[[connection]]\nname = \"c%02d\"\ngroup = \"g%d\"\nengine = \"postgres\"\ndsn = \"postgres://u@h/d\"\n\n", i, i)
	}
	a := groupedIn(t, b.String())
	a.run("conn.manage", 0)
	a.selectConn(0)
	a.form.list = true
	drawn := func(step string) {
		a.View()
		for _, h := range a.hits {
			if h.Target.Kind == ui.KindConn && h.Target.I == a.form.cur {
				return
			}
		}
		t.Errorf("%s: the selection %d is not drawn (top %d)", step, a.form.cur, a.form.top)
	}
	for i := range 13 {
		feed(t, a, "j")
		drawn(fmt.Sprintf("j %d", i))
	}
	for i := range 13 {
		feed(t, a, "k")
		drawn(fmt.Sprintf("k %d", i))
	}
}

// A rule is a line in sep and its title in dim, in the tree and in the manager
// (F10.6).
func TestGroupRuleColors(t *testing.T) {
	check := func(name string, f *ui.Frame, area uv.Rectangle) {
		t.Helper()
		var lines, titles int
		for y := area.Min.Y; y < area.Max.Y; y++ {
			var row strings.Builder
			for x := area.Min.X; x < area.Max.X; x++ {
				row.WriteString(f.Buf.CellAt(x, y).Content)
			}
			if !strings.Contains(row.String(), "── prod ") {
				continue
			}
			for x := area.Min.X; x < area.Max.X; x++ {
				c := f.Buf.CellAt(x, y)
				switch {
				case c.Content == "─" && c.Style.Fg == f.Theme.Sep:
					lines++
				case slices.Contains([]string{"p", "r", "o", "d"}, c.Content) && c.Style.Fg == f.Theme.Dim:
					titles++
				}
			}
		}
		if lines < 10 || titles < 4 {
			t.Errorf("%s: %d line cells in sep, %d title cells in dim", name, lines, titles)
		}
	}
	a := grouped(t)
	a.sess.open["conn:doraemon"] = false
	check("tree", a.render(), a.sidebarRect())
	a.run("conn.manage", 0)
	f := a.render()
	var list uv.Rectangle
	for _, h := range f.Hits {
		if h.Target.Kind == ui.KindConn {
			list = list.Union(h.Rect)
		}
	}
	check("manager", f, list)
}

// A session that starts on a connection of a group starts on that one, the
// order the tree has moving it: the console of a layout with no window is on
// it, and the connections are in the tree's order (F10.6).
func TestRestoreStartsOnItsConnection(t *testing.T) {
	grouped(t) // the file
	specs, _, err := config.LoadConnections("")
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	r := Restore(specs, &config.Layout{Sessions: []config.LayoutSession{{Name: "s", Connection: "stage"}}})
	s := r.Sessions[0]
	if got := s.start.Spec.Name; got != "stage" {
		t.Errorf("start %s", got)
	}
	if cs := s.consoles(); len(cs) != 1 || cs[0].conn != s.start {
		t.Errorf("console of %+v", cs)
	}
	var names []string
	for _, c := range s.Conns {
		names = append(names, c.Spec.Name)
	}
	if got := strings.Join(names, " "); got != "doraemon misc stage live ops" {
		t.Errorf("the tree's order: %q", got)
	}
}
