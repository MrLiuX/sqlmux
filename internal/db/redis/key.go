package redis

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	goredis "github.com/redis/go-redis/v9"

	"github.com/kackerx/sqlmux/internal/db"
	"github.com/kackerx/sqlmux/internal/i18n"
)

// Key is one Redis key as the rows of a table tab (§5「接口缝」, §8.7「键 tab」):
// what its type makes of it, read on Meta, written on Main. A hash, set or zset
// no longer than LoadAll is read whole and paged, filtered and sorted here;
// a longer one by HSCAN, SSCAN or ZRANGE, which has a next page alone, or a
// page by offset. Its db is the one its Workers are on.
type Key struct {
	Meta, Main *db.Worker
	Name       string
	LoadAll    int // the length a hash, set or zset is read whole up to ([redis] load_all)
	ScanKeys   int // the entries an export of one that is longer reads, as a db's scan reads keys
	ReadOnly   bool
}

// ErrNoKey is a key that is not there, or has another type than it had.
var ErrNoKey = msgError(i18n.N("键不存在"))

// ErrBinary is a change of a value that is bytes the grid shows no more of than
// their size (§8.1).
var ErrBinary = msgError(i18n.N("二进制值不能写回"))

// msgError is an error that says itself in the language in use, when it is asked
// (§14「界面语言」).
type msgError string

func (e msgError) Error() string { return i18n.T(string(e)) }

// typeOf is the type of the key, as TYPE says it.
func (k Key) typeOf(ctx context.Context, w *db.Worker) (typ string, err error) {
	err = w.RunSafe(ctx, func(ctx context.Context, c db.Conn) error {
		return Use(ctx, c, func(cl *goredis.Client) (err error) { typ, err = cl.Type(ctx, k.Name).Result(); return err })
	})
	if typ == "none" && err == nil {
		err = ErrNoKey
	}
	return typ, err
}

// length is how many entries the key holds, as its type counts them.
func length(ctx context.Context, cl *goredis.Client, typ, key string) (int64, error) {
	switch typ {
	case "string":
		return cl.StrLen(ctx, key).Result()
	case "hash":
		return cl.HLen(ctx, key).Result()
	case "list":
		return cl.LLen(ctx, key).Result()
	case "set":
		return cl.SCard(ctx, key).Result()
	case "zset":
		return cl.ZCard(ctx, key).Result()
	case "stream":
		return cl.XLen(ctx, key).Result()
	}
	return 1, nil // a JSON value is one
}

// Columns are the columns of the key's type, read with its length: whether it is
// read whole decides what sorts (§8.7's table).
func (k Key) Columns(ctx context.Context) (cols db.Columns, err error) {
	typ, err := k.typeOf(ctx, k.Meta)
	if err != nil {
		return cols, err
	}
	var n int64
	err = k.Meta.RunSafe(ctx, func(ctx context.Context, c db.Conn) error {
		return Use(ctx, c, func(cl *goredis.Client) (err error) { n, err = length(ctx, cl, typ, k.Name); return err })
	})
	if err != nil {
		return cols, err
	}
	str := func(name string, key bool) db.Column { return db.Column{Name: name, Type: "text", NotNull: key} }
	switch typ {
	case "string":
		cols = db.Columns{Cols: []db.Column{{Name: "value", Type: "text", NotNull: true}}, PK: []string{}, Sortable: []string{}, NoInsert: i18n.T("string 只有一个值")}
	case "ReJSON-RL":
		cols = db.Columns{Cols: []db.Column{{Name: "value", Type: "json", NotNull: true}}, PK: []string{}, Sortable: []string{}, NoInsert: i18n.T("JSON 只有一个值")}
	case "hash":
		cols = db.Columns{Cols: []db.Column{str("field", true), {Name: "value", Type: "text", NotNull: true}}, PK: []string{"field"}}
	case "list":
		cols = db.Columns{Cols: []db.Column{{Name: "index", Type: "bigint", NotNull: true, Generated: true, ReadOnly: i18n.T("index 由位置决定")}, {Name: "value", Type: "text", NotNull: true}}, PK: []string{"index"}, Sortable: []string{}}
	case "set":
		cols = db.Columns{Cols: []db.Column{str("member", true)}, PK: []string{"member"}}
	case "zset":
		cols = db.Columns{Cols: []db.Column{str("member", true), {Name: "score", Type: "double precision", NotNull: true}}, PK: []string{"member"}}
	case "stream":
		cols = db.Columns{Cols: []db.Column{str("id", true)}, PK: []string{"id"}, Sortable: []string{}, NoInsert: i18n.T("stream 的条目用 XADD 加，这里只能删除"), NoUpdate: i18n.T("stream 的条目不能改，只能删除")}
	default:
		return cols, fmt.Errorf("%s", i18n.T("不认识的类型 %s", typ))
	}
	if !k.whole(typ, n) { // read by scan: no local sorting, a zset has its score's two ways
		cols.Sortable = []string{}
		if typ == "zset" {
			cols.Sortable = []string{"score"}
		}
	}
	return cols, nil
}

