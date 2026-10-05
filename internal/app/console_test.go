package app

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/exp/golden"

	"sqlmux/internal/config"
	"sqlmux/internal/db"
	"sqlmux/internal/editor"
	"sqlmux/internal/keymap"
	"sqlmux/internal/ui"
)

// inConsole is the default layout with focus on console_1 (⟨2⟩), whose
// file is in a directory of the test's own, with the user's config.toml
// conf and keymaps.toml keys.
func inConsole(t *testing.T, conf, keys string) (*App, *consoleTab) {
	t.Helper()
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	a := sizedWith(160, 45, userConfig(t, conf, keys))
	feed(t, a, "<C-l>")
	c := consoleOf(a.focused())
	if c == nil {
		t.Fatal("C-l from ⟨1⟩ is not on the console")
	}
	return a, c
}

func text(c *consoleTab) string { return strings.Join(c.ed.Lines(), "\n") }

// Keys the keymap leaves go to the editor; one waiting for more takes the
// next key itself, so f<Space>x is no SPC x (§6.4). : and ; are its vim's.
func TestConsoleKeys(t *testing.T) {
	a, c := inConsole(t, "", "")
	feed(t, a, "ione two three<CR>four<Esc>gg0f<Space>x")
	if text(c) != "onetwo three\nfour" || len(a.win().Root.Leaves()) != 2 {
		t.Fatalf("f<Space>x: %q, panes %d", text(c), len(a.win().Root.Leaves()))
	}
	feed(t, a, "0fe;")
	if c.ed.Cursor() != (editor.Pos{Line: 0, Col: 10}) {
		t.Errorf("; repeats f: %v", c.ed.Cursor())
	}
	feed(t, a, ":")
	if a.palette != nil || c.ed.Mode() != editor.Command {
		t.Fatalf(": opens the editor's command line: palette %v, mode %v", a.palette, c.ed.Mode())
	}
	feed(t, a, "2<CR>")
	if c.ed.Cursor().Line != 1 {
		t.Errorf(":2 goes to line 2: %v", c.ed.Cursor())
	}
	feed(t, a, "d<C-c>") // C-c is esc: no quit toast
	if c.ed.Pending() != "" || a.toast != "" {
		t.Errorf("d then C-c: pending %q, toast %q", c.ed.Pending(), a.toast)
	}
	feed(t, a, ":foo<CR>")
	if a.toast != "不支持的命令：foo" {
		t.Errorf("toast %q", a.toast)
	}
}

// The user's maps for the console apply there, over the editor's own keys;
// the right-hand side goes in as typed, meeting the editor as it then is
// (§6.6): after i, a space is typed, no leader.
func TestConsoleMaps(t *testing.T) {
	a, c := inConsole(t, "tab_width = 4", "[map.console.normal]\nL = \"5l\"\nQ = \"iselect x<Esc>\"\nX = \"f<Space>\"\n[map.console.visual]\nH = \"2h\"\n")
	feed(t, a, "Q")
	if text(c) != "select x" || c.ed.Mode() != editor.Normal || len(a.win().Root.Leaves()) != 2 {
		t.Fatalf("Q = iselect x<Esc>: %q in %v, panes %d", text(c), c.ed.Mode(), len(a.win().Root.Leaves()))
	}
	feed(t, a, "0Xxx")
	if text(c) != "select" || len(a.win().Root.Leaves()) != 2 {
		t.Fatalf("X = f<Space>, x, x: %q, panes %d", text(c), len(a.win().Root.Leaves()))
	}
	feed(t, a, "dd")
	feed(t, a, "iabcdefgh<Esc>0L")
	if c.ed.Cursor().Col != 5 {
		t.Errorf("L = 5l: col %d", c.ed.Cursor().Col)
	}
	if feed(t, a, "vH"); c.ed.Cursor().Col != 3 || c.ed.Mode() != editor.Visual {
		t.Errorf("H = 2h in VISUAL: col %d in %v", c.ed.Cursor().Col, c.ed.Mode())
	}
	feed(t, a, "<Esc>")
	feed(t, a, "o<Tab>x<Esc>")
	if c.ed.Lines()[1] != "    x" {
		t.Errorf("tab_width 4: %q", c.ed.Lines()[1])
	}
}

