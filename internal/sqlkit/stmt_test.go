package sqlkit

import (
	"strings"
	"testing"
)

func TestStatements(t *testing.T) {
	for _, c := range []struct {
		d    Dialect
		in   string
		want []string
	}{
		{PG, "select 1; select 2;", []string{"select 1", "select 2"}},
		{PG, "-- lead\n  select 1 -- tail\n;\n\n/* x */ select 2", []string{"select 1", "select 2"}},
		{PG, "select ';', \"a;b\", $$;$$ /* ; */ -- ;\n; ;; select (1;2)", []string{`select ';', "a;b", $$;$$`, "select (1;2)"}},
		{PG, "  \n-- only a comment\n", nil},
		{MySQL, "select '\\';' # ;\n; select `a;`", []string{`select '\';'`, "select `a;`"}},
		// the /*! and */ around its words go with a statement
		{MySQL, "/*!40101 SET NAMES utf8 */;\nSELECT 5 /*! +1 */ ; /*! select 2 */ /* c */", []string{"/*!40101 SET NAMES utf8 */", "SELECT 5 /*! +1 */", "/*! select 2 */"}},
		{MySQL, "/*! SELECT 1 /* c */ */; /*! /* c */ SELECT 2 */; /* c */ select 3 -- c", []string{"/*! SELECT 1 /* c */ */", "/*! /* c */ SELECT 2 */", "select 3"}},
	} {
		var got []string
		for _, st := range Statements(c.in, c.d) {
			got = append(got, c.in[st.Start:st.End])
		}
		if strings.Join(got, "|") != strings.Join(c.want, "|") {
			t.Errorf("%q: %q, want %q", c.in, got, c.want)
		}
	}
}

func TestStmtAt(t *testing.T) {
	s := "\n\nselect 1;\n\n-- about\n/* the next */\n  select 2; select 3;\n-- note\n\nselect 4 /* x\ny */\n\n"
	stmts := Statements(s, PG)
	at := func(sub string, off int) int { return strings.Index(s, sub) + off }
	for _, c := range []struct{ pos, want int }{
		{0, 0},                   // before the first
		{at("1", 0), 0},          // in it
		{at(";\n\n-", 2), 0},     // the blank line after it
		{at("-- about", 3), 1},   // a comment right above the next
		{at("/* the", 0), 1},     // and another
		{at("  select 2", 0), 1}, // its line's indent
		{at("select 3", 0), 2},   // the second on a line
		{at("2; select", 1), 1},  // the ; of the first
		{at("-- note", 0), 2},    // a comment after it, a blank line below
		{at("y */", 0), 3},       // a comment in it
		{len(s) - 1, 3},          // the blank lines at the end
	} {
		if got := StmtAt(s, stmts, c.pos); got != c.want {
			t.Errorf("at %d (%q): %d, want %d", c.pos, s[c.pos:min(c.pos+8, len(s))], got, c.want)
		}
	}
	if StmtAt("", nil, 0) != -1 {
		t.Error("no statements")
	}
}

func TestIsRead(t *testing.T) {
	for s, want := range map[string]bool{
		"select 1": true, "  SELECT * from t": true, "((select 1) union (select 2))": true,
		"show search_path": true, "table t": true, "values (1)": true, "desc t": true,
		"explain select 1": true, "explain delete from t": true, "explain verbose update t set a = 1": true,
		"explain analyze select 1": true, "explain analyze delete from t": false,
		"EXPLAIN (ANALYZE, BUFFERS) update t set a = 1": false, "explain (analyze false) delete from t": true,
		"explain (analyze off, costs) delete from t": true, "explain (format json) delete from t": true,
		"explain (analyze) select 1":                                                               true,
		"with x as (select 1) select * from x":                                                     true,
		"with x as (delete from t returning *) select * from x":                                    false,
		"with x as (select 1) insert into t select * from x":                                       false,
		"with recursive x as (select 1) merge into t using x on true when matched then do nothing": false,
		"select a into t2 from t":                                                                  false,
		"select * from t where id in (select 1)":                                                   true,
		"select * from t for update":                                                               false,
		"select * from t for no key update nowait":                                                 false,
		"select * from (select * from t for share) s":                                              false,
		"select * from t for key share":                                                            false,
		"select * from t lock in share mode":                                                       false,
		"describe t":                                                                               true,
		"with x as (select 1 a) (select * into t2 from x)":                                         false,
		"select substring('abc' from 1 for 2)":                                                     true,
		"select 'delete', \"update\" -- insert":                                                    true,
		"delete from t":                                                                            false, "update t set a = 1": false, "insert into t values (1)": false,
		"set x = 1": false, "begin": false, "": false, "create table t (a int)": false,
	} {
		if got := IsRead(s, PG); got != want {
			t.Errorf("IsRead(%q) = %v", s, got)
		}
	}
}

