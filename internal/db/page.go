package db

import (
	"context"
	"fmt"
	"strconv"
	"strings"
)

// Query is what a data tab asks of its table (§8.5, §9.6).
type Query struct {
	Schema, Table string
	Where         string // a condition as typed; "" for none
	Order         string // the column to sort by; "" for Key alone
	Desc          bool
	Key           []string // the row identity: the order, or Order's tiebreaker
	Limit, Offset int
}

// from is "FROM t" and the WHERE: the input as typed, in parentheses so its
// OR binds inside them, and on lines of its own so a trailing -- comment
// can't swallow what follows (§9.6).
func (q Query) from(e Engine) string {
	s := " from " + e.Quote(q.Schema, q.Table)
	if strings.TrimSpace(q.Where) != "" {
		s += " where (\n" + q.Where + "\n)"
	}
	return s
}

// Page reads one page of a table (§8.5): every column, sorted by Order and
// then the row identity so pages are stable (unordered with neither); with
// no Order the row identity sorts, Desc turning it. It
// asks for one row more than Limit to tell whether a next page exists, even
// before the count is in. One statement only: PG's extended protocol and a
// MySQL connection without multiStatements refuse more, whatever Where
// holds.
func Page(ctx context.Context, c Conn, e Engine, q Query) (r Result, next bool, err error) {
	var order []string
	if q.Order != "" {
		dir := " asc"
		if q.Desc {
			dir = " desc"
		}
		order = append(order, e.Quote(q.Order)+dir)
	}
	for _, k := range q.Key {
		switch {
		case k == q.Order:
		case q.Order == "" && q.Desc: // the row identity is what sorts: the other way
			order = append(order, e.Quote(k)+" desc")
		default:
			order = append(order, e.Quote(k))
		}
	}
	sql := "select *" + q.from(e)
	if len(order) > 0 {
		sql += " order by " + strings.Join(order, ", ")
	}
	r, err = c.Query(ctx, sql+fmt.Sprintf(" limit %d offset %d", q.Limit+1, q.Offset))
	if next = len(r.Rows) > q.Limit; next {
		r.Rows = r.Rows[:q.Limit]
	}
	return r, next, err
}

// Count counts the rows q's condition keeps (§8.5); the caller bounds it
// with its ctx.
func Count(ctx context.Context, c Conn, e Engine, q Query) (int64, error) {
	r, err := c.Query(ctx, "select count(*)"+q.from(e))
	if err != nil {
		return 0, err
	}
	return strconv.ParseInt(r.Rows[0][0].S, 10, 64)
}