// whole is whether a hash, set or zset of n entries is read whole.
func (k Key) whole(typ string, n int64) bool {
	return (typ == "hash" || typ == "set" || typ == "zset") && n <= int64(k.LoadAll)
}

// Page reads one page of the key: of what the type reads whole, the page of the
// rows that match the filter, a glob on the field or the member, sorted by the
// order; of one read by scan, the next of its cursor; a list by offset, a zset
// beyond LoadAll by offset in the score's order or the reverse, a stream by the
// ID after the last of the page before (§8.7).
func (k Key) Page(ctx context.Context, q db.PageQuery) (p db.Paged, err error) {
	typ, err := k.typeOf(ctx, k.Meta)
	if err != nil {
		return p, err
	}
	err = k.Meta.RunSafe(ctx, func(ctx context.Context, c db.Conn) error {
		return Use(ctx, c, func(cl *goredis.Client) (err error) {
			if p, err = k.read(ctx, cl, typ, q); err != nil {
				return err
			}
			p.Info, err = k.info(ctx, cl, typ)
			return err
		})
	})
	return p, err
}

// read is one page of the key of type typ; a Limit under 1 is all of what is read
// whole.
func (k Key) read(ctx context.Context, cl *goredis.Client, typ string, q db.PageQuery) (p db.Paged, err error) {
	key, limit := k.Name, q.Limit
	rows := func(cols ...string) {
		for _, c := range cols {
			p.Cols = append(p.Cols, db.Col{Name: c})
		}
	}
	switch typ {
	case "string":
		rows("value")
		v, err := cl.Get(ctx, key).Result()
		if err != nil {
			return p, keyErr(err)
		}
		p.Rows = [][]db.Val{{text(v)}}
	case "ReJSON-RL":
		rows("value")
		v, err := cl.Do(ctx, "JSON.GET", key, ".").Text()
		if err != nil {
			return p, keyErr(err)
		}
		p.Rows = [][]db.Val{{text(v)}}
	case "hash":
		rows("field", "value")
		n, err := cl.HLen(ctx, key).Result()
		if err != nil {
			return p, err
		}
		if !k.whole(typ, n) {
			return k.scan(p, q, func(cursor uint64, match string, count int64) ([]string, uint64, error) {
				return cl.HScan(ctx, key, cursor, match, count).Result()
			}, 2)
		}
		m, err := cl.HGetAll(ctx, key).Result()
		if err != nil {
			return p, err
		}
		for f, v := range m {
			p.Rows = append(p.Rows, []db.Val{text(f), text(v)})
		}
		p = k.local(p, q, true)
	case "set":
		rows("member")
		n, err := cl.SCard(ctx, key).Result()
		if err != nil {
			return p, err
		}
		if !k.whole(typ, n) {
			return k.scan(p, q, func(cursor uint64, match string, count int64) ([]string, uint64, error) {
				return cl.SScan(ctx, key, cursor, match, count).Result()
			}, 1)
		}
		ms, err := cl.SMembers(ctx, key).Result()
		if err != nil {
			return p, err
		}
		for _, m := range ms {
			p.Rows = append(p.Rows, []db.Val{text(m)})
		}
		p = k.local(p, q, true)
	case "zset":
		rows("member", "score")
		n, err := cl.ZCard(ctx, key).Result()
		if err != nil {
			return p, err
		}
		args := goredis.ZRangeArgs{Key: key, Start: 0, Stop: -1}
		whole := k.whole(typ, n)
		if !whole { // by offset, in the score's order or the reverse
			args.Start, args.Stop = q.Offset, q.Offset+limit // one more than a page, to know there is a next
			args.Rev = len(q.Order) > 0 && q.Order[0].Desc
		}
		zs, err := cl.ZRangeArgsWithScores(ctx, args).Result()
		if err != nil {
			return p, err
		}
		for _, z := range zs {
			p.Rows = append(p.Rows, []db.Val{text(z.Member.(string)), {S: strconv.FormatFloat(z.Score, 'g', -1, 64)}})
		}
		if whole {
			p = k.local(p, q, false)
		} else if len(p.Rows) > limit {
			p.Rows, p.Next = p.Rows[:limit], true
		}
	case "list":
		rows("index", "value")
		stop := int64(-1)
		if limit > 0 {
			stop = int64(q.Offset + limit) // one more than a page
		}
		vs, err := cl.LRange(ctx, key, int64(q.Offset), stop).Result()
		if err != nil {
			return p, err
		}
		for i, v := range vs {
			p.Rows = append(p.Rows, []db.Val{{S: strconv.Itoa(q.Offset + i)}, text(v)})
		}
		if limit > 0 && len(p.Rows) > limit {
			p.Rows, p.Next = p.Rows[:limit], true
		}
	case "stream":
		return k.stream(ctx, cl, q)
	}
	return p, nil
}

