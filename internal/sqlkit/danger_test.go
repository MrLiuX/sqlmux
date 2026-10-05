package sqlkit

import "testing"

// What asks before it runs: a DELETE or UPDATE with no WHERE of its own
// (§13), TRUNCATE, DROP, an ALTER TABLE that drops a column, and EXPLAIN
// ANALYZE of any of them; a WHERE in a subquery or a CTE is not the
// statement's, text in a string or a comment is not a word.
func TestDangerous(t *testing.T) {
	both := map[string]bool{
		"delete from t": true, "DELETE FROM t": true, "update t set a = 1": true, "  \n delete\nfrom t": true,
		"delete from t where id = 1": false, "update t set a = 1 where id = 1": false, "UPDATE t SET a = 1 WHERE a > 0": false,
		"delete from t where": false, // the word is there; the database says the rest
		"update t set a = (select max(a) from u where u.id = 1)": true, "delete from t where id in (select id from u)": false,
		"delete from t where id in (select id from u where x)": false,
		"delete from t -- where id = 1":                        true, "delete from t /* where */": true, "delete from t returning 'where'": true,
		"update t set a = 'where x'": true, `update t set "where" = 1`: true, "update t set a = 1 where a = 'x'": false,
		"truncate t": true, "truncate table t restart identity": true, "TRUNCATE t": true,
		"drop table t": true, "drop table if exists t": true, "drop database d": true, "drop schema s cascade": true,
		"drop index i": true, "drop view v": true, "drop table t, u": true,
		"alter table t drop column c": true, "ALTER TABLE t DROP COLUMN IF EXISTS c": true, "alter table t drop c": true,
		"alter table t add column c int, drop column d": true, "alter table only public.t drop column c cascade": true,
		"alter table t add column c int": false, "alter table t drop constraint c": false, "alter table t alter column c drop default": false,
		"alter table t alter column c drop not null": false, "alter table t alter column c drop identity": false,
		"alter table t alter column c drop expression": false, "alter table t alter c drop default, drop c": true, "alter table t alter c drop default": false, "alter table t alter c drop not null": false,
		"alter table t drop partition p": true, "alter table t rename to u": false, "alter index i rename to j": false,
		"alter table t drop": false, "alter publication p drop table t": false, "alter extension e drop function f()": false, "alter type ty drop attribute a": false,
		"select 1": false, "select * from t for update": false, "": false, "selec 1": false,
		"insert into t values (1) on conflict (id) do update set a = 1": false, "insert into t select * from u": false,
		"with x as (select 1) select * from x": false, "with x as (select 1) delete from t": true,
		"with x as (select 1) delete from t where id in (select * from x)":    false,
		"with x as (delete from t returning *) select * from x":               true, // the CTE's own DELETE, in parentheses
		"with d as (delete from t returning *) insert into a select * from d": true, "copy (delete from t returning *) to stdout": true,
		"with d as (delete from t returning *) select * from d where x = 1": true, // not the DELETE's WHERE
		"with d as (delete from t where x returning *) select * from d":     false, "with d as (select 1) select * from (select 2) where x": false,
		"with d as (delete from t returning *), e as (select 1 where true) select 1": true, // a sibling's WHERE is as deep, not the DELETE's
		"with x as (select 1) select * from t for update":                            false, "with x as (select 1) insert into t values (1) on conflict do update set a = 1": false,
		"with x as (select 1) update t set a = 1": true, "with recursive x as (select 1) update t set a = 1 where a in (select * from x)": false,
		"(delete from t)": true, "(delete from t where a)": false,
		"explain delete from t": false, "explain analyze delete from t where a": false, "explain analyze select 1": false,
		"explain (analyze false) delete from t": false, "explain with x as (delete from t returning *) select 1": false,
		"explain (analyze": false,
	}
	for s, want := range both {
		if got := Dangerous(s, PG); got != want {
			t.Errorf("PG Dangerous(%q) = %v", s, got)
		}
		if got := Dangerous(s, MySQL); got != want {
			t.Errorf("MySQL Dangerous(%q) = %v", s, got)
		}
	}
	for s, want := range map[string]bool{
		"delete from t limit 10": true, "delete from t order by id limit 1": true, "delete from t where id = 1 limit 1": false,
		"delete t1 from t1 join t2 on t1.id = t2.id": true, "delete t1 from t1 join t2 on t1.id = t2.id where t2.x": false,
		"delete low_priority quick ignore from t": true, "update ignore t set a = 1": true, "update t set a = 1 limit 5": true,
		"update t1, t2 set t1.a = t2.a where t1.id = t2.id": false, "update t1 join t2 using (id) set t1.a = 1": true,
		"insert into t values (1) on duplicate key update a = 1": false, "replace into t values (1)": false,
		"drop temporary table t": true, "drop table if exists `t`": true, "truncate table t": true,
		"alter table t drop column c": true, "alter table t drop column a, drop b": true, "alter table t drop index i, add index j (a)": false,
		"alter table t drop index i": false, "alter table t drop key k": false, "alter table t drop primary key": false,
		"alter table t drop foreign key f": false, "alter table t drop check c": false, "alter table t drop constraint c": false,
		"alter table t drop partition p": true, "alter table t truncate partition p": true,
		"alter table t drop expression": true,
		"explain analyze delete from t": false, "explain analyze drop table t": false, // MySQL only plans it

		"alter table t alter column c drop default": false, "alter table t drop column `where`": true,
		"/*! delete from t */": true, "/*!40101 delete from t where a */": false, "# delete\nselect 1": false,
		"delete from t -- c\nwhere a = 1": false, "delete from t #c\n": true,
		"with x as (select 1) delete from t": true,
	} {
		if got := Dangerous(s, MySQL); got != want {
			t.Errorf("MySQL Dangerous(%q) = %v", s, got)
		}
	}
	for s, want := range map[string]bool{ // PG only: dollar quotes and ONLY, USING, the names of its own
		"delete from only t": true, "delete from t using u": true, "delete from t using u where t.id = u.id": false,
		"delete from t where current of c": false, "update t set a = $$ where $$": true, "delete from t where a = $q$ x $q$": false,
		"update t set a = b from u": true, "update t set a = u.b from u where u.id = t.id": false,
		"do $$ begin delete from t; end $$": false, "explain analyze delete from t": true, "explain (analyze) update t set a = 1": true,
		"explain analyze verbose truncate t": true, "explain analyze drop table t": true,
		"alter table t drop key": true, "alter table t drop index": true, "alter table t drop primary key": true, // columns of those names
		"alter table t drop partition p": true, "alter table t drop expression": true, "alter table t alter column c drop identity": false,
		"alter table t truncate partition p": false,
	} {
		if got := Dangerous(s, PG); got != want {
			t.Errorf("PG Dangerous(%q) = %v", s, got)
		}
	}
}
