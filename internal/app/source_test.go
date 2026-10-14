package app

import (
	"context"
	"strconv"
	"strings"
	"testing"

	"github.com/kackerx/sqlmux/internal/db"
)

// fakeSource is a Source in memory: the numbers 1..n as one column id, paged by
// cursor, which says how many came before it; it cannot count and sorts by
// nothing, and takes no new row.
type fakeSource struct {
	n     int
	asked []db.PageQuery // what Page was asked
}

func (f *fakeSource) Columns(context.Context) (db.Columns, error) {
	return db.Columns{PK: []string{"id"}, Cols: []db.Column{{Name: "id", Type: "int"}}, Sortable: []string{}, NoInsert: "no new rows here"}, nil
}

func (f *fakeSource) Page(_ context.Context, q db.PageQuery) (db.Paged, error) {
	f.asked = append(f.asked, q)
	from, _ := strconv.Atoi(q.Cursor)
	p := db.Paged{Result: db.Result{Cols: []db.Col{{Name: "id"}}}}
	for i := from + 1; i <= min(from+q.Limit, f.n); i++ {
		p.Rows = append(p.Rows, []db.Val{{S: strconv.Itoa(i)}})
	}
	if p.Next = from+q.Limit < f.n; p.Next {
		p.Cursor = strconv.Itoa(from + q.Limit)
	}
	return p, nil
}

func (f *fakeSource) Count(context.Context, string) (int64, error) { return -1, nil }

func (f *fakeSource) Each(context.Context, db.PageQuery, func([]db.Col) error, func([]db.Val) error) error {
	return nil
}

func (f *fakeSource) Save(context.Context, db.Changes, bool) (int, byte, error) { return -1, 0, nil }

// A table tab over a Source that pages by cursor (F16.0): the cursor of each
// page seen is kept, ] and [ and PAGE go by them and no further than the
// page after the last seen, a count of -1 shows ?, and the source's Sortable
// and NoInsert are what ORDER and a new row ask.
func TestDataTabOverASource(t *testing.T) {
	a := wide(160, 45)
	if a.openTable(a.sess.Conns[0], db.Table{Schema: "s", Name: "keys"}) == nil {
		t.Fatal("opening a table fetches it")
	}
	tab := dataOf(a.focused())
	src := &fakeSource{n: 7}
	tab.src = src
	tab.limit = 3
	load := func() {
		t.Helper()
		cmd := a.fetch(tab, false)
		if cmd == nil {
			t.Fatal("no fetch")
		}
		a.Update(cmd())
	}
	ids := func() string {
		var got []string
		for _, r := range tab.page.Rows {
			got = append(got, r[0].S)
		}
		return strings.Join(got, ",")
	}
	load()
	if ids() != "1,2,3" || !tab.next || strings.Join(tab.cursors, "|") != "|3" {
		t.Fatalf("page 1: %s next %v cursors %q", ids(), tab.next, tab.cursors)
	}
	a.turnPage(1)
	load()
	a.turnPage(1)
	load()
	if ids() != "7" || tab.next || strings.Join(tab.cursors, "|") != "|3|6" || src.asked[2].Cursor != "6" {
		t.Fatalf("page 3: %s next %v cursors %q, asked %+v", ids(), tab.next, tab.cursors, src.asked[2])
	}
	a.turnPage(-1)
	load()
	if ids() != "4,5,6" || src.asked[3].Cursor != "3" {
		t.Errorf("back: %s, asked %+v", ids(), src.asked[3])
	}
	a.Update(teaKey("<Esc>"))
	tab.pageIn.Text = "9" // past the last page seen: the last
	tab.typing = "page"
	a.Update(teaKey("<CR>"))
	if tab.pageNo != 2 {
		t.Errorf("PAGE 9: page %d, want 3 (the last seen)", tab.pageNo+1)
	}
	tab.pageNo = 0
	load() // a first page starts the cursors anew
	if strings.Join(tab.cursors, "|") != "|3" {
		t.Errorf("a first page again: cursors %q", tab.cursors)
	}
	a.gridLine(30, false) // NG: record 30 is on a page not seen: the last that is
	if tab.pageNo != 1 {
		t.Errorf("30G: page %d, want 2 (the one after the last seen)", tab.pageNo+1)
	}
	tab.cursors = []string{"", "3", "6"}
	src.n = 5 // the source is shorter now: page 2 is the last, and the page after it is gone
	tab.pageNo = 1
	load()
	if strings.Join(tab.cursors, "|") != "|3" || tab.next {
		t.Errorf("a last page: cursors %q next %v", tab.cursors, tab.next)
	}
	src.n = 2 // one page: no cursor at all, as a source that pages by offset has
	tab.pageNo = 0
	load()
	if len(tab.cursors) != 0 || tab.reach(5) != 5 {
		t.Errorf("one page: cursors %q", tab.cursors)
	}
	// a count of -1 is not known
	a.Update(a.count(tab)())
	if tab.counted != countLost {
		t.Errorf("count -1: %v", tab.counted)
	}
	// the source says what sorts and what adds
	if cmd := a.setOrder(tab, orderOf("id")); cmd == nil || len(tab.order) != 0 || !strings.Contains(a.toast, "id") {
		t.Errorf("ORDER of a column that does not sort: order %v toast %q", tab.order, a.toast)
	}
	a.toast = ""
	a.addRow()
	if len(tab.added) != 0 || a.toast != "no new rows here" {
		t.Errorf("a new row: %d added, toast %q", len(tab.added), a.toast)
	}
}