// keyErr is an error of a read of the key: one that says it is not there or is
// another type, as the key gone or changed.
func keyErr(err error) error {
	var re goredis.Error
	switch {
	case errors.Is(err, goredis.Nil):
		return ErrNoKey
	case errors.As(err, &re) && strings.HasPrefix(re.Error(), "WRONGTYPE"):
		return ErrNoKey
	}
	return err
}

// text is a value of the key as a cell: bytes that are no text are their size.
func text(s string) db.Val {
	if !utf8.ValidString(s) {
		return db.Val{S: db.BinaryShown(len(s))}
	}
	return db.Val{S: s}
}

// local filters the rows of a key read whole by the glob of the filter on the
// first column, sorts them as the order says, and cuts the page out (§8.7). With
// no order the first column sorts, as text, unless byName is false: a zset
// stays in the server's order, its score's.
func (k Key) local(p db.Paged, q db.PageQuery, byName bool) db.Paged {
	if f := strings.TrimSpace(q.Filter); f != "" {
		match := Matcher(f)
		p.Rows = slices.DeleteFunc(p.Rows, func(r []db.Val) bool { return !match(r[0].S) })
	}
	order := q.Order
	if len(order) == 0 && byName {
		order = []db.OrderCol{{Col: p.Cols[0].Name}}
	}
	cols := map[string]int{}
	for i, c := range p.Cols {
		cols[c.Name] = i
	}
	less := func(a, b []db.Val) int {
		for _, o := range order {
			i, ok := cols[o.Col]
			if !ok {
				continue
			}
			c := strings.Compare(a[i].S, b[i].S)
			if p.Cols[i].Name == "score" {
				x, _ := strconv.ParseFloat(a[i].S, 64)
				y, _ := strconv.ParseFloat(b[i].S, 64)
				c = cmpFloat(x, y)
			}
			if o.Desc {
				c = -c
			}
			if c != 0 {
				return c
			}
		}
		return strings.Compare(a[0].S, b[0].S) // the member breaks ties
	}
	if len(order) > 0 {
		slices.SortStableFunc(p.Rows, less)
	}
	if q.Limit > 0 {
		from := min(q.Offset, len(p.Rows))
		to := min(from+q.Limit, len(p.Rows))
		p.Next = to < len(p.Rows)
		p.Rows = p.Rows[from:to]
	}
	return p
}

