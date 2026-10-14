package redis

import (
	"math"
	"strings"
	"testing"

	"github.com/kackerx/sqlmux/internal/db"
	"github.com/kackerx/sqlmux/internal/i18n"
)

func texts(r db.Result) (out []string) {
	for _, row := range r.Rows {
		var cells []string
		for _, v := range row {
			if v.Null {
				cells = append(cells, "NULL")
			} else {
				cells = append(cells, v.S)
			}
		}
		out = append(out, strings.Join(cells, "|"))
	}
	return out
}

// A reply is laid out as a table (§11): a scalar a cell, an array a column, a
// map two columns by key, a nil NULL, what is nested its JSON, bytes that are no
// text the placeholder, and maxRows keeps rows back (F16.1; the rest of the
// shapes are F16.4's).
func TestLayout(t *testing.T) {
	for _, c := range []struct {
		name    string
		cmd     string
		reply   any
		maxRows int
		cols    string
		want    string
		tag     string
	}{
		{"a status", "set", "OK", 0, "value", "OK", "SET"},
		{"an integer", "incr", int64(5), 0, "value", "5", "INCR"},
		{"a double", "zscore", 2.5, 0, "value", "2.5", "ZSCORE"},
		{"a nil", "get", nil, 0, "value", "NULL", "GET"},
		{"a bool", "x", true, 0, "value", "true", "X"},
		{"bytes", "get", "\xff\xfe", 0, "value", "<binary 2 bytes>", "GET"},
		{"an array", "lrange", []any{"a", int64(2), nil}, 0, "value", "a,2,NULL", "LRANGE · " + i18n.T("%d 项", 3)},
		{"an empty array", "lrange", []any{}, 0, "value", "", "LRANGE · " + i18n.T("%d 项", 0)},
		{"a map by key", "hgetall", map[any]any{"b": "2", "a": "1"}, 0, "field,value", "a|1,b|2", "HGETALL · " + i18n.T("%d 个字段", 2)},
		{"nested", "x", []any{[]any{"a", int64(1)}, map[any]any{"k": []any{"v"}}}, 0, "value", `["a",1],{"k":["v"]}`, "X · " + i18n.T("%d 项", 2)},
		{"inf in a nested", "x", []any{[]any{inf()}}, 0, "value", `["+Inf"]`, "X · " + i18n.T("%d 项", 1)},
		{"cut", "lrange", []any{"1", "2", "3", "4"}, 2, "value", "1,2", "LRANGE · " + i18n.T("%d 项", 4)},
	} {
		r := Layout(c.cmd, c.reply, c.maxRows)
		var cols []string
		for _, col := range r.Cols {
			cols = append(cols, col.Name)
		}
		if got := strings.Join(texts(r), ","); got != c.want || strings.Join(cols, ",") != c.cols || r.Tag != c.tag || r.Truncated != (c.maxRows > 0 && len(c.reply.([]any)) > c.maxRows) {
			t.Errorf("%s: %q cols %q tag %q truncated %v", c.name, got, cols, r.Tag, r.Truncated)
		}
	}
}

func inf() float64 { return math.Inf(1) }
