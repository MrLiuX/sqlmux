package app

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/exp/golden"

	"github.com/kackerx/sqlmux/internal/config"
	"github.com/kackerx/sqlmux/internal/db"
	"github.com/kackerx/sqlmux/internal/editor"
	"github.com/kackerx/sqlmux/internal/ui"
)

// A run with a dangerous statement asks first, once, listing them; no is no
// run: no statement goes, no run is counted, no log line is made, no result
// area opens. Yes sends all of it. Reading statements and the ones with a
// WHERE do not ask; a run is asked again when it is run again; with
// confirm_dangerous off nothing asks (§13, F6.7).
func TestConfirmDangerousRun(t *testing.T) {
	d := &execDB{res: map[string]db.Result{"select 1": rows(1)}}
	a, c := inConsole(t, "", "")
	a.sess.Conns[0].Main = db.NewWorker(d)
	c.schema = ""
	a.confirmDangerous = true
	run := func() { // the whole text, as a selection
		t.Helper()
		feed(t, a, "ggVG")
		deliver(a, a.run("console.run", 0))
	}
	nothing := func(when string) {
		t.Helper()
		if len(d.ran) != 0 || a.sess.RunSeq != 0 || len(a.win().log) != 0 || a.win().Result != nil || c.running != nil {
			t.Errorf("%s: ran %q, run %d, log %d, result area %v", when, d.ran, a.sess.RunSeq, len(a.win().log), a.win().Result != nil)
		}
	}

	a.consoleDid(c, c.ed.Load("delete from t_log;\nselect 1;\ndrop table x;\ndelete from t where id = 1;"))
	run()
	if a.confirm == nil || !strings.HasPrefix(a.confirm.text, "这次执行有 2 条危险语句：\n  delete from t_log\n  drop table x") ||
		strings.Contains(a.confirm.text, "id = 1") || a.confirm.yes != "执行" {
		t.Fatalf("confirm: %+v", a.confirm)
	}
	nothing("while it asks")
	feed(t, a, "n")
	if a.confirm != nil {
		t.Error("n leaves it open")
	}
	nothing("no")
	run()
	feed(t, a, "<Esc>") // esc is no
	if a.confirm != nil {
		t.Error("esc leaves it open")
	}
	nothing("esc")

	run()
	press(t, a, "y")
	if a.confirm != nil || a.sess.RunSeq != 1 || len(ran(d)) != 4 {
		t.Fatalf("yes: confirm %v, run %d, ran %q", a.confirm, a.sess.RunSeq, ran(d))
	}

	// the run again, from its result tab, asks again
	a.focusPane(a.win().Result.ID)
	res := a.win().Result
	selectTab(res, slices.IndexFunc(res.Tabs, func(tb Tab) bool { return tb.Result != nil && tb.Result.run != nil }))
	deliver(a, a.run("result.rerun", 0))
	if a.confirm == nil || a.sess.RunSeq != 1 {
		t.Fatalf("rerun: %+v, run %d", a.confirm, a.sess.RunSeq)
	}
	press(t, a, "y")
	if a.sess.RunSeq != 2 {
		t.Errorf("rerun yes: run %d", a.sess.RunSeq)
	}

	// what does not ask
	a.focusPane(a.win().pane(2).ID)
	d.ran = nil
	a.consoleDid(c, c.ed.Load("select 1;\ndelete from t where id = 1;\nupdate t set a = 1 where b = 2;\nalter table t drop constraint c;"))
	run()
	if a.confirm != nil || len(ran(d)) != 4 {
		t.Errorf("safe statements: confirm %v, ran %q", a.confirm, ran(d))
	}

	// off
	a.confirmDangerous = false
	d.ran = nil
	a.consoleDid(c, c.ed.Load("delete from t_log"))
	run()
	if a.confirm != nil || len(ran(d)) != 1 {
		t.Errorf("confirm_dangerous off: confirm %v, ran %q", a.confirm, ran(d))
	}
}

