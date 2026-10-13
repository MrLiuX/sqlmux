package sqlkit

import (
	"reflect"
	"slices"
	"strings"
	"testing"
)

// flat is what a Value says, one call or place a line, the call around it after.
func flat(v Value) []string {
	var out []string
	for v.OK {
		s := map[ValueKind]string{ValueInsert: "insert", ValueUpdate: "update", ValueWhere: "where", ValueCall: "call"}[v.Kind]
		if v.Kind == ValueCall {
			s += " " + v.Func.Schema + "." + v.Func.Name
		} else {
			s += " " + strings.Join(v.Cols, ",")
		}
		s += " #" + string(rune('0'+v.Index))
		if v.Qualifier != "" {
			s += " of " + v.Qualifier
		}
		out = append(out, s)
		if v.Outer == nil {
			break
		}
		v = *v.Outer
	}
	return out
}

// Which value goes where: the INSERT's by the commas of its row, each row
// counted again; the SET's column; a condition's; a call's argument.
func TestValueContext(t *testing.T) {
	for _, c := range []struct {
		sql  string
		d    Dialect
		want []string
	}{
		{"insert into t (a, b, c) values (1, |", PG, []string{"insert a,b,c #1"}},
		{"insert into t (a, b, c) values (|", PG, []string{"insert a,b,c #0"}},
		{"insert into t (a, b, c) values (1, 2, 3|", PG, []string{"insert a,b,c #2"}},
		{"insert into t (a, b) values (1, 2),\n  (3, |", PG, []string{"insert a,b #1"}}, // each parenthesis counts again
		{"insert into t (a, b) values (1, 2),\n  (|", PG, []string{"insert a,b #0"}},
		{"insert into t values (1, |", PG, []string{"insert  #1"}}, // no list: the catalog's order
		{`insert into public."T" ("a b", c) values ('x', |`, PG, []string{"insert a b,c #1"}},
		{"insert into t (a, b) values ('x, y', |", PG, []string{"insert a,b #1"}}, // a comma in a string is no comma
		{"insert into t (a, b) values ('x, |", PG, []string{"insert a,b #0"}},     // the string being typed is a place too
		{"insert into t (a, b) values (1, /* 2, 3, */ |", PG, []string{"insert a,b #1"}},
		{"insert into t (a, b) values (1, -- 2, 3,\n |", PG, []string{"insert a,b #1"}},
		{"insert into t (a, b) values (1, /* |", PG, nil}, // in a comment
		{"insert into t (a, b) values (1, f(2, 3), |", PG, []string{"insert a,b #2"}},
		{"insert into t (a, b) values (1, f(2, |", PG, []string{"call .f #1", "insert a,b #1"}},              // the call first, what it is an argument of after
		{"insert into t (a, b) values (1, g(f(|", PG, []string{"call .f #0", "call .g #0", "insert a,b #1"}}, // the innermost first
		{"insert ignore into t (a) values (|", MySQL, []string{"insert a #0"}},
		{"replace into t (a, b) value (1, |", MySQL, []string{"insert a,b #1"}},
		{"insert into t as x (a, b) values (1, |", PG, []string{"insert a,b #1"}},
		{"insert into t (a, b) values (1, 2) |", PG, nil},  // the row is closed
		{"insert into t (a, b) values (1, 2), |", PG, nil}, // and the next is not opened
		{"insert into t (a, b) select 1, |", PG, nil},      // INSERT … SELECT has no values
		{"insert into t (a, b) select * from (values (1, |", PG, nil},
		{"insert into t (|", PG, nil},
		{"insert into t (a, |) values (1)", PG, nil}, // the columns' parentheses, though the values are written
		{"insert into t (a) select 1 on conflict (a) do update set a = |", PG, nil},
		{"insert into t (a, |", PG, nil},
		{"insert into t (a) values (1);\nselect (1, |", PG, nil}, // the statement is the one at the cursor
		{"select 1;\ninsert into t (a, b) values (1, |", PG, []string{"insert a,b #1"}},
		{"insert into t (a, b) values (1, 2);\ninsert into t (c, d) values (3, |", PG, []string{"insert c,d #1"}},

		{"update t set a = |", PG, []string{"update a #0"}},
		{"update t set a = 1, b = |", PG, []string{"update b #0"}},
		{"update t set a = f(1, 2), b = f(3, |", PG, []string{"call .f #1", "update b #0"}},
		{"update t set a = 1, b = b + |", PG, []string{"update b #0"}},
		{`update only public.t as x set "Status" = |`, PG, []string{"update Status #0"}},
		{"update t set x.a = |", PG, []string{"update a #0 of x"}},
		{"update t set a|", PG, nil},
		{"update t set a| = 1", PG, nil},                                     // the = is after the cursor
		{"update t1, t2 set t1.a = |", MySQL, []string{"update a #0 of t1"}}, // before its =
		{"update t set a = 1 where b = |", PG, []string{"where b #0"}},       // the WHERE is another place
		{"update t set a = 1 where |", PG, nil},
		{"update t set a = 1 returning |", PG, nil},

		{"select * from t where a = |", PG, []string{"where a #0"}},
		{"select * from t where a <> |", PG, []string{"where a #0"}},
		{"select * from t where a = 'x|", PG, []string{"where a #0"}},
		{"select * from t where x.a = |", PG, []string{"where a #0 of x"}},
		{`select * from t where "Status" = |`, PG, []string{"where Status #0"}},
		{"select * from t x where x.a in (1, |", PG, []string{"where a #0 of x"}},
		{"select * from t where a in ('x', |", PG, []string{"where a #0"}},
		{"select * from t where a in (|", PG, []string{"where a #0"}},
		{"select * from t where b = 1 and a = |", PG, []string{"where a #0"}},
		{"select * from t where (b = 1 or a = |", PG, []string{"where a #0"}},
		{"select * from t where id in (select x from u where a = |", PG, []string{"where a #0"}},
		{"select * from t join u on t.id = u.|", PG, nil},
		{"select * from t where t . a = |", PG, []string{"where a #0 of t"}},
		{"select * from t join u on t.id = |", PG, []string{"where id #0 of t"}},
		{"select a from t group by b having count(*) > 1 and b = |", PG, []string{"where b #0"}},
		{"select a = |", PG, nil},                                     // not a condition
		{"select * from t order by a = |", PG, nil},                   // nor is this
		{"select * from t where a = 1 |", PG, nil},                    // the value is done
		{"select * from t where a = f(|", PG, []string{"call .f #0"}}, // the condition's value is what is right after the =
		{"select * from t where a = f(1, |", PG, []string{"call .f #1"}},
		{"select * from t where a = 1 -- b = |", PG, nil},
		{"select * from t where f(a = |", PG, []string{"call .f #0"}}, // a call's parenthesis is not a group: nothing of the condition's

		{"select my.f(1, |", PG, []string{"call my.f #1"}},
		{`select "my"."F"(|`, PG, []string{"call my.F #0"}},
		{"select f(g(1, 2), |", PG, []string{"call .f #1"}},
		{"select f('a, b', |", PG, []string{"call .f #1"}},
		{"call proc(1, |", PG, []string{"call .proc #1"}},
		{"select * from f(|", PG, []string{"call .f #0"}},
		{"create function f(a int, |", PG, nil}, // its definition is no call
		{"create table t (a int, |", PG, nil},
		{"select (1, |", PG, nil},
		{"select ", PG, nil},
	} {
		pos := strings.Index(c.sql, "|")
		sql := c.sql
		if pos >= 0 {
			sql = c.sql[:pos] + c.sql[pos+1:]
		} else {
			pos = len(sql)
		}
		if got := flat(ValueContext(sql, pos, c.d)); !slices.Equal(got, c.want) {
			t.Errorf("%q: %q, want %q", c.sql, got, c.want)
		}
	}
}