// The file (§11): :w writes it, 0600; unchanged it is not written; :q
// writes and closes; a new session has it back. The autosave writes only
// for the latest change.
func TestConsoleFile(t *testing.T) {
	a, c := inConsole(t, "", "")
	if c.path != config.ConsolePath(1) {
		t.Fatalf("path %s", c.path)
	}
	feed(t, a, "iselect 1;<Esc>")
	a.Update(autosave{c, c.ver - 1})
	if _, err := os.Stat(c.path); err == nil {
		t.Fatal("a stale autosave wrote")
	}
	a.Update(autosave{c, c.ver})
	if data, _ := os.ReadFile(c.path); string(data) != "select 1;\n" {
		t.Fatalf("autosave: %q", data)
	}
	os.Remove(c.path)
	feed(t, a, ":w<CR>")
	if _, err := os.Stat(c.path); err == nil {
		t.Error("written with nothing changed")
	}
	feed(t, a, "A -- one<Esc>:w<CR>")
	info, err := os.Stat(c.path)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf(":w: %v %v", info, err)
	}
	feed(t, a, "o2<Esc>:wq<CR>")
	if data, _ := os.ReadFile(c.path); string(data) != "select 1; -- one\n2\n" || len(a.win().Root.Leaves()) != 1 {
		t.Fatalf(":wq: %q, panes %d", data, len(a.win().Root.Leaves()))
	}
	if s := soloSession(); text(consoleOf(s.Windows[0].Root.B.Pane)) != "select 1; -- one\n2" || s.warning != "" {
		t.Fatalf("a new session: %q", s.warning)
	}
}

// A console runs on the connection its title's dropdown picks (§7.8
// 「console 标题」): only that part lights under the pointer; a connected
// one at once, in its Schema, kept for the file (§11); not while its own
// is in a transaction; one not connected once it connects, the console
// staying meanwhile, and on, it failing.
func TestConsoleConn(t *testing.T) {
	a, c := inConsole(t, "", "")
	seed := readyConn("seed")
	seed.Schema, seed.Schemas, seed.DB = "s1", []string{"s1", "s2"}, "seeddb"
	idle := newConn(config.Connection{Name: "idle", Engine: "postgres"})
	a.sess.Conns = append(a.sess.Conns, seed, idle)
	r := find(t, a, ui.Target{Kind: ui.KindHint, Pane: a.focused().ID, Action: "console.conn"})
	a.Update(tea.MouseMotionMsg{X: r.Min.X, Y: r.Min.Y})
	if f := a.render(); f.Buf.CellAt(r.Max.X-1, r.Min.Y).Style.Bg != a.theme.Select || f.Buf.CellAt(r.Max.X+1, r.Min.Y).Style.Bg == a.theme.Select {
		t.Error("the pointer on the connection lights it alone, not the ·")
	}
	click(a, r.Min)
	if v := a.dropView(); a.drop == nil || strings.Join(v.Items, " ") != "doraemon seed idle" || v.Mark != 0 {
		t.Fatalf("dropdown %+v", a.drop)
	}
	feed(t, a, "seed<CR>")
	if c.conn != seed || c.schema != "s1" || a.state.Consoles["console_1"] != (config.ConsoleState{Conn: "seed", Schema: "s1"}) {
		t.Fatalf("seed: on %s.%s, state %+v", c.conn.Spec.Name, c.schema, a.state.Consoles)
	}
	if top := strings.Split(a.render().String(), "\n")[r.Min.Y]; !strings.Contains(top, "seed · seeddb.s1 ▾") {
		t.Errorf("title %q", top)
	}
	feed(t, a, "gs<C-n><CR>")
	if c.schema != "s2" || a.state.Consoles["console_1"].Schema != "s2" {
		t.Errorf("schema %s, kept %+v", c.schema, a.state.Consoles)
	}
	seed.tx = 'T'
	a.run("console.conn", 0)
	if feed(t, a, "doraemon<CR>"); c.conn != seed || a.toast != "当前连接有未提交的事务，先提交或回滚" {
		t.Errorf("in a transaction: on %s, toast %q", c.conn.Spec.Name, a.toast)
	}
	seed.tx, c.running = 'I', &run{}
	a.run("console.conn", 0)
	if feed(t, a, "doraemon<CR>"); c.conn != seed || a.toast != "正在执行，结束后再换连接" {
		t.Errorf("running: on %s, toast %q", c.conn.Spec.Name, a.toast)
	}
	c.running = nil
	a.run("console.conn", 0)
	if feed(t, a, "idle<CR>"); c.conn != seed || c.want != idle || idle.State != connConnecting {
		t.Fatalf("idle: on %s, want %v, %v", c.conn.Spec.Name, c.want, idle.State)
	}
	if a.Update(connected{conn: idle, err: errors.New("no route")}); c.conn != seed || c.want != nil || a.toast != "idle：no route" {
		t.Errorf("idle failing: on %s, want %v, toast %q", c.conn.Spec.Name, c.want, a.toast)
	}
}

