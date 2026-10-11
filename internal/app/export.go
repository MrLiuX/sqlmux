package app

import (
	"bufio"
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/kackerx/sqlmux/internal/db"
	"github.com/kackerx/sqlmux/internal/i18n"
	"github.com/kackerx/sqlmux/internal/ui"
)

// What is exported (§7.6「导出」, F12.4): the options of the dropdown, in its
// order. The page is the rows in memory; all is the statement asked again, no
// LIMIT, its rows written as they come.
const (
	exportCSVPage = iota
	exportCSVAll
	exportJSONPage
	exportJSONAll
)

func exportItems() []string {
	return []string{i18n.T("CSV · 当前页"), i18n.T("CSV · 全部"), i18n.T("JSON · 当前页"), i18n.T("JSON · 全部")}
}

// exportSrc is a table or a result as an export has it: the columns written,
// the rows of its page, and how to read them all.
type exportSrc struct {
	owner any    // the tab, to keep two exports of it from running at once
	base  string // the file's name before the extension, for a page
	// the columns written: names, and the type of each when the catalog says
	// (a result's come with the rows); as rows of page are laid out, page.Cols
	cols []exportCol
	page db.Result
	// all is what runs the statement for the rows of the whole: on the worker,
	// the first function being what it does first on its connection; nil when
	// the statement cannot be asked for again, and the page is what "all" gives
	all  *exportAll
	conn *Conn
}

type exportCol struct{ name, typ string }

type exportAll struct {
	base   string // the file's name for all of it
	worker *db.Worker
	setup  func(context.Context, db.Conn) error
	sql    string
}

// exportOpen is ge, and the export buttons: the dropdown of what to export,
// for the focused table or result.
func (a *App) exportOpen() tea.Cmd {
	p := a.focused()
	d := &dropdown{kind: dropExport, pane: p}
	switch t, rt := dataOf(p), resultOf(p); {
	case t != nil && len(t.page.Cols) > 0:
		d.tab = t
	case rt != nil && rt.run != nil && rt.page.Cols != nil:
		d.result = rt
	default:
		return nil
	}
	a.drop = d
	a.dropMove(0)
	return nil
}

// exportSource is the table or the result of the dropdown as an export has it.
func (a *App) exportSource(d *dropdown) exportSrc {
	if t := d.tab; t != nil {
		src := exportSrc{owner: t, base: t.table.Name, conn: t.conn, page: db.Result{Rows: t.page.Rows}}
		for _, f := range t.shownCols() {
			name := t.page.Cols[f].Name
			src.cols = append(src.cols, exportCol{name, t.typeOf(name)})
			src.page.Cols = append(src.page.Cols, db.Col{Name: name})
		}
		src.page.Rows = nil
		for _, row := range t.page.Rows {
			out := make([]db.Val, 0, len(src.cols))
			for _, f := range t.shownCols() {
				out = append(out, row[f])
			}
			src.page.Rows = append(src.page.Rows, out)
		}
		w := t.conn.Meta
		if t.conn.Manual || t.conn.inTx() { // as fetch reads, to see what is not committed
			w = t.conn.Main
		}
		s := t.shown // what the grid shows, not what is asked and not there yet
		q := db.Query{Schema: t.table.Schema, Table: t.table.Name, Where: s.applied, Order: s.order, Desc: s.desc, Key: t.cols.Key()}
		src.all = &exportAll{base: t.table.Name, worker: w, sql: db.Select(t.conn.eng, q)}
		return src
	}
	rt := d.result
	r := rt.run
	name := fmt.Sprintf("%s-%d", r.name, r.seq)
	if rt.part > 1 {
		name += fmt.Sprintf("-%d", rt.part)
	}
	src := exportSrc{owner: rt, base: name, conn: r.conn, page: rt.page}
	if rt.offset > 0 { // the page shown is what is written, so the file says which (F10.4)
		src.base += fmt.Sprintf("-p%d", rt.offset/a.maxRows+1)
	}
	for _, c := range rt.page.Cols {
		src.cols = append(src.cols, exportCol{c.Name, c.Type})
	}
	if rt.pageable && r.from != nil {
		s := r.stmts[rt.stmt]
		schema := r.schema
		src.all = &exportAll{base: name, worker: r.conn.Main, sql: r.sql[s.Start:s.End],
			setup: func(ctx context.Context, c db.Conn) error { _, err := r.conn.mainSetup(ctx, c, schema); return err }}
	}
	return src
}

// exportDone answers an export: where it went and how many rows, or why not.
type exportDone struct {
	src      exportSrc
	path     string
	rows     int
	err      error
	streamed bool // the connection was busy with it
}

// export writes what the dropdown's item at is, a file in the current
// directory named after the table or the result (§7.6「导出」). Another export
// of the same table or result still going is refused; the rows of all come
// on the connection, which is busy till they have, C-c cancelling.
func (a *App) export(src exportSrc, at int) tea.Cmd {
	if a.exporting[src.owner] {
		return a.showToast(i18n.T("正在导出"), toastTTL)
	}
	all, asJSON := at == exportCSVAll || at == exportJSONAll, at >= exportJSONPage
	ext := ".csv"
	if asJSON {
		ext = ".json"
	}
	base := src.base
	if all && src.all != nil {
		base = src.all.base
	}
	f, path, err := createExport(base + ext)
	if err != nil {
		return a.showToast(err.Error(), toastTTL)
	}
	if a.exporting == nil {
		a.exporting = map[any]bool{}
	}
	a.exporting[src.owner] = true
	stream := all && src.all != nil
	if stream {
		src.conn.busy++
	}
	return func() tea.Msg {
		out := &exportOut{f: f, w: bufio.NewWriter(f), json: asJSON, want: src.cols}
		var err error
		if stream {
			err = src.all.worker.Run(context.Background(), func(ctx context.Context, c db.Conn) error {
				if src.all.setup != nil {
					if err := src.all.setup(ctx, c); err != nil {
						return err
					}
				}
				return db.Each(ctx, c, src.all.sql, out.head, out.row)
			})
		} else {
			err = out.head(src.page.Cols)
			for _, row := range src.page.Rows {
				if err != nil {
					break
				}
				err = out.row(row)
			}
		}
		if err == nil {
			err = out.finish()
		}
		if cerr := f.Close(); err == nil {
			err = cerr
		}
		if err != nil { // no half of a file is left
			_ = os.Remove(path)
		}
		return exportDone{src: src, path: path, rows: out.n, err: err, streamed: stream}
	}
}

