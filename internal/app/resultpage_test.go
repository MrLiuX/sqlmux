package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/kackerx/sqlmux/internal/db"
)

// pagesDB answers a statement with the result its whole text, LIMIT and
// OFFSET in it, names, or fails it.
type pagesDB struct {
	execDB
	pages map[string]db.Result
	fail  error
}

func (d *pagesDB) Exec(_ context.Context, sql string, _ int) ([]db.Result, error) {
	d.ran = append(d.ran, sql)
	if d.fail != nil {
		return nil, d.fail
	}
	return []db.Result{d.pages[sql]}, nil
}

func cut(r db.Result) db.Result { r.Truncated = true; return r }

// ] and [ on a result tab of a statement given its LIMIT ask for the next page
// and the one before, OFFSET after the LIMIT, and put it in that tab only; the
// title and the log say which rows; the first page has no page before, the
// last none after, a statement with a LIMIT of its own no pages; a pinned tab
// pages too; R is the first page again (F10.4).
func TestResultPaging(t *testing.T) {
	const q = "select * from t order by id"
	d := &pagesDB{pages: map[string]db.Result{
		q + "\nLIMIT 4":           cut(rows(3)),
		q + "\nLIMIT 4 OFFSET 3":  cut(rows(3)),
		q + "\nLIMIT 4 OFFSET 6":  rows(2),
		"select 2\nLIMIT 4":       rows(1),
		"select * from u limit 5": cut(rows(3)),
	}}
	a, _ := inRun(t, &d.execDB, "")
	a.sess.Conns[0].Main = db.NewWorker(d)
	a.maxRows = 3
	c := queryOf(a.focused())
	c.ed.Load(q + ";\nselect 2;\nselect * from u limit 5;")
	press(t, a, "ggVG<CR>")
	p := a.win().Result
	a.win().focus(p.ID)
	selectTab(p, 1)
	tab := func(i int) *resultTab { return p.Tabs[i].Result }
	title := func() string { return a.resultHints(tab(p.Cur))[0].Label } // as the result area's title says it
	if len(p.Tabs) != 4 || !tab(1).pageable || !tab(2).pageable || tab(3).pageable || tab(2).stmt != 1 || tab(3).stmt != 2 {
		t.Fatalf("%d tabs, pageable %v %v %v", len(p.Tabs), tab(1).pageable, tab(2).pageable, tab(3).pageable)
	}
	ran := len(d.ran)
	press(t, a, "[") // the first page has none before
	if len(d.ran) != ran {
		t.Fatalf("[ on the first page ran %q", d.ran[ran:])
	}
	press(t, a, "]")
	if got := d.ran[len(d.ran)-1]; got != q+"\nLIMIT 4 OFFSET 3" || tab(1).offset != 3 || !strings.HasPrefix(title(), "4–6+ 行") || len(p.Tabs) != 4 {
		t.Fatalf("]: ran %q, offset %d, title %q, %d tabs", got, tab(1).offset, title(), len(p.Tabs))
	}
	if !strings.Contains(logText(a), q+"  4–6+ 行") {
		t.Errorf("the log:\n%s", logText(a))
	}
	// the export is of the page shown, its name saying which
	t.Chdir(t.TempDir())
	a.run("result.export", 0)
	press(t, a, "<CR>")
	if data, err := os.ReadFile(fmt.Sprintf("query_1-%d-p2.csv", tab(1).run.seq)); err != nil || strings.Count(string(data), "\n") != 4 {
		t.Errorf("export of page 2: %q %v, toast %q", data, err, a.toast)
	}
	if tab(2).page.Rows == nil || tab(2).offset != 0 {
		t.Error("another tab of the run is left as it was")
	}
	press(t, a, "]")
	if tab(1).offset != 6 || !strings.HasPrefix(title(), "7–8 行") || tab(1).page.Truncated {
		t.Fatalf("the last page: offset %d, title %q", tab(1).offset, title())
	}
	ran = len(d.ran)
	press(t, a, "]") // none after the last
	if len(d.ran) != ran {
		t.Errorf("] on the last page ran %q", d.ran[ran:])
	}
	press(t, a, "[")
	if tab(1).offset != 3 || !strings.HasPrefix(title(), "4–6+ 行") {
		t.Errorf("[: offset %d, title %q", tab(1).offset, title())
	}
	// a pinned tab pages as well, in place
	press(t, a, "P]")
	if !tab(1).pinned || tab(1).offset != 6 {
		t.Errorf("pinned: %v, offset %d", tab(1).pinned, tab(1).offset)
	}
	// the tab of a statement with no more rows, and the one with a LIMIT of its own: nothing to ask
	ran = len(d.ran)
	selectTab(p, 2)
	press(t, a, "]")
	selectTab(p, 3)
	press(t, a, "]")
	if len(d.ran) != ran {
		t.Errorf("no pages: ran %q", d.ran[ran:])
	}
	// a page on its way takes no second request
	selectTab(p, 1)
	_, cmd := a.Update(teaKey("["))
	if _, again := a.Update(teaKey("[")); again != nil {
		t.Error("[ while the page is on its way")
	}
	deliver(a, cmd)
	if tab(1).offset != 3 || tab(1).paging {
		t.Errorf("the page came: offset %d paging %v", tab(1).offset, tab(1).paging)
	}
	// a page that fails: logged, the log shows, the rows as they were
	d.fail = errors.New("boom")
	press(t, a, "]")
	if tab(1).offset != 3 || tab(1).paging || p.Cur != 0 || !strings.Contains(logText(a), "boom") {
		t.Errorf("failed: offset %d, on %d\n%s", tab(1).offset, p.Cur, logText(a))
	}
	d.fail = nil
	// R runs the statements again, the first page
	selectTab(p, 1)
	press(t, a, "R")
	if len(p.Tabs) != 5 || tab(p.Cur).offset != 0 { // the pinned one stays, the run's new tabs come
		t.Errorf("R: %d tabs, offset %d", len(p.Tabs), tab(p.Cur).offset)
	}
	// a page that lands after its tab is closed has nothing to take it
	selectTab(p, 1)
	_, cmd = a.Update(teaKey("]"))
	gone := tab(1)
	a.closeTab(p, p.Cur)
	deliver(a, cmd)
	if slices.ContainsFunc(p.Tabs, func(tb Tab) bool { return tb.Result == gone }) || gone.paging {
		t.Error("the page of a closed tab: the tab is back, or still waiting")
	}
}