func TestAutoLimit(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{"select 1", "select 1\nLIMIT 101"},
		{"select 1;", "select 1\nLIMIT 101"},
		{"select 1; -- done", "select 1 -- done\nLIMIT 101"},
		{"select * from t -- why", "select * from t -- why\nLIMIT 101"},
		{"with x as (select 1) select * from x", "with x as (select 1) select * from x\nLIMIT 101"},
		{"table t", "table t\nLIMIT 101"},
		{"values (1), (2)", "values (1), (2)\nLIMIT 101"},
		{"(select 1 limit 5)", "(select 1 limit 5)"},
		{"((select 1 limit 5));", "((select 1 limit 5));"},
		{"(select 1 fetch first 1 rows only)", "(select 1 fetch first 1 rows only)"},
		{"with x as (select 1) (select * from x limit 5)", "with x as (select 1) (select * from x limit 5)"},
		{"(select 1 limit 5) union select 2", "(select 1 limit 5) union select 2\nLIMIT 101"},
		{"(select 1 limit 5) union (select 2)", "(select 1 limit 5) union (select 2)\nLIMIT 101"},
		{"with x as (select 1) select * from (select 1 limit 5) s", "with x as (select 1) select * from (select 1 limit 5) s\nLIMIT 101"},
		{"select * from t where id in (select id from u limit 3)", "select * from t where id in (select id from u limit 3)\nLIMIT 101"},
		{"select * from t limit 5", "select * from t limit 5"},
		{"select * from t LIMIT 5, 10", "select * from t LIMIT 5, 10"},
		{"select * from t offset 5 fetch first 3 rows only", "select * from t offset 5 fetch first 3 rows only"},
		{"select * from t for update", "select * from t for update"},
		{"select a into t2 from t", "select a into t2 from t"},
		{"show search_path", "show search_path"},
		{"explain select 1", "explain select 1"},
		{"delete from t", "delete from t"},
		{"with x as (delete from t returning *) select * from x", "with x as (delete from t returning *) select * from x"},
		{"select 'limit'", "select 'limit'\nLIMIT 101"},
	} {
		if got := AutoLimit(c.in, PG, 101); got != c.want {
			t.Errorf("%q: %q, want %q", c.in, got, c.want)
		}
	}
	if got := AutoLimit("/*! select 5 */ /*! limit 1 */", MySQL, 101); got != "/*! select 5 */ /*! limit 1 */" {
		t.Errorf("a LIMIT in /*! */: %q", got)
	}
}

func TestHasSemicolon(t *testing.T) {
	for s, want := range map[string]bool{
		"1=1; drop table t": true, "(a = 1;)": true, "a = ';'": false, `"a;b" = 1`: false,
		"a = 1 -- ;": false, "a = 1 /* ; */": false, "a = $$;$$": false,
	} {
		if got := HasSemicolon(s, PG); got != want {
			t.Errorf("HasSemicolon(%q) = %v", s, got)
		}
	}
	if HasSemicolon("a = '\\';'", MySQL) || !HasSemicolon("a = 1 # x\n;", MySQL) {
		t.Error("MySQL strings and # comments")
	}
}

func TestFirstWord(t *testing.T) {
	for s, want := range map[string]string{
		"-- make it\nCREATE table t (a int)": "create",
		"(select 1)":                         "select",
		"/* x */ drop table t":               "drop",
		"  ":                                 "",
	} {
		if got := FirstWord(s, PG); got != want {
			t.Errorf("%q: %q, want %q", s, got, want)
		}
	}
	if got := FirstWord("/*!40101 SET NAMES utf8 */", MySQL); got != "set" {
		t.Errorf("/*!40101 SET: %q", got)
	}
}