// A read-only connection keeps the dangerous statements back as it did,
// without asking; a selection and a plain console's C-e are runs too.
func TestConfirmDangerousReadOnlyAndSelection(t *testing.T) {
	d := &execDB{}
	a, c := inConsole(t, "vim = false", "")
	a.sess.Conns[0].Main = db.NewWorker(d)
	c.schema = ""
	a.confirmDangerous = true
	a.consoleDid(c, c.ed.Load("select 1;\ndelete from t;"))
	c.ed.Goto(editor.Pos{})
	shiftArrow(a, tea.KeyDown, 1)
	press(t, a, "<C-e>") // only the select is selected
	if a.confirm != nil || len(ran(d)) != 1 {
		t.Fatalf("a selection of a read: confirm %v, ran %q", a.confirm, ran(d))
	}
	d.ran = nil
	c.ed.Goto(editor.Pos{})
	shiftArrow(a, tea.KeyRight, 24)
	press(t, a, "<C-e>")
	if a.confirm == nil || len(d.ran) != 0 {
		t.Fatalf("a selection of both: confirm %v, ran %q", a.confirm, d.ran)
	}
	feed(t, a, "n")

	a.sess.Conns[0].readOnly = true
	c.ed.Goto(editor.Pos{})
	shiftArrow(a, tea.KeyRight, 24)
	press(t, a, "<C-e>")
	if a.confirm != nil || len(d.ran) != 0 || len(a.win().log) == 0 {
		t.Errorf("read-only: confirm %v, ran %q, log %d", a.confirm, d.ran, len(a.win().log))
	}
}

// The box lists eight, each with its white space squeezed to a space and cut
// to 60 cells, and says how many more there are.
func TestDangerText(t *testing.T) {
	var risky []string
	for i := range 10 {
		risky = append(risky, fmt.Sprintf("delete\nfrom\tt%d", i))
	}
	risky[0] = "  drop table " + strings.Repeat("long_", 30)
	risky[2] = "delete from " + strings.Repeat("表", 40)
	lines := strings.Split(dangerText(risky), "\n")
	if len(lines) != 10 || lines[0] != "这次执行有 10 条危险语句：" || lines[2] != "  delete from t1" || lines[9] != "… 另有 2 条" {
		t.Fatalf("%q", lines)
	}
	for _, i := range []int{1, 3} {
		if w := ui.Width(strings.TrimPrefix(lines[i], "  ")); !strings.HasSuffix(lines[i], "…") || w < 59 || w > 60 { // a wide rune is not split
			t.Errorf("a long line is cut to 60 cells: %q is %d", lines[i], w)
		}
	}
}

// A connection the console waits for may land while the box asks, and the
// console goes over to it: yes then runs nothing, as it was asked about
// another connection (§13).
func TestConfirmDangerousConnLands(t *testing.T) {
	d := &execDB{}
	a, c := inConsole(t, "", "")
	a.sess.Conns[0].Main = db.NewWorker(d)
	c.schema = ""
	a.consoleDid(c, c.ed.Load("delete from t_log"))
	idle := newConn(config.Connection{Name: "idle", Engine: "postgres"})
	a.sess.Conns = append(a.sess.Conns, idle)
	c.want, idle.State = idle, connConnecting
	feed(t, a, "<CR>")
	if a.confirm == nil {
		t.Fatal("no question")
	}
	a.Update(connected{conn: idle, d: dialed{main: &execDB{}, meta: noDB{}, spec: idle.Spec}})
	if c.conn != idle {
		t.Fatalf("the console stays on %s", c.conn.Spec.Name)
	}
	_, cmd := a.Update(teaKey("y"))
	deliver(a, cmd)
	if len(d.ran) != 0 || a.sess.RunSeq != 0 || c.running != nil || a.toast == "" {
		t.Errorf("ran %q, run %d, toast %q", d.ran, a.sess.RunSeq, a.toast)
	}
}

