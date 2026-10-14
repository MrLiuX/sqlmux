package redis

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/kackerx/sqlmux/internal/db"
	"github.com/kackerx/sqlmux/internal/i18n"
)

// Layout is the reply of command cmd as a table (§11「Redis 控制台」): a scalar
// is one cell of the column value, an array a column value with a row for each
// of its elements, a map the columns field and value with the keys sorted (the
// client's map has lost the server's order); a nil is NULL; what is nested in an
// element is its JSON text. maxRows, when it is not 0, keeps that many rows and
// marks the result Truncated.
func Layout(cmd string, reply any, maxRows int) db.Result {
	r := db.Result{Cols: []db.Col{{Name: "value"}}, Tag: strings.ToUpper(cmd)}
	switch v := reply.(type) {
	case []any:
		for _, e := range v {
			r.Rows = append(r.Rows, []db.Val{cell(e)})
		}
		r.Tag += " · " + i18n.T("%d 项", len(v))
	case map[any]any:
		r.Cols = []db.Col{{Name: "field"}, {Name: "value"}}
		keys := make([]any, 0, len(v))
		for k := range v {
			keys = append(keys, k)
		}
		sort.Slice(keys, func(i, j int) bool { return cell(keys[i]).S < cell(keys[j]).S })
		for _, k := range keys {
			r.Rows = append(r.Rows, []db.Val{cell(k), cell(v[k])})
		}
		r.Tag += " · " + i18n.T("%d 个字段", len(v))
	default:
		r.Rows = [][]db.Val{{cell(reply)}}
	}
	if maxRows > 0 && len(r.Rows) > maxRows {
		r.Rows, r.Truncated = r.Rows[:maxRows], true
	}
	return r
}

// cell is a value of a reply as one cell: text as it is, a number as it is
// written, bytes that are no text as the placeholder (§8.1), a nil NULL, and
// what is nested as JSON.
func cell(v any) db.Val {
	switch v := v.(type) {
	case nil:
		return db.Val{Null: true}
	case string:
		if !utf8.ValidString(v) {
			return db.Val{S: db.BinaryShown(len(v))}
		}
		return db.Val{S: v}
	case int64:
		return db.Val{S: strconv.FormatInt(v, 10)}
	case float64:
		return db.Val{S: strconv.FormatFloat(v, 'g', -1, 64)}
	case bool:
		return db.Val{S: strconv.FormatBool(v)}
	case []any, map[any]any:
		b, _ := json.Marshal(jsonValue(v))
		return db.Val{S: string(b)}
	case error:
		return db.Val{S: v.Error()}
	}
	return db.Val{S: fmt.Sprint(v)}
}

// jsonValue is v for encoding/json: a map with the keys as text, bytes that are no text as the placeholder.
func jsonValue(v any) any {
	switch v := v.(type) {
	case []any:
		out := make([]any, len(v))
		for i, e := range v {
			out[i] = jsonValue(e)
		}
		return out
	case map[any]any:
		out := make(map[string]any, len(v))
		for k, e := range v {
			out[cell(k).S] = jsonValue(e)
		}
		return out
	case string:
		return cell(v).S
	case error:
		return v.Error()
	case float64:
		if math.IsInf(v, 0) || math.IsNaN(v) { // JSON has no inf or nan
			return cell(v).S
		}
	}
	return v
}
