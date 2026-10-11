package app

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/kackerx/sqlmux/internal/db"
)

// streamDB is a Conn that streams n rows of cols made by row, the SQL it was
// asked for kept; with gate set it stops after the first row until the gate
// opens or the request is cancelled, started closed when it is there.
type streamDB struct {
	*execDB
	cols    []db.Col
	n       int
	row     func(i int) []db.Val
	gate    chan struct{}
	started chan struct{}
	once    sync.Once
	mu      sync.Mutex
	sqls    []string
}

func (d *streamDB) Stream(ctx context.Context, sql string, head func([]db.Col) error, row func([]db.Val) error) error {
	d.mu.Lock()
	d.sqls = append(d.sqls, sql)
	d.mu.Unlock()
	if err := head(d.cols); err != nil {
		return err
	}
	for i := range d.n {
		if err := row(d.row(i)); err != nil {
			return err
		}
		if d.gate != nil && i == 0 {
			d.once.Do(func() { close(d.started) })
			select {
			case <-d.gate:
			case <-ctx.Done():
				return ctx.Err()
			}
		}
	}
	return nil
}

// ordersStream is n rows of t_order's columns, as the server sends them.
func ordersStream(n int) *streamDB {
	_, cols, _ := ordersTable(0)
	d := &streamDB{execDB: &execDB{}, n: n}
	for _, c := range cols.Cols {
		d.cols = append(d.cols, db.Col{Name: c.Name})
	}
	d.row = func(i int) []db.Val {
		note := db.Val{Null: true}
		if i%2 == 0 {
			note = db.Val{S: "a, \"b\"\nc"}
		}
		return []db.Val{{S: strconv.Itoa(i + 1)}, {S: "done"}, {S: strconv.Itoa(i) + ".5"}, {S: "t"}, {S: `{"n": ` + strconv.Itoa(i) + `}`}, note, {S: "2026-09-01 00:00:00+00"}}
	}
	return d
}

// inDir runs the rest of the test in a directory of its own, where exports
// are written.
func inDir(t *testing.T) string {
	dir := t.TempDir()
	t.Chdir(dir)
	return dir
}

// exportAt picks item at of the export dropdown of the focused pane and
// delivers the export's end.
func exportAt(t *testing.T, a *App, at int) {
	t.Helper()
	a.run("grid.export", 0)
	if a.drop == nil || a.drop.kind != dropExport {
		t.Fatal("no export dropdown")
	}
	for range at {
		a.Update(teaKey("<C-n>"))
	}
	_, cmd := a.Update(teaKey("<CR>"))
	deliver(a, cmd)
}

// The page is exported as it is shown: the columns COLS lets through in their
// order, NULL as nothing in CSV and null in JSON, a number a number, a
// boolean a boolean, json as json; the file is named after the table and one
// of that name is not touched (F12.4).
func TestExportPage(t *testing.T) {
	dir := inDir(t)
	a := wide(160, 45)
	tab := loadOrders(t, a, 4)
	tab.hidden["meta"], tab.hidden["created_at"] = true, true
	tab.colOrder = []string{"note", "id"}
	exportAt(t, a, exportCSVPage)
	want := "note,id,status,amount,paid\n" +
		"note 1,1,running,1.99,t\n" +
		"\"line one\nline two\tafter tab \x1b[31mred\",2,done,2.99,t\n" +
		",3,failed,3.99,t\n" +
		"note 4,4,pending,4.99,t\n"
	if got, err := os.ReadFile(filepath.Join(dir, "t_order.csv")); err != nil || string(got) != want || !strings.Contains(a.toast, filepath.Join(dir, "t_order.csv")) || !strings.HasSuffix(a.toast, " · 4 行") {
		t.Fatalf("csv: %q %v, toast %q", got, err, a.toast)
	}
	exportAt(t, a, exportJSONPage)
	got, err := os.ReadFile(filepath.Join(dir, "t_order.json"))
	if err != nil {
		t.Fatal(err)
	}
	var rows []map[string]any
	if err := json.Unmarshal(got, &rows); err != nil || len(rows) != 4 || rows[0]["id"] != float64(1) || rows[0]["paid"] != true || rows[2]["note"] != nil || rows[0]["amount"] != 1.99 {
		t.Fatalf("json: %s %v", got, err)
	}
	if _, has := rows[0]["meta"]; has || !strings.HasPrefix(string(got), "[\n  {\n    \"note\": \"note 1\",\n    \"id\": 1,") {
		t.Errorf("the columns: %s", got)
	}
	exportAt(t, a, exportCSVPage) // there is one: the next is t_order-2
	if _, err := os.Stat(filepath.Join(dir, "t_order-2.csv")); err != nil {
		t.Errorf("not -2: %v", err)
	}
	if first, _ := os.ReadFile(filepath.Join(dir, "t_order.csv")); string(first) != want {
		t.Error("the first file was written over")
	}
	tab.page.Rows = nil // an empty page is an empty array
	exportAt(t, a, exportJSONPage)
	if got, _ := os.ReadFile(filepath.Join(dir, "t_order-2.json")); string(got) != "[]\n" {
		t.Errorf("empty: %q", got)
	}
}