// console_1 of the layout started with opens where its file last ran:
// on a connection connected, now; on one not, once it connects; a schema
// gone, the connection's once its catalog is in. A new console goes on the
// current connection, in its Schema, kept for its file (§11, F5.11).
func TestConsoleRestore(t *testing.T) {
	a, c1 := inConsole(t, "", "")
	seed, idle := readyConn("seed"), newConn(config.Connection{Name: "idle", Engine: "postgres"})
	seed.Schema = "s1"
	a.sess.Conns = append(a.sess.Conns, seed, idle)
	a.state.Consoles = map[string]config.ConsoleState{"console_1": {Conn: "seed", Schema: "dropped"}, "console_2": {Conn: "idle", Schema: "x"}}
	if a.Init(); c1.conn != seed || c1.schema != "dropped" {
		t.Errorf("console_1 at start: on %s.%s", c1.conn.Spec.Name, c1.schema)
	}
	a.Update(catalogMsg{conn: seed, schemas: []string{"s1"}, current: "s1"})
	if c1.schema != "s1" || a.state.Consoles["console_1"].Schema != "s1" {
		t.Errorf("its schema gone: %s, kept %+v", c1.schema, a.state.Consoles["console_1"])
	}
	a.run("console.new", 0)
	c := consoleOf(a.focused())
	if c.conn != seed || c.schema != "s1" || c.want != nil || idle.State != connIdle || a.state.Consoles["console_2"] != (config.ConsoleState{Conn: "seed", Schema: "s1"}) {
		t.Errorf("console_2: on %s.%s, want %v, kept %+v", c.conn.Spec.Name, c.schema, c.want, a.state.Consoles["console_2"])
	}
}

// soloSession is a new session over doraemon alone, not connected.
func soloSession() *Session {
	c := newConn(config.Connection{Name: "doraemon", Engine: "postgres"})
	return newSession("doraemon", []*Conn{c}, c, 1)
}

// A console file that cannot be read is not opened, empty or not: a toast
// says so and the rest starts (§11).
func TestConsoleUnreadable(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	if err := os.MkdirAll(config.ConsolePath(1), 0o700); err != nil {
		t.Fatal(err)
	} // a directory where the file goes
	s := soloSession()
	if p := s.Windows[0].Root.B.Pane; len(p.Tabs) != 0 || !strings.HasPrefix(s.warning, "读取失败：") {
		t.Fatalf("tabs %v, warning %q", p.Tabs, s.warning)
	}
	c := config.Default()
	keys, _ := keymap.New(c)
	if a := New(c, keys, s, &config.State{}, "careful"); a.warning != "careful；"+s.warning {
		t.Errorf("warning %q", a.warning)
	}
}

// A console that cannot be written says so, and neither closes nor quits.
func TestConsoleSaveFails(t *testing.T) {
	a, c := inConsole(t, "", "")
	c.path = filepath.Join(os.DevNull, "console_1.sql")
	feed(t, a, "ix<Esc>:w<CR>")
	if !strings.HasPrefix(a.toast, "保存失败：") {
		t.Fatalf("toast %q", a.toast)
	}
	if feed(t, a, ":q<CR>"); consoleOf(a.focused()) != c {
		t.Error(":q closed what was not written")
	}
	if feed(t, a, "<C-p>>qa<CR>") {
		t.Error("quit with the console not written")
	}
	c.path = filepath.Join(t.TempDir(), "console_1.sql")
	if !feed(t, a, "<C-p>>qa<CR>") {
		t.Fatal("quit")
	}
	if data, _ := os.ReadFile(c.path); string(data) != "x\n" {
		t.Errorf("quitting writes the console: %q", data)
	}
}

