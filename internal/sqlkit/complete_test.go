package sqlkit

import (
	"reflect"
	"slices"
	"strings"
	"testing"
)

// | is the cursor. Cases after lazysql's components/sql_context_test.go.
func TestCompletionContext(t *testing.T) {
	for _, c := range []struct {
		sql          string
		ok           bool
		kind         CompKind
		prefix, qual string
		depth        int
		tables       []TableRef
		ctes         []string
	}{
		{"select * from t_o|", true, CompTables, "t_o", "", 0, []TableRef{{Name: "t_o"}}, nil},
		{"select * from public.t_o|", true, CompColumns, "t_o", "public", 0, []TableRef{{Schema: "public", Name: "t_o"}}, nil},
		{"select o.| from t_order o", true, CompColumns, "", "o", 0, []TableRef{{Name: "t_order", Alias: "o"}}, nil},
		{"select * from t_order as o join t_user u on u.na|", true, CompColumns, "na", "u", 0, []TableRef{{Name: "t_order", Alias: "o"}, {Name: "t_user", Alias: "u"}}, nil},
		{"select * from a, b|", true, CompTables, "b", "", 0, []TableRef{{Name: "a"}, {Name: "b"}}, nil},
		{"select a, b|", true, CompAny, "b", "", 0, nil, nil},
		{"select * from t where st|", true, CompAny, "st", "", 0, []TableRef{{Name: "t"}}, nil},
		{"update t_o| set a = 1", true, CompTables, "t_o", "", 0, []TableRef{{Name: "t_o"}}, nil},
		{"insert into t (a) values (|", true, CompAny, "", "", 1, []TableRef{{Name: "t"}}, nil},
		{"with recent as (select * from t_order), b as (select 1) select * from rec|", true, CompTables, "rec", "", 0, []TableRef{{Name: "t_order", Depth: 1}, {Name: "rec"}}, []string{"recent", "b"}},
		{"select * from t1 where id in (select id from t2 where x|)", true, CompAny, "x", "", 1, []TableRef{{Name: "t1"}, {Name: "t2", Depth: 1}}, nil},
		{"select 1 from a;\nselect * from b where |", true, CompAny, "", "", 0, []TableRef{{Name: "b"}}, nil},
		{`select "Odd".| from "Odd"`, true, CompColumns, "", "Odd", 0, []TableRef{{Name: "Odd"}}, nil},
		{"select * from t_o|ur", true, CompTables, "t_o", "", 0, []TableRef{{Name: "t_our"}}, nil}, // the word's start to the cursor, as lazysql's
		{"select o.o_st|us from t o", true, CompColumns, "o_st", "o", 0, []TableRef{{Name: "t", Alias: "o"}}, nil},
		{"create table x (a timestamp with time zone, b |", true, CompAny, "", "", 1, nil, nil}, // no CTE named time
		{"with a as not materialized (select 1) select * from |", true, CompTables, "", "", 0, nil, []string{"a"}},
		{"select 'ab'|", true, CompAny, "", "", 0, nil, nil},
		{"select * from t where note = 'ab|", false, 0, "", "", 0, nil, nil}, // the string the text ends in is open
		{`select "ab|`, false, 0, "", "", 0, nil, nil},
		{"select 'ab|c'", false, 0, "", "", 0, nil, nil},
		{"select 1 -- ab|", false, 0, "", "", 0, nil, nil},
		{"select /* a| */ 1", false, 0, "", "", 0, nil, nil},
	} {
		pos := strings.Index(c.sql, "|")
		sql := c.sql[:pos] + c.sql[pos+1:]
		got := CompletionContext(sql, pos, PG)
		if !c.ok {
			if got.OK {
				t.Errorf("%s: completes", c.sql)
			}
			continue
		}
		want := Completion{OK: true, Kind: c.kind, Prefix: c.prefix, Start: pos - len(c.prefix), Qualifier: c.qual, Depth: c.depth, Tables: c.tables, CTEs: c.ctes}
		want.WantsColumn = got.WantsColumn // TestWantsColumn's
		if !reflect.DeepEqual(got, want) {
			t.Errorf("%s:\n got %+v\nwant %+v", c.sql, got, want)
		}
	}
}

// Common has DML and DDL starts, and the dialect's own clauses.
func TestCommonKeywords(t *testing.T) {
	has := func(d Dialect, k string) bool { return slices.Contains(Common(d), k) }
	for _, k := range []string{"insert into", "update", "delete from", "create table"} {
		if !has(PG, k) || !has(MySQL, k) {
			t.Errorf("%s missing", k)
		}
	}
	for _, c := range []struct {
		k       string
		in, not Dialect
	}{{"returning", PG, MySQL}, {"full join", PG, MySQL}, {"replace into", MySQL, PG}, {"ifnull", MySQL, PG}} {
		if !has(c.in, c.k) || has(c.not, c.k) {
			t.Errorf("%s: in one dialect only", c.k)
		}
	}
	if SpaceAfter("null") || SpaceAfter("count") || !SpaceAfter("and") {
		t.Error("space after")
	}
}