// A statement after a SET of its run has no pages: asked alone, it would
// run with the query's schema again and read another table; one before
// the SET has (F10.4).
func TestResultNoPagesAfterSet(t *testing.T) {
	d := &pagesDB{pages: map[string]db.Result{
		"select 1\nLIMIT 4": rows(1), "select 2\nLIMIT 4": rows(1),
	}}
	a, _ := inRun(t, &d.execDB, "")
	a.sess.Conns[0].Main = db.NewWorker(d)
	a.maxRows = 3
	queryOf(a.focused()).ed.Load("select 1;\nset x to y;\nselect 2;")
	press(t, a, "ggVG<CR>")
	p := a.win().Result
	if len(p.Tabs) != 3 || !p.Tabs[1].Result.pageable || p.Tabs[2].Result.pageable {
		t.Fatalf("%d tabs: pageable %v, %v", len(p.Tabs), p.Tabs[1].Result.pageable, p.Tabs[2].Result.pageable)
	}
}

// A page whose query's schema cannot be set logs the SET as what failed,
// as a run does, not the statement that never ran (§8.6, F10.4).
func TestResultPageSetFails(t *testing.T) {
	d := &pagesDB{pages: map[string]db.Result{"select 1\nLIMIT 4": cut(rows(3)), "select 1\nLIMIT 4 OFFSET 3": rows(1)}}
	a, c := inRun(t, &d.execDB, "")
	conn := a.sess.Conns[0]
	conn.Main = db.NewWorker(d)
	a.maxRows = 3
	c.schema = "agentable"
	c.ed.Load("select 1;")
	press(t, a, "<CR>")
	conn.mainPath = "" // what the connection has is not known, as after a SET of another run
	d.execDB.fail = map[string]error{`search_path "agentable" `: errors.New("no schema")}
	a.win().focus(a.win().Result.ID)
	press(t, a, "]")
	if got := logText(a); !strings.Contains(got, "set search_path to agentable") || !strings.Contains(got, "no schema") || strings.Contains(got, "select 1  ERROR") {
		t.Errorf("the log:\n%s", got)
	}
}