// A save that deletes rows asks first, with what else it holds; no, and the
// table is as it was, nothing sent; a save of cells and new rows, and one
// with the question off, goes; one on its way is not asked about again, and
// :wq's close stays (§13, F6.7).
func TestConfirmDeletes(t *testing.T) {
	setup := func(conf string) (*App, *dataTab, *tagDB) {
		a := sizedWith(160, 45, userConfig(t, conf, ""))
		a.removePane(2)
		a.win().focus(1)
		tab := loadOrders(t, a, 3)
		d := &tagDB{}
		a.sess.Conns[0].Main = db.NewWorker(d)
		return a, tab, d
	}
	save := func(a *App) tea.Cmd { _, cmd := a.Update(teaKey("<C-s>")); return cmd }
	yes := func(a *App) {
		t.Helper()
		_, cmd := a.Update(teaKey("y"))
		if cmd == nil {
			t.Fatal("yes sends nothing")
		}
		a.Update(cmd())
	}

	a, tab, d := setup("")
	feed(t, a, "dd")
	save(a)
	if a.confirm == nil || a.confirm.text != "将删除 1 行，确定保存？" || a.confirm.yes != "保存" || len(d.sqls) != 0 {
		t.Fatalf("a delete: %+v, sent %q", a.confirm, d.sqls)
	}
	feed(t, a, "n")
	if a.confirm != nil || len(tab.deleted) != 1 || len(d.sqls) != 0 || tab.saving {
		t.Fatalf("no: confirm %v, marks %d, sent %q", a.confirm, len(tab.deleted), d.sqls)
	}
	feed(t, a, "jlix<Esc>") // a cell of another row as well
	save(a)
	if a.confirm == nil || a.confirm.text != "将删除 1 行（另有 1 处修改），确定保存？" {
		t.Fatalf("a delete and a change: %+v", a.confirm)
	}
	feed(t, a, "<Esc>")
	if len(tab.deleted) != 1 || tab.changes() != 2 || len(d.sqls) != 0 {
		t.Fatalf("esc: marks %d, changes %d, sent %q", len(tab.deleted), tab.changes(), d.sqls)
	}
	save(a)
	yes(a)
	if !strings.Contains(strings.Join(d.sqls, "\n"), "delete from") || a.confirm != nil {
		t.Errorf("yes: sent %q", d.sqls)
	}

	a, _, _ = setup("")
	feed(t, a, "lix<Esc>jo")
	if cmd := save(a); a.confirm != nil || cmd == nil {
		t.Errorf("a cell changed and a row added: confirm %v, sent %v", a.confirm, cmd != nil)
	}
	a, tab, _ = setup("")
	feed(t, a, "dd")
	a.run("tab.save.close", 0)
	press(t, a, "y") // on its way
	if cmd := save(a); a.confirm != nil || cmd != nil || !tab.saving || !tab.closing {
		t.Errorf("while it saves: confirm %v, cmd %v, closing %v", a.confirm, cmd != nil, tab.closing)
	}
	a, _, _ = setup("confirm_dangerous = false")
	feed(t, a, "dd")
	if cmd := save(a); a.confirm != nil || cmd == nil {
		t.Errorf("off: confirm %v, sent %v", a.confirm, cmd != nil)
	}
}

// :wq closes the tab once a save that asked has landed.
func TestConfirmDeletesThenClose(t *testing.T) {
	a := sizedWith(160, 45, userConfig(t, "", ""))
	a.removePane(2)
	a.win().focus(1)
	tab := loadOrders(t, a, 3)
	a.sess.Conns[0].Main = db.NewWorker(&tagDB{})
	feed(t, a, "dd")
	a.run("tab.save.close", 0)
	if a.confirm == nil || tab.saving {
		t.Fatalf("asks first: %+v", a.confirm)
	}
	_, cmd := a.Update(teaKey("y"))
	if !tab.closing {
		t.Error("the tab is to close when it lands")
	}
	a.Update(cmd())
	if p := a.paneShowing(tab); p != nil {
		t.Error("the tab is still there")
	}
}

// dangerScene is a console about to run two dangerous statements of three,
// asked about, on a 100 × 30 screen.
func dangerScene(t *testing.T) *App {
	t.Helper()
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	a := sized(100, 30, "nerd")
	a.win().focus(2)
	c := consoleOf(a.focused())
	a.sess.Conns[0].Main = db.NewWorker(&execDB{})
	c.schema = ""
	a.consoleDid(c, c.ed.Load("delete from t_log;\nselect 1;\ndrop table if exists archive_2024"))
	feed(t, a, "ggVG")
	a.run("console.run", 0)
	if a.confirm == nil {
		t.Fatal("no question")
	}
	return a
}

func TestGoldenDangerConfirm(t *testing.T) {
	golden.RequireEqual(t, dangerScene(t).render().String())
}