// IsQuery takes what starts as a query, past comments and parentheses.
func TestIsQuery(t *testing.T) {
	for s, want := range map[string]bool{
		"select 1": true, "  SELECT 1": true, "values (1)": true, "table t_order": true,
		"with x as (select 1) select * from x":                 true,
		"-- note\n/* why */ ((select 1) union all (select 2))": true,
		"show search_path": false, "explain select 1": false, "delete from t": false,
		"selectx 1": false, `"select"`: false, "'select'": false, "": false, "(": false,
	} {
		if got := IsQuery(s, PG); got != want {
			t.Errorf("IsQuery(%q) = %v", s, got)
		}
	}
}

// pgCommands are the first words of PG 17's SQL Commands (the docs'
// Reference, I. SQL Commands), each statement's, and analyse, ANALYZE
// spelled as PG takes it too.
var pgCommands = strings.Fields(`abort alter analyse analyze begin call checkpoint close cluster comment commit
	copy create deallocate declare delete discard do drop end execute explain fetch grant import
	insert listen load lock merge move notify prepare reassign refresh reindex release reset revoke
	rollback savepoint security select set show start table truncate unlisten update vacuum values with`)

// A read-only connection keeps back writes and what turns read only off,
// whatever the case and quoting; transaction control and other settings
// go (§13).
func TestBreaksReadOnly(t *testing.T) {
	for s, want := range map[string]bool{
		"delete from t": true, "vacuum": true, "call p()": true, "explain analyze delete from t": true,
		"set transaction read only": true, "SET SESSION CHARACTERISTICS AS TRANSACTION READ WRITE": true,
		"set default_transaction_read_only to default": true, `set local "Transaction_Read_Only" = off`: true,
		"SET SESSION SESSION CHARACTERISTICS AS TRANSACTION READ WRITE": true,
		"begin read write": true, "start transaction isolation level serializable, read write": true,
		"select 1": false, "selec 1": false, "": false, "begin": false, "begin read only": false, "commit": false,
		"end": false, "abort": false, "rollback to savepoint s": false, "release s": false, "savepoint s": false,
		"set search_path to public": false, "set session authorization default": false, "set": false,
		"reset default_transaction_read_only": false, "reset all": false, "discard all": false,
	} {
		if got := BreaksReadOnly(s, PG); got != want {
			t.Errorf("BreaksReadOnly(%q) = %v", s, got)
		}
	}
	for s, want := range map[string]bool{
		"SET GLOBAL TRANSACTION READ WRITE": true, "set persist_only transaction_read_only = 0": true,
		"set @@transaction_read_only = 0": true, "SET @@Session.transaction_read_only = 0": true,
		"start transaction read write": true, "/*!40101 SET SESSION transaction_read_only = 0 */": true,
		"set @a = (select 1, 2), names utf8mb4, @@global.transaction_read_only = off": true, "set": false,
		"set @transaction_read_only = 0": false, "set @@session.sql_mode = ''": false, "set names utf8mb4": false,
		"set autocommit = 0, @x = 1": false, "use sqlmux": false, "start transaction read only": false,
		"set session sql_mode = 'transaction_read_only'": false, "check table t": false,
		// the server's: RESET, a SET of the whole server's, the first words of MySQL's writes
		"reset persist x": true, "set global max_connections = 10": true, "set @a = 1, persist_only x = 2": true,
		"set @@persist.x = 1": true, "kill 5": true, "flush tables": true, "xa start 'x'": true, "shutdown": true,
	} {
		if got := BreaksReadOnly(s, MySQL); got != want {
			t.Errorf("BreaksReadOnly(%q, MySQL) = %v", s, got)
		}
	}
}

// IsWrite takes what IsRead doesn't but for a first word no keyword: a
// typo goes to the database. Every PG statement's first word is one: what
// IsRead doesn't take of them is a write.
func TestIsWrite(t *testing.T) {
	for _, w := range pgCommands {
		if s := w + " x"; IsWrite(s, PG) == IsRead(s, PG) {
			t.Errorf("%s: IsWrite %v, IsRead %v", s, IsWrite(s, PG), IsRead(s, PG))
		}
	}
	for _, s := range []string{"selec 1", "sleect 1"} {
		if IsWrite(s, PG) {
			t.Errorf("a typo, %s, is kept back", s)
		}
	}
	for s, want := range map[string]bool{
		"delete from t": true, "with x as (delete from t returning *) select * from x": true, "(insert into t values (1))": true,
		"select 1": false, "selec 1": false, "t_order": false, "": false, "explain analyze delete from t": true,
	} {
		if got := IsWrite(s, PG); got != want {
			t.Errorf("IsWrite(%q) = %v", s, got)
		}
	}
}