// The rows a page says: all of a first page, which are on one after it, none
// when a table that shrank leaves a page with no rows (F10.4).
func TestResultTextPages(t *testing.T) {
	took := " · 0s"
	for _, c := range []struct {
		r      db.Result
		offset int
		want   string
	}{
		{cut(rows(3)), 0, "3+ 行" + took},
		{rows(3), 0, "3 行" + took},
		{cut(rows(3)), 3, "4–6+ 行" + took},
		{rows(2), 6, "7–8 行" + took},
		{rows(0), 6, "0 行" + took},
		{db.Result{Tag: "INSERT 0 1"}, 0, "INSERT 0 1" + took},
	} {
		if got := resultText(c.r, c.offset); got != c.want {
			t.Errorf("%d rows, offset %d: %q, want %q", len(c.r.Rows), c.offset, got, c.want)
		}
	}
}

// A page is sent as the run of the query is: in the manual mode in a
// transaction begun before it, the connection's status taken from where it
// comes back, the connection busy meanwhile; a cancel is logged and told, the
// tab as it was; a deadlock says so; a connection not up runs nothing; the
// transposed view stays and the arrival is of the page (F10.4).
func TestResultPageAsARunIs(t *testing.T) {
	const q = "select * from t order by id"
	d := &pagesDB{pages: map[string]db.Result{q + "\nLIMIT 4": cut(rows(3)), q + "\nLIMIT 4 OFFSET 3": rows(2)}}
	a, c := inRun(t, &d.execDB, "")
	conn := a.sess.Conns[0]
	conn.Main = db.NewWorker(d)
	a.maxRows = 3
	c.ed.Load(q + ";")
	press(t, a, "<CR>")
	p := a.win().Result
	a.win().focus(p.ID)
	rt := resultOf(p)
	rt.transpose = true
	arrived := rt.arrived

	// busy from the request to the page
	conn.Manual, conn.tx = true, 'T' // what it was; the server says it is 'I' now
	ran := len(d.ran)
	_, cmd := a.Update(teaKey("]"))
	if conn.busy != 1 || !rt.paging {
		t.Fatalf("on its way: busy %d, paging %v", conn.busy, rt.paging)
	}
	deliver(a, cmd)
	if conn.busy != 0 || rt.paging || rt.offset != 3 || conn.tx != 'I' || !rt.transpose || !rt.arrived.After(arrived) {
		t.Errorf("came: busy %d, paging %v, offset %d, tx %c, transposed %v", conn.busy, rt.paging, rt.offset, conn.tx, rt.transpose)
	}
	if got := d.ran[ran:]; len(got) != 2 || got[0] != "begin" || got[1] != q+"\nLIMIT 4 OFFSET 3" {
		t.Errorf("sent %q, a begin first in the manual mode", got)
	}
	conn.Manual = false

	// cancelled: logged, told, the rows and the tab as they were
	rt.offset, rt.page = 0, cut(rows(3))
	d.fail = context.Canceled
	press(t, a, "]")
	if got := logText(a); !strings.HasSuffix(got, "已取消") || a.toast != "查询已取消" || rt.offset != 0 || len(rt.page.Rows) != 3 || rt.paging || p.Tabs[p.Cur].Result != rt {
		t.Errorf("cancelled: log\n%s\ntoast %q offset %d paging %v", got, a.toast, rt.offset, rt.paging)
	}

	// a deadlock
	a.toast = ""
	d.fail = db.RolledBack{Err: errors.New("deadlock")}
	press(t, a, "]")
	if a.toast != "死锁，整个事务已回滚" {
		t.Errorf("deadlock: toast %q", a.toast)
	}
	d.fail = nil

	// a connection that is not up runs nothing
	selectTab(p, 1) // the deadlock showed the log
	a.toast, conn.State = "", connIdle
	ran = len(d.ran)
	press(t, a, "]")
	if a.toast != "连接还没连上" || len(d.ran) != ran || rt.paging {
		t.Errorf("not up: toast %q, sent %q, paging %v", a.toast, d.ran[ran:], rt.paging)
	}
}
