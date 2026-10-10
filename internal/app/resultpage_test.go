package app

import (
	"context"
	"errors"
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
	c := consoleOf(a.focused())
	c.ed.Load(q + ";\nselect 2;\nselect * from u limit 5;")
	press(t, a, "ggVG<CR>")
	p := a.win().Result
	a.win().focus(p.ID)
	selectTab(p, 1)
	tab := func(i int) *resultTab { return p.Tabs[i].Result }
	title := func() string { return resultText(tab(p.Cur).page, tab(p.Cur).offset) }
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