// What psql's AUTOCOMMIT off sends with no BEGIN before it: transaction
// control but savepoints, and what runs in no transaction block.
func TestNoBegin(t *testing.T) {
	for s, want := range map[string]bool{
		"commit": true, "END": true, "abort": true, "start transaction": true, "begin": true, "rollback": true,
		"rollback to savepoint s": true, "prepare transaction 'x'": true, "vacuum t": true, "cluster": true, "cluster;": true,
		"create database x": true, "create unique index concurrently i on t (a)": true, "create index concurrently i on t (a)": true,
		"alter system set x = 1": true, "drop database x": true, "drop index concurrently i": true, "reindex table concurrently t": true,
		"discard all": true,
		"savepoint s": false, "release s": false, "prepare p as select 1": false, "cluster t": false, "create index i on t (a)": false,
		"create table t (a int)": false, "drop index i": false, "alter table t add b int": false, "discard plans": false,
		"update t set a = 1": false, "select 1": false, "": false,
	} {
		if got := NoBegin(s, PG); got != want {
			t.Errorf("NoBegin(%q) = %v", s, got)
		}
	}
}

// MySQL's transaction goes by the statements run (§8.2): BEGIN, START
// TRANSACTION and … AND CHAIN begin one; COMMIT, ROLLBACK but to a
// savepoint, and what commits implicitly end it; the rest leave it be.
func TestMySQLTx(t *testing.T) {
	for _, c := range []struct {
		s      string
		tx, to byte
	}{
		{"begin", 'I', 'T'}, {"BEGIN WORK", 'I', 'T'}, {"start transaction read only", 'I', 'T'},
		{"/*!40101 START TRANSACTION */", 'I', 'T'}, {"begin", 'T', 'T'},
		{"commit", 'T', 'I'}, {"rollback", 'T', 'I'}, {"commit work and chain", 'T', 'T'},
		{"rollback and no chain", 'T', 'I'}, {"ROLLBACK AND CHAIN", 'T', 'T'},
		{"rollback to s", 'T', 'T'}, {"rollback work to savepoint s", 'T', 'T'},
		{"/* c */ rollback to s", 'T', 'T'}, {"rollback/**/to s", 'T', 'T'},
		{"create table t (a int)", 'T', 'I'}, {"create temporary table t (a int)", 'T', 'T'},
		{"drop temporary table t", 'T', 'T'}, {"drop table t", 'T', 'I'}, {"alter table t add b int", 'T', 'I'},
		{"truncate t", 'T', 'I'}, {"rename table a to b", 'T', 'I'}, {"grant select on *.* to u", 'T', 'I'},
		{"lock tables t write", 'T', 'I'}, {"unlock tables", 'T', 'I'}, {"analyze table t", 'T', 'I'},
		{"flush privileges", 'T', 'I'}, {"reset persist", 'T', 'T'}, {"reset binary logs and gtids", 'T', 'I'},
		{"set password = 'x'", 'T', 'I'}, {"set autocommit = 1", 'T', 'I'}, {"set @@session.autocommit=ON", 'T', 'I'},
		{"set autocommit = 0", 'T', 'T'}, {"load index into cache t", 'T', 'I'}, {"load data infile 'f' into table t", 'T', 'T'},
		{"insert into t values (1)", 'T', 'T'}, {"select 1", 'I', 'I'}, {"savepoint s", 'T', 'T'}, {"release savepoint s", 'T', 'T'},
	} {
		if got := MySQLTx(c.s, c.tx); got != c.to {
			t.Errorf("MySQLTx(%q, %c) = %c, want %c", c.s, c.tx, got, c.to)
		}
	}
	for s, want := range map[string]bool{
		"begin": true, "start transaction": true, "commit": true, "rollback to s": true, "set transaction read write": true,
		"create table t (a int)": true, "drop database x": true, "lock tables t read": true, "set autocommit = 1": true,
		"set session transaction isolation level read committed": false, "create temporary table t (a int)": false,
		"insert into t values (1)": false, "set names utf8mb4": false, "use sqlmux": false, "select 1": false,
	} {
		if got := NoBegin(s, MySQL); got != want {
			t.Errorf("NoBegin(%q, MySQL) = %v", s, got)
		}
	}
}