// A paste is text in every mode, never keys (§11); a yank into "+ goes to
// the clipboard, a plain one not; "+p asks the terminal for it and puts
// what comes back (F3.38).
func TestConsolePasteAndYank(t *testing.T) {
	a, c := inConsole(t, "", "")
	feed(t, a, "iab<Esc>0")
	a.Update(tea.PasteMsg{Content: "dd\r\nx"})
	if text(c) != "add\nxb" || c.ed.Mode() != editor.Normal {
		t.Fatalf("pasted in NORMAL: %q %v", text(c), c.ed.Mode())
	}
	if clipped(keys(t, a, "yy"), "xb\n") {
		t.Error("yy reaches the clipboard")
	}
	if !clipped(keys(t, a, `"+yy`), "xb\n") {
		t.Error(`"+yy does not reach the clipboard`)
	}
	if cmd := keys(t, a, `"+p`); cmd == nil || text(c) != "add\nxb" {
		t.Fatalf(`"+p before the clipboard comes: %q`, text(c))
	}
	if a.Update(tea.ClipboardMsg{Content: "clip"}); text(c) != "add\nxclipb" {
		t.Errorf(`"+p: %q`, text(c))
	}
}

// The status bar: the editor's mode by name in VISUAL's or INSERT's color,
// the selection's size with the key that runs it, what the editor waits
// on (§7.8).
func TestConsoleStatusBar(t *testing.T) {
	a, _ := inConsole(t, "", "")
	feed(t, a, "iabc<CR>def<Esc>gg")
	th := a.theme
	for _, c := range []struct {
		keys, mode, info string
		bg               any
	}{
		{"vl", "VISUAL", "2 字符 · ↵ run", th.Keyword},
		{"<Esc>Vj", "V-LINE", "2 行 · ↵ run", th.Keyword},
		{"<Esc>gg<C-v>jl", "V-BLOCK", "2 行 × 2 列 · ↵ run", th.Keyword},
		{"<Esc>R", "REPLACE", "", th.Warn},
		{"<Esc>:", "COMMAND", "", th.Info},
		{"<Esc>2d", "NORMAL", "", th.Focus},
	} {
		feed(t, a, c.keys)
		f := a.render()
		bar := strings.Split(f.String(), "\n")[44]
		if st := styleOf(t, f, " "+c.mode+" "); !strings.Contains(bar, " "+c.mode+" ") || st.Bg != c.bg {
			t.Errorf("%s: %q, bg %v", c.keys, bar, st.Bg)
		}
		if c.info != "" && !strings.Contains(bar, c.info) {
			t.Errorf("%s: no %q in %q", c.keys, c.info, bar)
		}
	}
	if bar := strings.Split(a.render().String(), "\n")[44]; !strings.Contains(bar, " 2d ") {
		t.Errorf("showcmd: %q", bar)
	}
}

// The terminal cursor is vim's: a block, a bar in INSERT, a line under in
// REPLACE (§11).
func TestConsoleCursorShape(t *testing.T) {
	a, _ := inConsole(t, "", "")
	for _, c := range []struct {
		keys string
		want tea.CursorShape
	}{{"", tea.CursorBlock}, {"i", tea.CursorBar}, {"<Esc>R", tea.CursorUnderline}, {"<Esc>:", tea.CursorBlock}} {
		feed(t, a, c.keys)
		if v := a.View(); v.Cursor == nil || v.Cursor.Shape != c.want {
			t.Errorf("%q: cursor %+v", c.keys, v.Cursor)
		}
	}
}