// UPDATE is followed by a table where it starts a statement, not as a clause
// of an upsert or a lock (F5.12 review).
func TestUpdateClause(t *testing.T) {
	for _, c := range []struct {
		sql    string
		d      Dialect
		kind   CompKind
		tables []TableRef
	}{
		{"update t_o|", PG, CompTables, []TableRef{{Name: "t_o"}}},
		{"update |", PG, CompTables, nil},
		{"insert into t (id) values (1) on conflict (id) do update |", PG, CompAny, []TableRef{{Name: "t"}}},
		{"insert into t (id) values (1) on duplicate key update st|", MySQL, CompAny, []TableRef{{Name: "t"}}},
		{"select * from t for update|", PG, CompAny, []TableRef{{Name: "t"}}},
		{"select * from t for no key update of t|", PG, CompAny, []TableRef{{Name: "t"}}},
	} {
		pos := strings.Index(c.sql, "|")
		got := CompletionContext(c.sql[:pos]+c.sql[pos+1:], pos, c.d)
		if got.Kind != c.kind || !reflect.DeepEqual(got.Tables, c.tables) {
			t.Errorf("%s: %v %+v, want %v %+v", c.sql, got.Kind, got.Tables, c.kind, c.tables)
		}
	}
}

// The columns of an INSERT's parentheses: after the ( or a comma, not once
// they are closed, not inside an expression of them, not in VALUES' own.
func TestInsertColumnsContext(t *testing.T) {
	for _, c := range []struct {
		sql, prefix string
		d           Dialect
		ok          bool
		into        TableRef
		listed      []string
	}{
		{"insert into t_log(|)", "", PG, true, TableRef{Name: "t_log"}, nil},
		{"insert into play.t_log (id, |", "", PG, true, TableRef{Schema: "play", Name: "t_log"}, []string{"id"}},
		{`insert into "T" ("a", "b", c|, d)`, "c", PG, true, TableRef{Name: "T"}, []string{"a", "b", "d"}},
		{"replace into `t` (`a`,|", "", MySQL, true, TableRef{Name: "t"}, []string{"a"}},
		{"insert into t (a, f(x), |", "", PG, true, TableRef{Name: "t"}, []string{"a", "f"}},
		{"insert into t (a, f(|", "", PG, false, TableRef{}, nil},
		{"insert into t (a) values (|", "", PG, false, TableRef{}, nil},
		{"insert into t (a) select (|", "", PG, false, TableRef{}, nil},
		{"insert into t (a) values (1);\nselect (|", "", PG, false, TableRef{}, nil},
		{"select * from t (|", "", PG, false, TableRef{}, nil},
		{"insert into comment (|", "", PG, true, TableRef{Name: "comment"}, nil}, // a table named as a keyword is
		{"insert into public.comment (a, |", "", PG, true, TableRef{Schema: "public", Name: "comment"}, []string{"a"}},
		{"insert into `key` (|", "", MySQL, true, TableRef{Name: "key"}, nil},
		{"insert into key (|", "", MySQL, true, TableRef{Name: "key"}, nil},
		{"load data local infile 'f' ignore into table t (|", "", MySQL, false, TableRef{}, nil},
	} {
		pos := strings.Index(c.sql, "|")
		got := CompletionContext(c.sql[:pos]+c.sql[pos+1:], pos, c.d)
		if ok := got.Kind == CompInsertCols; ok != c.ok || ok && (got.Into != c.into || !slices.Equal(got.Listed, c.listed) || got.Prefix != c.prefix) {
			t.Errorf("%s: %v %+v %q", c.sql, got.Kind, got.Into, got.Listed)
		}
	}
}

// A column is what comes next, with nothing typed after a blank, where
// WantsColumn says so; not after a ., a FROM or when something is typed
// (M8 F8.4).
func TestWantsColumn(t *testing.T) {
	for sql, want := range map[string]bool{
		"select |":                                 true,
		"select * from t where |":                  true,
		"select * from t where a = 1 and |":        true,
		"select * from t where a = 1 or |":         true,
		"select * from a join b on |":              true,
		"update t set |":                           true,
		"update t set a = 1, |":                    true,
		"select * from t group by |":               true,
		"select * from t order by a, |":            true,
		"select a, |":                              true,
		"select * from t having |":                 true,
		"select * from t where id in (select a, |": true,
		"select\n|":                                true,
		"select|":                                  false, // no blank yet
		"select a,|":                               false,
		"select * from t where a|":                 false, // typed
		"select * from |":                          false, // a table
		"select * from a, |":                       false,
		"select * from t where a = |":              false,
		"select * from t where id in (1, |":        false,
		"select max(a, |":                          false,
		"select o.|":                               false,
		"insert into t (|":                         false,
		"select 'a |":                              false,
	} {
		pos := strings.Index(sql, "|")
		got := CompletionContext(strings.Replace(sql, "|", "", 1), pos, PG)
		if got.WantsColumn != want {
			t.Errorf("%q: WantsColumn %v", sql, got.WantsColumn)
		}
	}
}