func cmpFloat(a, b float64) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}

// scan is a page of a hash or set read by HSCAN or SSCAN from the cursor of the
// query, reading on till it has a page of entries or the cursor comes round. A
// batch gives what it gives, a listpack all of the key whatever COUNT says, so the
// cursor of the next page may be in the middle of a batch: "cursor:skip", the
// batch read again from cursor, its first skip entries left out. width is how
// many of the strings it gives make a row.
func (k Key) scan(p db.Paged, q db.PageQuery, step func(cursor uint64, match string, count int64) ([]string, uint64, error), width int) (db.Paged, error) {
	var cursor uint64
	var skip int
	if q.Cursor != "" {
		c, sk, _ := strings.Cut(q.Cursor, ":")
		var err error
		if cursor, err = strconv.ParseUint(c, 10, 64); err == nil && sk != "" {
			skip, err = strconv.Atoi(sk)
		}
		if err != nil {
			return p, err
		}
	}
	entries, cursor, skip, err := scanLoop(cursor, skip, q.Filter, q.Limit, width, step)
	for _, e := range entries {
		row := make([]db.Val, width)
		for j := range width {
			row[j] = text(e[j])
		}
		p.Rows = append(p.Rows, row)
	}
	if err != nil {
		return p, err
	}
	if cursor != 0 || skip != 0 {
		p.Next, p.Cursor = true, strconv.FormatUint(cursor, 10)
		if skip != 0 {
			p.Cursor += ":" + strconv.Itoa(skip)
		}
	}
	return p, nil
}

// scanLoop reads the batches step gives from cursor on, the first without its first
// skip entries, till limit entries of width strings are in (0: till the cursor is
// round), and says where it stopped: the cursor, 0 at the end, and how many entries
// of the batch it is read from were taken already, when the limit cut it. An error
// is the batches' own, those read before it kept.
func scanLoop(cursor uint64, skip int, match string, limit, width int, step func(cursor uint64, match string, count int64) ([]string, uint64, error)) ([][]string, uint64, int, error) {
	var entries [][]string
	count := int64(max(limit, 10))
	for {
		batch, next, err := step(cursor, match, count)
		if err != nil {
			return entries, cursor, skip, err
		}
		var rows [][]string
		for i := 0; i+width <= len(batch); i += width {
			rows = append(rows, batch[i:i+width])
		}
		rows = rows[min(skip, len(rows)):]
		if limit > 0 && len(entries)+len(rows) > limit {
			take := limit - len(entries)
			return append(entries, rows[:take]...), cursor, skip + take, nil
		}
		entries = append(entries, rows...)
		skip = 0
		if cursor = next; cursor == 0 || limit > 0 && len(entries) >= limit {
			return entries, cursor, 0, nil
		}
	}
}