// A click puts the cursor there, a drag selects, the wheel scrolls the
// console under the pointer without taking the focus (§11).
func TestConsoleMouse(t *testing.T) {
	a, c := inConsole(t, "", "")
	feed(t, a, "i"+strings.Repeat("select 1;<CR>", 60)+"<Esc>gg")
	feed(t, a, "<C-h>") // focus back on ⟨1⟩
	text := c.ed.Top()
	wheel := func() {
		r := a.layout()[2]
		a.Update(tea.MouseWheelMsg{X: r.Min.X + 10, Y: r.Min.Y + 5, Button: tea.MouseWheelDown})
	}
	if wheel(); c.ed.Top() != text+wheelStep || a.win().Focus != 1 {
		t.Fatalf("wheel: top %d, focus %d", c.ed.Top(), a.win().Focus)
	}
	view, body := a.consoleView(a.win().pane(2), c)
	at := view.TextArea(body).Min
	click(a, uv.Pos(at.X+3, at.Y+1))
	if a.win().Focus != 2 || c.ed.Cursor() != (editor.Pos{Line: c.ed.Top() + 1, Col: 3}) {
		t.Fatalf("click: focus %d, cursor %v", a.win().Focus, c.ed.Cursor())
	}
	a.Update(tea.MouseClickMsg{X: at.X, Y: at.Y, Button: tea.MouseLeft})
	a.Update(tea.MouseMotionMsg{X: at.X + 2, Y: at.Y + 1, Button: tea.MouseLeft})
	a.Update(tea.MouseReleaseMsg{X: at.X + 2, Y: at.Y + 1, Button: tea.MouseLeft})
	from, to, ok := c.ed.Selection()
	if !ok || from != (editor.Pos{Line: c.ed.Top(), Col: 0}) || to != (editor.Pos{Line: c.ed.Top() + 1, Col: 2}) {
		t.Errorf("drag: %v %v %v", from, to, ok)
	}
	if a.Update(tea.MouseMotionMsg{X: at.X + 5, Y: at.Y + 1}); c.ed.Cursor().Col != 2 {
		t.Error("moving after the release selects")
	}
}