// "All" asks the statement of the page again with no LIMIT and no OFFSET, its
// WHERE and ORDER as they are, and writes the rows as they come; the connection
// is not busy after (F12.4).
func TestExportAll(t *testing.T) {
	dir := inDir(t)
	a := wide(160, 45)
	tab := loadOrders(t, a, 3)
	d := ordersStream(1203)
	tab.conn.Meta = db.NewWorker(d)
	tab.applied, tab.order, tab.limit, tab.pageNo = "id > 0", orderOf("status-"), 100, 2
	tab.shown = tab.request
	tab.hidden["meta"] = true
	exportAt(t, a, exportCSVAll)
	data, err := os.ReadFile(filepath.Join(dir, "t_order.csv"))
	if err != nil || strings.Count(string(data), "\n") != 1203+1+1203/2+1 { // the header, a row each and the lines of a note that has two
		t.Fatalf("csv: %d lines, %v", strings.Count(string(data), "\n"), err)
	}
	if want := `select * from "public"."t_order" where ( id > 0 ) order by "status" desc, "id"`; len(d.sqls) != 1 || !strings.Contains(strings.ReplaceAll(d.sqls[0], "\n", " "), "order by \"status\" desc, \"id\"") || strings.Contains(d.sqls[0], "limit") || strings.Contains(d.sqls[0], "offset") {
		t.Errorf("asked %q, want like %q", d.sqls, want)
	}
	if !strings.HasPrefix(string(data), "id,status,amount,paid,note,created_at\n1,done,0.5,t,") || !strings.HasSuffix(a.toast, " · 1203 行") || tab.conn.busy != 0 || len(a.exporting) != 0 {
		t.Errorf("head of file %q, toast %q, busy %d", strings.SplitN(string(data), "\n", 2)[0], a.toast, tab.conn.busy)
	}
	exportAt(t, a, exportJSONAll)
	var rows []map[string]any
	if data, _ := os.ReadFile(filepath.Join(dir, "t_order.json")); json.Unmarshal(data, &rows) != nil || len(rows) != 1203 || rows[1202]["id"] != float64(1203) {
		t.Errorf("json: %d rows", len(rows))
	}
}