// gotExport tells how an export went.
func (a *App) gotExport(m exportDone) tea.Cmd {
	delete(a.exporting, m.src.owner)
	if m.streamed {
		m.src.conn.busy--
	}
	switch {
	case errors.Is(m.err, context.Canceled):
		return a.showToast(i18n.T("导出已取消"), toastTTL)
	case m.err != nil:
		return a.showToast(m.err.Error(), toastTTL)
	}
	return a.showToast(i18n.T("已导出 %s · %d 行", m.path, m.rows), toastTTL)
}

// createExport makes the file name, or name-2, name-3 before the extension
// when there is one already, which is not touched; it is made with the
// mode the umask leaves, as a file made to be handed on.
func createExport(name string) (*os.File, string, error) {
	ext := filepath.Ext(name)
	stem := strings.TrimSuffix(name, ext)
	for i := 1; i < 100000; i++ {
		n := name
		if i > 1 {
			n = fmt.Sprintf("%s-%d%s", stem, i, ext)
		}
		path, err := filepath.Abs(n)
		if err != nil {
			return nil, "", err
		}
		f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o666)
		if errors.Is(err, fs.ErrExist) {
			continue
		}
		return f, path, err
	}
	return nil, "", fs.ErrExist
}

// exportOut writes rows as CSV or a JSON array of objects, the columns want,
// in that order, found among those of the rows by name; what a result's
// columns say of their types fills in what want leaves out.
type exportOut struct {
	f       *os.File
	w       *bufio.Writer
	csv     *csv.Writer
	json    bool
	want    []exportCol
	at      []int        // where each wanted column is in a row
	class   []ui.ColType // what each is, for JSON
	names   []string
	n       int  // rows written
	started bool // head was given
}

// head starts the file, again from its start when it is given again, as a
// request that ran a second time on a connection dialed anew does.
func (o *exportOut) head(cols []db.Col) error {
	if o.started {
		if err := o.f.Truncate(0); err != nil {
			return err
		}
		if _, err := o.f.Seek(0, 0); err != nil {
			return err
		}
		o.w.Reset(o.f)
	}
	o.started, o.n, o.at, o.class, o.names = true, 0, nil, nil, nil
	for _, w := range o.want {
		i := slices.IndexFunc(cols, func(c db.Col) bool { return c.Name == w.name })
		if i < 0 {
			return fmt.Errorf("%s: %s", i18n.T("没有这一列"), w.name)
		}
		typ := cmpOr(w.typ, cols[i].Type)
		o.at, o.class, o.names = append(o.at, i), append(o.class, colType(typ)), append(o.names, w.name)
	}
	if o.json {
		_, err := o.w.WriteString("[")
		return err
	}
	o.csv = csv.NewWriter(o.w)
	return o.csv.Write(o.names)
}

func cmpOr(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

func (o *exportOut) row(vals []db.Val) error {
	if o.json {
		fs := make([]jsonField, len(o.at))
		for i, at := range o.at {
			fs[i] = jsonField{o.names[i], o.class[i], vals[at]}
		}
		raw, err := rowRaw(fs)
		if err != nil {
			return err
		}
		var b bytes.Buffer
		if err := json.Indent(&b, raw, "  ", "  "); err != nil {
			return err
		}
		sep := ",\n  "
		if o.n == 0 {
			sep = "\n  "
		}
		if _, err := o.w.WriteString(sep); err != nil {
			return err
		}
		_, err = o.w.Write(b.Bytes())
		o.n++
		return err
	}
	rec := make([]string, len(o.at))
	for i, at := range o.at {
		rec[i] = vals[at].S // "" for a NULL
	}
	o.n++
	return o.csv.Write(rec)
}

// finish ends the file: the array closed, the buffers out.
func (o *exportOut) finish() error {
	if o.json {
		end := "\n]\n"
		if o.n == 0 {
			end = "]\n"
		}
		if _, err := o.w.WriteString(end); err != nil {
			return err
		}
	} else if o.csv != nil {
		o.csv.Flush()
		if err := o.csv.Error(); err != nil {
			return err
		}
	}
	return o.w.Flush()
}

// csvOf is r as CSV: the header, the rows, NULL as nothing, as lazysql's
// helpers/csv.go writes it; the quick SQL's copy is it (F-04).
func csvOf(r db.Result) string {
	var b strings.Builder
	w := csv.NewWriter(&b)
	rec := make([]string, len(r.Cols))
	for i, c := range r.Cols {
		rec[i] = c.Name
	}
	_ = w.Write(rec)
	for _, row := range r.Rows {
		for i, v := range row {
			rec[i] = v.S // "" when NULL
		}
		_ = w.Write(rec)
	}
	w.Flush()
	return b.String()
}