// $EDITOR edits the file, written first; what it holds after is one undo
// step (§11).
func TestConsoleExternal(t *testing.T) {
	a, c := inConsole(t, "", "")
	feed(t, a, "iselect 1<Esc>")
	if a.run("console.external", 0) == nil {
		t.Fatal("no editor run")
	}
	if data, _ := os.ReadFile(c.path); string(data) != "select 1\n" {
		t.Fatalf("not written first: %q", data)
	}
	if err := os.WriteFile(c.path, []byte("select 2;\nselect 3;\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	a.Update(externalDone{c, nil})
	if text(c) != "select 2;\nselect 3;" || c.saved != c.ver {
		t.Fatalf("read back: %q, saved %d of %d", text(c), c.saved, c.ver)
	}
	if feed(t, a, "u"); text(c) != "select 1" {
		t.Errorf("u: %q", text(c))
	}
}

// A keyword taken in a console gets a space, and the DML starts are
// offered first (§9.7); a word typed out is the first, a table's name not
// taking its place, a keyword of two words not put in twice; one undo takes
// it back with the INSERT.
func TestConsoleKeywordSpace(t *testing.T) {
	a, c := inConsole(t, "", "")
	for in, first := range map[string]string{"ins": "insert into", "upd": "update", "del": "delete from"} {
		feed(t, a, "<Esc>ggdGi"+in)
		if c.comp == nil || c.comp.items[0].label != first {
			t.Fatalf("%s: %+v", in, c.comp)
		}
	}
	for in, want := range map[string]string{
		"selec":                     "select ",
		"SELECT":                    "SELECT ",
		"update t_order set":        "update t_order set ",
		"alter table t rename to":   "alter table t rename to ",
		"insert into":               "insert into ",
		"select 1 from t union all": "select 1 from t union all ",
		"insert into t_order (id) values (1) on conflict (id) do update set": "insert into t_order (id) values (1) on conflict (id) do update set ",
	} {
		feed(t, a, "<Esc>ggdGi"+in+"<CR>")
		if text(c) != want || c.ed.Cursor().Col != len(want) {
			t.Errorf("%s ↵: %q at %v", in, text(c), c.ed.Cursor())
		}
	}
	feed(t, a, "<Esc>ggdGiinsert in")
	i := slices.IndexFunc(c.comp.items, func(cd candidate) bool { return cd.label == "insert into" })
	if i < 0 {
		t.Fatalf("insert in: %+v", c.comp.items)
	}
	c.comp.sel = i
	if feed(t, a, "<CR>"); text(c) != "insert into " {
		t.Errorf("insert in, insert into taken: %q", text(c))
	}
	feed(t, a, "<Esc>u")
	if text(c) != "" {
		t.Errorf("u: %q", text(c))
	}
}

// INSERT INTO t ( lists the columns the parentheses lack, all of them first,
// and a generated one never; taking all of them leaves the cursor before the
// ) (§9.7).
func TestConsoleInsertColumns(t *testing.T) {
	a, c := inConsole(t, "", "")
	table, cols, _ := ordersTable(0)
	cols.Cols[2].Generated = true
	for i, tb := range a.sess.Conns[0].Tables {
		if tb.Name == "t_order" {
			a.sess.Conns[0].Tables[i] = table
		}
	}
	a.sess.Conns[0].cols[idOf(table)] = cols
	feed(t, a, "iinsert into t_order(")
	if c.comp == nil || len(c.comp.items) != 7 || c.comp.items[0].note != "全部 6 列" || !strings.HasPrefix(c.comp.items[0].label, "id, status, paid") {
		t.Fatalf("after (: %+v", c.comp)
	}
	feed(t, a, "<CR>")
	if text(c) != "insert into t_order(id, status, paid, meta, note, created_at)" || c.ed.Cursor().Col != len(text(c))-1 {
		t.Fatalf("all: %q at %v", text(c), c.ed.Cursor())
	}
	feed(t, a, "<Esc>ggdGiinsert into t_order(id")
	feed(t, a, ", ")
	if c.comp == nil || len(c.comp.items) != 6 || c.comp.items[0].label == "id" || c.comp.items[1].label != "status" {
		t.Fatalf("after a comma: %+v", c.comp)
	}
	feed(t, a, "s")
	if c.comp == nil || c.comp.items[0].label != "status" || strings.HasPrefix(c.comp.items[0].note, "全部") {
		t.Fatalf("a word typed: no all-columns item: %+v", c.comp)
	}
	feed(t, a, "<Esc>ggdGiinsert into t_order(id, status, paid, meta, note, ")
	if c.comp == nil || len(c.comp.items) != 1 || c.comp.items[0].label != "created_at" {
		t.Fatalf("one column left: %+v", c.comp)
	}
	cols.Cols = append(cols.Cols, db.Column{Name: "x"})
	a.sess.Conns[0].cols[idOf(table)] = cols
	feed(t, a, "<Esc>ggdGiinsert into t_order(")
	if all := c.comp.items[0]; c.comp == nil || all.note != "全部 7 列" || all.label != "id, status, paid, meta, note, created_a…" || all.insert != "id, status, paid, meta, note, created_at, x" {
		t.Fatalf("41+ characters: %q, inserts %q", all.label, all.insert)
	}
}

// In a console the candidates follow the cursor's place (§9.7): tables
// after FROM; an alias's columns right after its ., fetched once and
// completed again as they come; CTE names among the tables; C-n with
// nothing typed. ↵ takes one as typing it would, part of the INSERT's undo
// step, and is a newline when that changes nothing; esc closes the list
// and leaves INSERT, as nvim-cmp; a character not of a name closes it.
func TestConsoleCompletion(t *testing.T) {
	a, c := inConsole(t, "", "")
	table, cols, _ := ordersTable(0)
	for i, tb := range a.sess.Conns[0].Tables {
		if tb.Name == "t_order" {
			a.sess.Conns[0].Tables[i] = table
		}
	}
	feed(t, a, "iselect * from t_o")
	if c.comp == nil || c.comp.items[0].label != "t_order" || a.context().Overlay != "complete" {
		t.Fatalf("after from: %+v, overlay %q", c.comp, a.context().Overlay)
	}
	feed(t, a, "<CR>")
	if text(c) != "select * from t_order" || c.comp != nil || c.ed.Mode() != editor.Insert {
		t.Fatalf("↵ takes t_order: %q in %v", text(c), c.ed.Mode())
	}
	feed(t, a, " o where o.")
	if !c.asked[idOf(table)] || c.comp != nil {
		t.Fatalf("o.: asked %v, list %+v", c.asked, c.comp)
	}
	a.Update(colsMsg{a.sess.Conns[0], table, cols, nil})
	if c.comp == nil || c.comp.items[0].label != "id" || c.comp.items[0].note != "bigint · t_order" {
		t.Fatalf("the columns in: %+v", c.comp)
	}
	feed(t, a, "st<CR>")
	if text(c) != "select * from t_order o where o.status" {
		t.Fatalf("o.st ↵: %q", text(c))
	}
	feed(t, a, "s<CR>")
	if text(c) != "select * from t_order o where o.statuss\n" {
		t.Fatalf("no candidate: ↵ is a newline: %q", text(c))
	}
	feed(t, a, "<Esc>u")
	if text(c) != "" {
		t.Fatalf("u takes back the INSERT, what was taken with it: %q", text(c))
	}
	feed(t, a, "iwith recent as (select 1) select * from ")
	if c.comp != nil {
		t.Fatal("a blank opens nothing")
	}
	feed(t, a, "<C-n>")
	if c.comp == nil || c.comp.items[0].label != "recent" || c.comp.items[0].note != "CTE" {
		t.Fatalf("C-n: %+v", c.comp)
	}
	feed(t, a, "<Esc>")
	if c.comp != nil || c.ed.Mode() != editor.Normal {
		t.Fatalf("esc: list %+v in %v", c.comp, c.ed.Mode())
	}
	feed(t, a, "At_o;")
	if c.comp != nil {
		t.Error("; closes it")
	}
}

// X. lists an alias's or a table's columns, the one nearest the cursor's
// depth; a schema's tables after a schema; nothing for a subquery's alias
// out of it (§9.7, lazysql's resolveAliases).
func TestSQLCompleteQualifier(t *testing.T) {
	a := sized(160, 45, "nerd")
	table, cols, _ := ordersTable(0)
	a.sess.Conns[0].cols[idOf(table)] = cols
	a.sess.Conns[0].cols[tableID{"public", "t_user"}] = db.Columns{Cols: []db.Column{{Name: "email", Type: "text"}}}
	a.sess.Conns[0].cols[tableID{"agentable", "planner"}] = db.Columns{Cols: []db.Column{{Name: "goal", Type: "text"}}}
	for _, c := range []struct{ sql, first string }{
		{"select * from agentable.|", "planner"},
		{"select * from t_order join agentable.| on", "planner"},
		{"select planner.| from agentable.planner", "goal"},
		{"select * from t_order o where exists (select 1 from t_user o where o.|", "email"},
		{"select * from t_order o where o.| in (select 1 from t_user o)", "id"},
		{"select a.| from (select * from t_user a) sub", ""},
	} {
		pos := strings.Index(c.sql, "|")
		got, _ := a.sqlComplete(a.sess.Conns[0], strings.Replace(c.sql, "|", "", 1), pos, "public", map[tableID]bool{}, true)
		first := ""
		if got != nil {
			first = got.items[0].label
		}
		if first != c.first {
			t.Errorf("%s: %q, want %q", c.sql, first, c.first)
		}
	}
}

// A console's schema (§8.6): on its title, gs or a click opens the
// dropdown there, its own schema marked, ↵ picks; a new console takes the
// tree's, then each keeps its own; completion lists its schema's tables.
func TestConsoleSchema(t *testing.T) {
	a, c := inConsole(t, "", "")
	title := func() string { return strings.Split(a.render().String(), "\n")[0] }
	if !strings.Contains(title(), " doraemon.public ▾  ▶ run ") {
		t.Fatalf("title %q", title())
	}
	feed(t, a, "gs")
	if a.drop == nil || a.drop.kind != dropSchema || a.dropView().Mark != 1 {
		t.Fatalf("gs: %+v", a.drop)
	}
	feed(t, a, "age<CR>")
	if c.schema != "agentable" || a.drop != nil || !strings.Contains(title(), " doraemon.agentable ▾ ") {
		t.Fatalf("picked: %q, title %q", c.schema, title())
	}
	click(a, find(t, a, ui.Target{Kind: ui.KindHint, Pane: a.focused().ID, Action: "console.schema"}).Min)
	if a.drop == nil || a.drop.console != c {
		t.Fatal("a click opens it")
	}
	// under the button as this frame draws it, to the pane's right border;
	// at that border with the button left out
	for _, w := range []int{130, 160, 110} {
		a.Update(tea.WindowSizeMsg{Width: w, Height: 45})
		f, r := a.render(), a.layout()[a.focused().ID]
		x, y := r.Max.X-16, r.Min.Y+1 // 16: its least width, more than its items need
		for _, h := range f.Hits {
			if h.Target.Action == "console.schema" {
				x = h.Rect.Min.X
			}
		}
		if f.Buf.CellAt(x, y).Content != "┌" || f.Buf.CellAt(r.Max.X-1, y).Content != "┐" {
			t.Errorf("%d wide: the box's corners at %d, %d on row %d:\n%s", w, x, r.Max.X-1, y, f.String())
		}
	}
	feed(t, a, "<Esc>")
	a.run("console.new", 0)
	if c2 := consoleOf(a.focused()); c2 == c || c2.schema != "public" {
		t.Fatalf("a new console: %q", c2.schema)
	}
	selectTab(a.focused(), 0)
	feed(t, a, "iselect * from pl")
	if c.comp == nil || c.comp.items[0].label != "planner" {
		t.Errorf("agentable's tables: %+v", c.comp)
	}
}

// A console with no schema yet, or one gone, takes the tree's when the
// catalog comes in (§8.6).
func TestConsoleSchemaFromCatalog(t *testing.T) {
	a, c := inConsole(t, "", "")
	for _, was := range []string{"", "gone"} {
		c.schema = was
		a.Update(catalogMsg{conn: a.sess.Conns[0], schemas: []string{"agentable", "public"}, current: "public", tables: a.sess.Conns[0].Tables})
		if c.schema != "public" {
			t.Errorf("%q: %q", was, c.schema)
		}
	}
	c.schema = "agentable"
	if a.Update(catalogMsg{conn: a.sess.Conns[0], schemas: []string{"agentable", "public"}, current: "public", tables: a.sess.Conns[0].Tables}); c.schema != "agentable" {
		t.Errorf("its own: %q", c.schema)
	}
}

// The console's title as the window narrows: ▶ run stays longest, then
// the connection and the schema, together, then ↵ (§7.8); at 200 columns
// everything and the name.
func TestGoldenConsoleTitle(t *testing.T) {
	var titles []string
	for _, w := range []int{200, 160, 130, 110, 100, 90} {
		a, _ := inConsole(t, "", "")
		a.Update(tea.WindowSizeMsg{Width: w, Height: 20})
		r := a.layout()[a.focused().ID]
		titles = append(titles, ansi.Cut(strings.Split(a.render().String(), "\n")[r.Min.Y], r.Min.X, r.Max.X))
	}
	golden.RequireEqual(t, strings.Join(titles, "\n")+"\n")
}

// The console's candidates open under the word they complete (§9.7).
func TestGoldenConsoleCompletion160x45(t *testing.T) {
	a, _ := inConsole(t, "", "")
	feed(t, a, "iselect 1;<CR>select * from t_o")
	golden.RequireEqual(t, a.render().String())
}

// What the console draws is the editor's state: the selection by kind,
// the command line.
func TestConsoleView(t *testing.T) {
	a, c := inConsole(t, "", "")
	c.ed.Load("ab\nc\td")
	feed(t, a, "Gfd")
	view := func() ui.Console { v, _ := a.consoleView(a.focused(), c); return v }
	if v := view(); v.Cursor != (ui.TextPos{Line: 1, Col: 2}) || v.CursorCol != 2 || !v.Normal || v.Sel.Mode != ui.SelNone || v.Prompt != "" {
		t.Errorf("NORMAL: %+v", v)
	}
	for _, s := range []struct {
		keys string
		want ui.Sel
	}{
		{"kv", ui.Sel{Mode: ui.SelChars, From: ui.TextPos{Line: 0, Col: 1}, To: ui.TextPos{Line: 0, Col: 1}}},
		{"<Esc>Vj", ui.Sel{Mode: ui.SelLines, From: ui.TextPos{Line: 0, Col: 1}, To: ui.TextPos{Line: 1, Col: 2}}},
		{"<Esc>gg<C-v>j", ui.Sel{Mode: ui.SelBlock, From: ui.TextPos{Line: 0, Col: 1}, To: ui.TextPos{Line: 1, Col: 2}, Left: 1, Right: 2}}, // b to d, past the tab
	} {
		feed(t, a, s.keys)
		if v := view(); v.Sel != s.want || v.Normal {
			t.Errorf("%s: %+v", s.keys, v.Sel)
		}
	}
	feed(t, a, "<Esc>:%s")
	if v := view(); v.Prompt != ":" || v.Text != "%s" || v.Pos != 2 {
		t.Errorf("command line: %+v", v)
	}
}