// C-c in the middle of an export takes the file away and says so; a failure
// of the statement does the same, with its message; the same table cannot
// be exported twice at once (F12.4).
func TestExportCancelAndFail(t *testing.T) {
	dir := inDir(t)
	a := wide(160, 45)
	tab := loadOrders(t, a, 3)
	d := ordersStream(50)
	d.gate, d.started = make(chan struct{}), make(chan struct{})
	tab.conn.Meta = db.NewWorker(d)
	a.run("grid.export", 0)
	a.Update(teaKey("<C-n>")) // CSV · 全部
	_, cmd := a.Update(teaKey("<CR>"))
	done := make(chan tea.Msg)
	go func() { done <- cmd() }()
	<-d.started
	if tab.conn.busy != 1 {
		t.Errorf("busy %d during an export", tab.conn.busy)
	}
	a.run("grid.export", 0)
	a.Update(teaKey("<CR>"))
	if a.toast != "正在导出" {
		t.Errorf("a second export: toast %q", a.toast)
	}
	a.run("cancel", 0)
	a.Update(<-done)
	if _, err := os.Stat(filepath.Join(dir, "t_order.csv")); !os.IsNotExist(err) || a.toast != "导出已取消" || tab.conn.busy != 0 || len(a.exporting) != 0 {
		t.Errorf("cancelled: file %v, toast %q, busy %d", err, a.toast, tab.conn.busy)
	}

	d = ordersStream(5)
	tab.conn.Meta = db.NewWorker(&failDB{streamDB: d, after: 2})
	exportAt(t, a, exportJSONAll)
	if _, err := os.Stat(filepath.Join(dir, "t_order.json")); !os.IsNotExist(err) || a.toast != "boom" {
		t.Errorf("failed: file %v, toast %q", err, a.toast)
	}
}

// failDB fails its stream after the rows.
type failDB struct {
	*streamDB
	after int
}

func (d *failDB) Stream(ctx context.Context, sql string, head func([]db.Col) error, row func([]db.Val) error) error {
	d.n = d.after
	if err := d.streamDB.Stream(ctx, sql, head, row); err != nil {
		return err
	}
	return errors.New("boom")
}

func (d *failDB) Close() error { return nil }

// A request that runs again on a connection dialed anew gives the head again:
// the file starts over, not with its rows twice (F12.4).
func TestExportOutStartsOver(t *testing.T) {
	f, err := os.Create(filepath.Join(t.TempDir(), "x.csv"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	o := &exportOut{f: f, w: bufio.NewWriter(f), want: []exportCol{{name: "n"}}}
	cols := []db.Col{{Name: "n"}}
	for range 2 {
		if err := o.head(cols); err != nil {
			t.Fatal(err)
		}
		_ = o.row([]db.Val{{S: "1"}})
	}
	if err := o.finish(); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(f.Name()); string(data) != "n\n1\n" || o.n != 1 {
		t.Errorf("file %q, n %d", data, o.n)
	}
}

// A result's all is its statement as it was written, no LIMIT, on its
// connection's Main; one that cannot be paged gives its page (F12.4).
func TestExportResult(t *testing.T) {
	dir := inDir(t)
	d := &streamDB{execDB: &execDB{res: map[string]db.Result{"select 1": rows(2)}}, n: 5, cols: []db.Col{{Name: "n", Type: "int4"}}}
	d.row = func(i int) []db.Val { return []db.Val{{S: strconv.Itoa(i)}} }
	a, _ := inRun(t, d.execDB, "select 1")
	a.sess.Conns[0].Main = db.NewWorker(d)
	press(t, a, "<CR>")
	a.win().focus(a.win().Result.ID)
	rt := resultOf(a.focused())
	name := "query_1-" + strconv.Itoa(rt.run.seq)
	if !rt.pageable {
		t.Fatal("select 1 is not paged")
	}
	exportAt(t, a, exportCSVAll)
	if data, err := os.ReadFile(filepath.Join(dir, name+".csv")); err != nil || string(data) != "n\n0\n1\n2\n3\n4\n" || len(d.sqls) != 1 || d.sqls[0] != "select 1" {
		t.Errorf("all: %q %v, asked %q", data, err, d.sqls)
	}
	exportAt(t, a, exportJSONPage)
	if data, err := os.ReadFile(filepath.Join(dir, name+".json")); err != nil || !strings.Contains(string(data), `"n": "a"`) {
		t.Errorf("page: %q %v", data, err)
	}
	rt.pageable = false // its own LIMIT, say: all is the page
	exportAt(t, a, exportCSVAll)
	if data, err := os.ReadFile(filepath.Join(dir, name+"-2.csv")); err != nil || string(data) != "n\na\nb\n" || len(d.sqls) != 1 {
		t.Errorf("all of one not paged: %q %v", data, err)
	}
}