// stream is a page of entries by ID: those after the cursor, the ID of the last
// of the page before, from the first when there is none. Its columns are id and
// the fields of the entries of the page, all of them, an entry that lacks one
// with NULL there.
func (k Key) stream(ctx context.Context, cl *goredis.Client, q db.PageQuery) (p db.Paged, err error) {
	start := "-"
	if q.Cursor != "" {
		start = nextID(q.Cursor)
	}
	limit := int64(q.Limit)
	if limit < 1 {
		limit = 1000
	}
	ms, err := cl.XRangeN(ctx, k.Name, start, "+", limit+1).Result()
	if err != nil {
		return p, err
	}
	if len(ms) > int(limit) {
		ms, p.Next = ms[:limit], true
		p.Cursor = ms[len(ms)-1].ID
	}
	var fields []string
	for _, m := range ms {
		for f := range m.Values {
			if !slices.Contains(fields, f) {
				fields = append(fields, f)
			}
		}
	}
	slices.Sort(fields)
	p.Cols = []db.Col{{Name: "id"}}
	for _, f := range fields {
		p.Cols = append(p.Cols, db.Col{Name: f})
	}
	for _, m := range ms {
		row := []db.Val{{S: m.ID}}
		for _, f := range fields {
			if v, ok := m.Values[f]; ok {
				row = append(row, text(fmt.Sprint(v)))
			} else {
				row = append(row, db.Val{Null: true})
			}
		}
		p.Rows = append(p.Rows, row)
	}
	return p, nil
}

// nextID is the ID that follows id, the one an exclusive range starts at, which
// a server before 6.2 has no ( for.
func nextID(id string) string {
	ms, seq, ok := strings.Cut(id, "-")
	n, err := strconv.ParseUint(seq, 10, 64)
	if !ok || err != nil {
		return id
	}
	return ms + "-" + strconv.FormatUint(n+1, 10)
}

// info is the line under the title of the key's tab (§8.7): its type, how long it
// has to live, its encoding, how much memory it takes, and how many entries it has,
// the ones the tab shows first first.
func (k Key) info(ctx context.Context, cl *goredis.Client, typ string) ([]db.Attr, error) {
	ttl, err := cl.PTTL(ctx, k.Name).Result()
	if err != nil {
		return nil, err
	}
	enc, _ := cl.ObjectEncoding(ctx, k.Name).Result() // a JSON value has none
	mem, merr := cl.MemoryUsage(ctx, k.Name).Result()
	n, err := length(ctx, cl, typ, k.Name)
	if err != nil {
		return nil, err
	}
	attrs := []db.Attr{{Name: "type", Value: typeName(typ)}}
	if typ != "ReJSON-RL" {
		attrs = append(attrs, db.Attr{Name: "length", Value: strconv.FormatInt(n, 10)})
	}
	attrs = append(attrs, db.Attr{Name: "ttl", Value: HumanTTL(ttl)})
	if enc != "" {
		attrs = append(attrs, db.Attr{Name: "encoding", Value: enc})
	}
	if merr == nil {
		attrs = append(attrs, db.Attr{Name: "memory", Value: humanBytes(mem)})
	}
	return attrs, nil
}

// typeName is TYPE's word as the tab says it: json for RedisJSON's.
func typeName(typ string) string {
	if typ == "ReJSON-RL" {
		return "json"
	}
	return typ
}