// The tables a value's statement names come with it.
func TestValueTables(t *testing.T) {
	for _, c := range []struct {
		sql  string
		want []TableRef
	}{
		{"insert into s.t (a) values (|", []TableRef{{Schema: "s", Name: "t"}}},
		{"update t as x set a = |", []TableRef{{Name: "t", Alias: "x"}}},
		{"select * from t x join u on x.a = |", []TableRef{{Name: "t", Alias: "x"}, {Name: "u"}}},
		{"select * from t where a = 'x|", []TableRef{{Name: "t"}}},
	} {
		pos := strings.Index(c.sql, "|")
		v := ValueContext(c.sql[:pos]+c.sql[pos+1:], pos, PG)
		if !reflect.DeepEqual(v.Tables, c.want) {
			t.Errorf("%q: %+v, want %+v", c.sql, v.Tables, c.want)
		}
	}
}

// What is typed at the cursor: a word, a number, a string with its quote, even
// with the closing quote after the cursor; a string closed before it is none.
func TestValuePrefix(t *testing.T) {
	for _, c := range []struct {
		sql, prefix string
		depth       int
	}{
		{"insert into t (a) values (|", "", 1},
		{"insert into t (a) values (pe|", "pe", 1},
		{"insert into t (a) values (12|", "12", 1},
		{"insert into t (a) values ('pe|'", "'pe", 1},
		{"insert into t (a) values ('pe|", "'pe", 1},
		{"insert into t (a) values ('pe'|", "", 1},
		{"insert into t (a) values (1, 'a, b|'", "'a, b", 1},
		{"update t set a = (|", "", 1},
		{"update t set a = |", "", 0},
		{"update t set a = nu|", "nu", 0},
		{"select * from t where a = 'x|", "'x", 0},
		{"select * from t where a in ('x', 'y|", "'y", 1},
	} {
		pos := strings.Index(c.sql, "|")
		sql := c.sql[:pos] + c.sql[pos+1:]
		v := ValueContext(sql, pos, PG)
		if !v.OK || v.Prefix != c.prefix || v.Depth != c.depth || sql[v.Start:pos] != c.prefix {
			t.Errorf("%q: %+v", c.sql, v)
		}
	}
}
