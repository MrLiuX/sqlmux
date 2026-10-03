package app

import (
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/exp/golden"

	"sqlmux/internal/db"
	"sqlmux/internal/ui"
)

// Closing the last console on a connection in a transaction asks first,
// and y rolls it back once it is closed; one of two closes as before.
// Quitting asks for every connection in one, both sentences in one box
// with changes too (§10.5).
func TestCloseAsksForTx(t *testing.T) {
	a, c := inConsole(t, "", "")
	main := &saveDB{}
	c.conn.Main, c.conn.tx = db.NewWorker(main), 'T'
	a.run("console.new", 0)
	if feed(t, a, "<Esc>"); a.run("tab.close", 0) != nil || a.confirm != nil || len(a.sess.consoles()) != 1 {
		t.Fatalf("one of two: %+v, %d consoles", a.confirm, len(a.sess.consoles()))
	}
	a.run("tab.close", 0)
	if a.confirm == nil || a.confirm.text != "doraemon 有未提交的事务，关闭会回滚。" {
		t.Fatalf("the last: %+v", a.confirm)
	}
	if feed(t, a, "y"); len(a.sess.consoles()) != 0 || !slices.Contains(main.sqls, "rollback []") {
		t.Fatalf("y: %d consoles, sent %q", len(a.sess.consoles()), main.sqls)
	}

	a = sized(160, 45, "nerd")
	tab := loadOrders(t, a, 3)
	a.sess.Conns[0].tx = 'E'
	if feed(t, a, "lix<Esc>:qa<CR>"); a.confirm == nil || a.confirm.text != "有 1 处修改未保存，退出会丢弃。doraemon 有未提交的事务，退出会回滚。" {
		t.Fatalf("quit: %+v", a.confirm)
	}
	if feed(t, a, "<Esc>"); len(tab.edits) != 1 {
		t.Error("esc keeps them")
	}
}

// The mode goes back to auto only out of a transaction, from the palette
// or the title's dropdown (§5); a commit or a rollback loads the
// connection's tables again, a commit of a failed transaction saying it
// rolled back (§7.8).
func TestTxMode(t *testing.T) {
	a := sized(160, 45, "nerd")
	tab := loadOrders(t, a, 3)
	conn := tab.conn
	if a.run("tx.mode", 0); !conn.Manual || !actions["tx.mode"].On(a) {
		t.Fatal("tx.mode: manual")
	}
	conn.tx = 'T'
	if a.run("tx.mode", 0); !conn.Manual || a.toast != txOpen {
		t.Fatalf("in a transaction: manual %v, toast %q", conn.Manual, a.toast)
	}
	seq := tab.seq
	a.Update(txDone{conn: conn, sql: "commit", tag: "ROLLBACK", tx: 'I'})
	if conn.tx != 'I' || a.toast != "事务已出错，提交改为了回滚" || tab.seq != seq+1 {
		t.Fatalf("committed a failed one: %c, toast %q, seq %d → %d", conn.tx, a.toast, seq, tab.seq)
	}
	if a.run("tx.mode", 0); conn.Manual {
		t.Error("out of a transaction: auto")
	}
	if a.run("tx.commit", 0) != nil {
		t.Error("a commit out of a transaction")
	}

	a, c := inConsole(t, "", "")
	a.Update(tea.WindowSizeMsg{Width: 200, Height: 45})
	c.conn.Manual, c.conn.tx = true, 'T'
	click(a, find(t, a, ui.Target{Kind: ui.KindHint, Pane: a.focused().ID, Action: "console.tx"}).Min)
	if v := a.dropView(); strings.Join(v.Items, "|") != "自动|手动" || v.Mark != 1 {
		t.Fatalf("the dropdown: %q, mark %d", v.Items, v.Mark)
	}
	if a.dropPick(0); !c.conn.Manual || a.toast != txOpen {
		t.Errorf("auto picked in a transaction: %v, %q", c.conn.Manual, a.toast)
	}
}