// HumanTTL is a time to live as a person reads it: 12m03s, 1h01m40s, 3d02h03m;
// none for a key that does not expire, and for one that is gone.
func HumanTTL(d time.Duration) string {
	switch {
	case d == -1:
		return i18n.T("不过期")
	case d < 0:
		return "-"
	case d < time.Second:
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	s := int64(d / time.Second)
	days, h, m, sec := s/86400, s/3600%24, s/60%60, s%60
	switch {
	case days > 0:
		return fmt.Sprintf("%dd%02dh%02dm", days, h, m)
	case h > 0:
		return fmt.Sprintf("%dh%02dm%02ds", h, m, sec)
	case m > 0:
		return fmt.Sprintf("%dm%02ds", m, sec)
	}
	return fmt.Sprintf("%ds", sec)
}

// humanBytes is a size in bytes as B, KB, MB, GB.
func humanBytes(n int64) string {
	f := float64(n)
	for _, u := range []string{"B", "KB", "MB"} {
		if f < 1024 {
			if u == "B" {
				return fmt.Sprintf("%d%s", n, u)
			}
			return strings.TrimSuffix(fmt.Sprintf("%.1f", f), ".0") + u
		}
		f /= 1024
	}
	return strings.TrimSuffix(fmt.Sprintf("%.1f", f), ".0") + "GB"
}

// Count is how many entries the filter keeps: what the type counts when there is
// none, those that match when the key is read whole, -1 of one read by scan with
// a filter, which cannot say without reading it all (§8.7).
func (k Key) Count(ctx context.Context, filter string) (n int64, err error) {
	typ, err := k.typeOf(ctx, k.Meta)
	if err != nil {
		return 0, err
	}
	err = k.Meta.RunSafe(ctx, func(ctx context.Context, c db.Conn) error {
		return Use(ctx, c, func(cl *goredis.Client) error {
			if n, err = length(ctx, cl, typ, k.Name); err != nil {
				return err
			}
			if typ == "string" || typ == "ReJSON-RL" {
				n = 1
			}
			if strings.TrimSpace(filter) == "" || typ != "hash" && typ != "set" && typ != "zset" {
				return nil
			}
			if !k.whole(typ, n) {
				n = -1
				return nil
			}
			p, err := k.read(ctx, cl, typ, db.PageQuery{Filter: filter})
			n = int64(len(p.Rows))
			return err
		})
	})
	return n, err
}

// Each gives the entries one at a time for an export: a key read whole all of what
// the filter keeps in the order, the others, the longer hash, set or zset, the
// list and the stream page after page; q.Limit asks for that page alone. Past
// ScanKeys entries it stops and says ErrTruncated (§8.7).
func (k Key) Each(ctx context.Context, q db.PageQuery, head func([]db.Col) error, row func([]db.Val) error) error {
	typ, err := k.typeOf(ctx, k.Meta)
	if err != nil {
		return err
	}
	return k.Meta.Run(ctx, func(ctx context.Context, c db.Conn) error {
		return Use(ctx, c, func(cl *goredis.Client) error {
			var n int64
			if n, err = length(ctx, cl, typ, k.Name); err != nil {
				return err
			}
			sent, started := 0, false
			for {
				pq := q
				if q.Limit < 1 && k.whole(typ, n) {
					pq.Limit = 0 // all of it
				} else if q.Limit < 1 {
					pq.Limit = 1000
				}
				p, err := k.read(ctx, cl, typ, pq)
				if err != nil {
					return err
				}
				if !started {
					if err := head(p.Cols); err != nil {
						return err
					}
					started = true
				}
				for _, r := range p.Rows {
					if err := row(r); err != nil {
						return err
					}
					sent++
				}
				switch {
				case q.Limit > 0 || !p.Next:
					return nil
				case sent >= k.ScanKeys:
					return db.ErrTruncated
				}
				q.Offset += len(p.Rows)
				q.Cursor = p.Cursor
			}
		})
	})
}

// Rebuild is the commands that make the key again, as the DDL of a table is the
// statement that makes it (§8.7): its entries, a key longer than LoadAll cut to
// that many and said so, then its time to live.
func (k Key) Rebuild(ctx context.Context) (string, error) {
	typ, err := k.typeOf(ctx, k.Meta)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	line := func(words ...string) {
		for i, w := range words {
			if i > 0 {
				b.WriteByte(' ')
			}
			b.WriteString(Quote(w))
		}
		b.WriteByte('\n')
	}
	err = k.Meta.RunSafe(ctx, func(ctx context.Context, c db.Conn) error {
		return Use(ctx, c, func(cl *goredis.Client) error {
			n, err := length(ctx, cl, typ, k.Name)
			if err != nil {
				return err
			}
			if typ != "string" && typ != "ReJSON-RL" && n > int64(k.LoadAll) {
				b.WriteString(fmt.Sprintf("# %s\n", i18n.T("只列出前 %d 项，一共 %d 项", k.LoadAll, n)))
			}
			upto := int64(k.LoadAll - 1)
			switch typ {
			case "string":
				v, err := cl.Get(ctx, k.Name).Result()
				if err != nil {
					return keyErr(err)
				}
				line("SET", k.Name, v)
			case "ReJSON-RL":
				v, err := cl.Do(ctx, "JSON.GET", k.Name, ".").Text()
				if err != nil {
					return keyErr(err)
				}
				line("JSON.SET", k.Name, "$", v)
			case "hash":
				entries, _, _, err := scanLoop(0, 0, "", k.LoadAll, 2, func(cursor uint64, match string, count int64) ([]string, uint64, error) {
					return cl.HScan(ctx, k.Name, cursor, match, count).Result()
				})
				if err != nil {
					return err
				}
				words := []string{"HSET", k.Name}
				// ponytail: the first LoadAll entries HSCAN gives, in the order of their names; a
				// hash longer than that is not sorted whole, which would read it whole
				slices.SortFunc(entries, func(a, b []string) int { return strings.Compare(a[0], b[0]) })
				for _, e := range entries {
					words = append(words, e...)
				}
				line(words...)
			case "list":
				vs, err := cl.LRange(ctx, k.Name, 0, upto).Result()
				if err != nil {
					return err
				}
				line(append([]string{"RPUSH", k.Name}, vs...)...)
			case "set":
				entries, _, _, err := scanLoop(0, 0, "", k.LoadAll, 1, func(cursor uint64, match string, count int64) ([]string, uint64, error) {
					return cl.SScan(ctx, k.Name, cursor, match, count).Result()
				})
				if err != nil {
					return err
				}
				words := []string{"SADD", k.Name} // sorted as the hash's is, of what it read
				slices.SortFunc(entries, func(a, b []string) int { return strings.Compare(a[0], b[0]) })
				for _, e := range entries {
					words = append(words, e[0])
				}
				line(words...)
			case "zset":
				zs, err := cl.ZRangeWithScores(ctx, k.Name, 0, upto).Result()
				if err != nil {
					return err
				}
				words := []string{"ZADD", k.Name}
				for _, z := range zs {
					words = append(words, strconv.FormatFloat(z.Score, 'g', -1, 64), z.Member.(string))
				}
				line(words...)
			case "stream":
				ms, err := cl.XRangeN(ctx, k.Name, "-", "+", int64(k.LoadAll)).Result()
				if err != nil {
					return err
				}
				for _, m := range ms {
					words := []string{"XADD", k.Name, m.ID}
					for _, f := range slices.Sorted(maps.Keys(m.Values)) {
						words = append(words, f, fmt.Sprint(m.Values[f]))
					}
					line(words...)
				}
			}
			if ttl, err := cl.PTTL(ctx, k.Name).Result(); err == nil && ttl > 0 {
				line("PEXPIRE", k.Name, strconv.FormatInt(ttl.Milliseconds(), 10))
			}
			return nil
		})
	})
	return strings.TrimSuffix(b.String(), "\n"), err
}

// Quote is s as redis-cli reads a word: bare when it has nothing in it that a
// line splits or quotes, else in double quotes with the escapes of SplitLine.
func Quote(s string) string {
	if s != "" && !strings.ContainsFunc(s, func(r rune) bool {
		return r <= ' ' || r == '"' || r == '\'' || r == '\\' || r == 0x7f || r == utf8.RuneError
	}) && !strings.HasPrefix(s, "#") {
		return s
	}
	var b strings.Builder
	b.WriteByte('"')
	valid := utf8.ValidString(s) // once: it reads all of s
	for _, c := range []byte(s) {
		switch {
		case c == '"' || c == '\\':
			b.WriteByte('\\')
			b.WriteByte(c)
		case c == '\n':
			b.WriteString(`\n`)
		case c == '\r':
			b.WriteString(`\r`)
		case c == '\t':
			b.WriteString(`\t`)
		case c < 0x20 || c == 0x7f || c >= 0x80 && !valid:
			fmt.Fprintf(&b, `\x%02x`, c)
		default:
			b.WriteByte(c)
		}
	}
	b.WriteByte('"')
	return b.String()
}