// On a read-only connection a run with a statement BreaksReadOnly takes is
// no run: the log says so, shown; others run (§13). The grid takes no
// change (§10.1); the connection dropdown marks it with the lock.
func TestReadOnly(t *testing.T) {
	a, c := inConsole(t, "", "")
	c.conn.Spec.ReadOnly = true
	seq := a.sess.RunSeq
	feed(t, a, "iselect 1; delete from t_order<Esc>V")
	a.run("console.run", 0)
	log := a.win().log
	if c.running != nil || a.sess.RunSeq != seq || len(log) == 0 || !log[len(log)-1].Err ||
		!strings.HasSuffix(log[len(log)-1].Head, "delete from t_order  ") || log[len(log)-1].Tail != "doraemon 是只读连接，整次都没有执行" || resultOf(a.win().Result).run != nil {
		t.Fatalf("kept back: running %v, seq %d → %d, log %+v", c.running, seq, a.sess.RunSeq, log)
	}
	feed(t, a, "<Esc>ggcGset search_path to public; select 1<Esc>V")
	if a.run("console.run", 0); c.running == nil {
		t.Error("set search_path is kept back")
	}
	a.openConsoleDrop(dropConsoleConn)
	if v := a.dropView(); len(v.Notes) != 1 || v.Notes[0] != a.icons.Lock.Text {
		t.Errorf("the dropdown's notes: %q", v.Notes)
	}

	a = sized(160, 45, "nerd")
	tab := loadOrders(t, a, 3)
	tab.conn.Spec.ReadOnly = true
	for _, keys := range []string{"i", "o", "dd", "yyp"} {
		a.toast = ""
		if feed(t, a, keys); tab.cell != nil || tab.changes() != 0 || a.toast != "doraemon 是只读连接" {
			t.Fatalf("%s: cell %+v, %d changes, toast %q", keys, tab.cell, tab.changes(), a.toast)
		}
	}
	if a.Update(tea.PasteMsg{Content: "x"}); tab.cell != nil {
		t.Error("a paste edits")
	}
	if a.run("cell.null", 0); tab.changes() != 0 {
		t.Error("cell.null changes it")
	}
}

// A console's title shows its connection's mode, ✓ ↺ in the manual one,
// dim out of a transaction, and a read-only one's lock; the status bar
// says when Main is in a transaction, or a failed one (§7.8).
func TestGoldenConsoleTx(t *testing.T) {
	var rows []string
	for _, c := range []struct {
		manual, readOnly bool
		tx               byte
	}{{true, false, 'I'}, {true, false, 'T'}, {false, true, 'E'}} {
		a, con := inConsole(t, "", "")
		a.Update(tea.WindowSizeMsg{Width: 200, Height: 20})
		con.conn.Manual, con.conn.Spec.ReadOnly, con.conn.tx = c.manual, c.readOnly, c.tx
		f := a.render()
		lines := strings.Split(f.String(), "\n")
		r := a.layout()[a.focused().ID]
		rows = append(rows, ansi.Cut(lines[r.Min.Y], r.Min.X, r.Max.X), lines[len(lines)-1])
		if !c.manual {
			continue
		}
		commit, want := styleOf(t, f, "✓").Fg, a.theme.Dim
		if c.tx == 'T' {
			want = a.theme.Focus
		}
		if commit != want {
			t.Errorf("%c: ✓ in %v, want %v", c.tx, commit, want)
		}
	}
	golden.RequireEqual(t, strings.Join(rows, "\n")+"\n")
}

// A console running is not closed, its tab, its pane or its window: a begin
// in its run would come in after (F5.3).
func TestCloseRunning(t *testing.T) {
	a, c := inConsole(t, "", "")
	feed(t, a, "<Space>c<Space>p")
	c.running = &run{}
	for _, act := range []string{"tab.close", "pane.close", "window.close"} {
		a.toast = ""
		if a.run(act, 0); a.toast != "正在执行，结束后再关闭" || len(a.sess.consoles()) != 2 || len(a.sess.Windows) != 2 {
			t.Errorf("%s: toast %q, %d consoles, %d windows", act, a.toast, len(a.sess.consoles()), len(a.sess.Windows))
		}
	}
}
