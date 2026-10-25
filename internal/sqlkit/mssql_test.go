package sqlkit

import (
	"strings"
	"testing"
)

// A MSSQL text scans its own names and strings (§9.1, F18.6): [name] with a
// doubled ], "name" an identifier, N'…' a string, @x and #x names.
func TestScanMSSQL(t *testing.T) {
	for _, s := range []string{"select [a]]b] from [t]", `select "a b" from t`, "select N'海]it', n'x''y'"} {
		for _, tk := range Scan(s, MSSQL) {
			if tk.Kind == Quoted || tk.Kind == String {
				what := s[tk.Start:tk.End]
				if !strings.HasPrefix(what, "[") && !strings.HasPrefix(what, `"`) && !strings.Contains(strings.ToLower(what), "'") {
					t.Errorf("%q: %q is neither", s, what)
				}
			}
		}
	}
	kinds := kindsOf(t, "select @p1, #t.x, [order] from [#temp]", MSSQL)
	want := []Kind{Keyword, Ident, Punct, Ident, Punct, Ident, Punct, Quoted, Keyword, Quoted}
	if len(kinds) != len(want) {
		t.Fatalf("kinds %v", kinds)
	}
	for i, k := range want {
		if kinds[i] != k {
			t.Errorf("token %d: %v, want %v", i, kinds[i], k)
		}
	}
}

func kindsOf(t *testing.T, s string, d Dialect) []Kind {
	t.Helper()
	var out []Kind
	for _, tk := range Scan(s, d) {
		if tk.Kind != Space && tk.Kind != Comment {
			out = append(out, tk.Kind)
		}
	}
	return out
}

// A GO of its own line cuts the batches; the GO goes to no statement; a GO
// on the code's line, and a count with it, cut as the server has them cut
// (§9.2, F18.6).
func TestStatementsMSSQL(t *testing.T) {
	s := "select 1;\ngo\n\ncreate table t (a int);\nGO 3\nselect 2; -- trailing\nselect 'go' as [go], 3"
	stmts := Statements(s, MSSQL)
	texts := make([]string, len(stmts))
	for i, st := range stmts {
		texts[i] = strings.TrimSpace(s[st.Start:st.End])
	}
	want := []string{"select 1", "create table t (a int)", "select 2", "select 'go' as [go], 3"}
	if len(texts) != len(want) {
		t.Fatalf("statements %q", texts)
	}
	for i := range want {
		if texts[i] != want[i] {
			t.Errorf("statement %d = %q, want %q", i, texts[i], want[i])
		}
	}
	// the Lead is where the comments above a statement are
	if stmts := Statements("-- a note\ngo\nselect 1", MSSQL); len(stmts) != 1 || stmts[0].Lead != stmts[0].Start {
		t.Errorf("a note above a GO: %+v", stmts)
	}
	// what a GO line is: its own line, its count alone with it, at no depth
	s = "select 1 go\nselect 2\ngo 2\nselect 3\ngo x\nselect 4\nselect (1 + go) , 5\ngo\nselect 6"
	stmts = Statements(s, MSSQL)
	texts = texts[:0]
	for i, st := range stmts {
		if i >= len(texts) {
			texts = append(texts, "")
		}
		texts[i] = strings.TrimSpace(s[st.Start:st.End])
	}
	want = []string{
		"select 1 go\nselect 2",                         // the GO on the code's line is no separator, and nothing cuts between
		"select 3\ngo x\nselect 4\nselect (1 + go) , 5", // go 2 cut; go x is no GO line, a go in the parentheses neither
		"select 6",
	}
	if len(texts) != len(want) {
		t.Fatalf("go lines: %q", texts)
	}
	for i := range want {
		if texts[i] != want[i] {
			t.Errorf("statement %d = %q, want %q", i, texts[i], want[i])
		}
	}
	// N'…' is one string token: its ; cuts nothing
	if stmts := Statements("select N'a;b', 1", MSSQL); len(stmts) != 1 {
		t.Errorf("N'a;b': %d statements", len(stmts))
	}
	// a ] in a [name] doubled: one identifier, and the statement ends where
	// it ends
	if stmts := Statements("select [a]]b]; select 1", MSSQL); len(stmts) != 2 {
		t.Errorf("[a]]b]: %d statements", len(stmts))
	}
}

// The statement the cursor is in over a GO-cut text is the batch's (§9.2).
func TestStmtAtMSSQL(t *testing.T) {
	s := "select 1\ngo\nselect 2"
	stmts := Statements(s, MSSQL)
	if n := StmtAt(s, stmts, len(s)-1); n != 1 {
		t.Errorf("at the end: %d", n)
	}
}

// An AutoLimit is the OFFSET/FETCH a T-SQL page takes, the ORDER BY it must
// have when none is there (§9.4, F18.6). The statements an OFFSET cannot
// stand in — a TOP's (10741), a FOR JSON's or FOR XML's (a syntax error),
// a DISTINCT's or a set operation's without an ORDER BY of its own (145,
// 104) — come back as they are: self-limited, the rows capped.
func TestAutoLimitMSSQL(t *testing.T) {
	s, ok := AutoLimit("select a from t", MSSQL, 100, 0)
	if want := "select a from t\nORDER BY (SELECT NULL)\nOFFSET 0 ROWS FETCH NEXT 100 ROWS ONLY"; !ok || s != want {
		t.Errorf("no order: %q, ok %v", s, ok)
	}
	s, ok = AutoLimit("select a from t order by b", MSSQL, 100, 50)
	if want := "select a from t order by b\nOFFSET 50 ROWS FETCH NEXT 100 ROWS ONLY"; !ok || s != want {
		t.Errorf("with an order: %q, ok %v", s, ok)
	}
	if s, ok := AutoLimit("select a from t offset 5 rows", MSSQL, 100, 0); ok || s != "select a from t offset 5 rows" {
		t.Errorf("an offset of its own: %q, ok %v", s, ok)
	}
	for _, s := range []string{
		"select top 3 id from t",
		"select top 2 id from t order by id",
		"select top (10) percent id from t",
		"select id from t for json path",
		"select id from t for xml raw",
		"select distinct a from t",
		"select a from t union select a from u",
		"select id from t intersect select id from u",
		"select a from t except select a from u",
	} {
		if got, ok := AutoLimit(s, MSSQL, 100, 0); ok || got != s {
			t.Errorf("%q: %q, ok %v, want it as it is", s, got, ok)
		}
	}
	for _, s := range []string{
		"select count(*) from t",
		"with x as (select id from t) select * from x",
		"select distinct a from t order by a",
		"select a from t union select a from u order by a",
	} {
		got, ok := AutoLimit(s, MSSQL, 100, 0)
		if !ok || !strings.Contains(got, "OFFSET 0 ROWS FETCH NEXT 100 ROWS ONLY") {
			t.Errorf("%q: %q, ok %v, want the OFFSET/FETCH", s, got, ok)
		}
	}
}

// A procedure that goes without its EXEC is the EXEC the words take it for
// (§9.2, F18.6): a write, a dangerous one; a save's transaction is not.
func TestBareExecWords(t *testing.T) {
	for _, s := range []string{"sp_rename 'dbo.t', 't2'", "dbo.some_proc 1", "sp_help 't'"} {
		if !BreaksReadOnly(s, MSSQL) || !Dangerous(s, MSSQL) || !IsWrite(s, MSSQL) {
			t.Errorf("%q: bare exec is a write and a dangerous one", s)
		}
	}
	for _, s := range []string{"save transaction sqlmux_page", "select 1", "exec sp_help 't'"} {
		if BeginsBareExec(s, MSSQL) {
			t.Errorf("%q is not one of the name-first calls", s)
		}
	}
}

// The dangerous words of T-SQL: an EXEC is one (F18.6); a SET is none on a
// read-only connection, there being no read only of the server's to undo,
// an isolation level the least of it (§13).
func TestMSSQLDangerousAndReadOnly(t *testing.T) {
	if !Dangerous("exec dbo.do_things", MSSQL) || Dangerous("exec dbo.do_things", PG) {
		t.Error("an EXEC is T-SQL's to ask about")
	}
	if !Dangerous("truncate table t", MSSQL) || !Dangerous("delete from t", MSSQL) {
		t.Error("the words the other engines ask about")
	}
	for _, s := range []string{"set transaction isolation level read uncommitted", "set nocount on", "set language us_english"} {
		if BreaksReadOnly(s, MSSQL) {
			t.Errorf("%q is kept from a read-only connection", s)
		}
	}
	for _, s := range []string{"exec dbo.do_things", "drop table t", "truncate table t", "update t set a = 1"} {
		if !BreaksReadOnly(s, MSSQL) {
			t.Errorf("%q runs on a read-only connection", s)
		}
	}
	for _, s := range []string{"save transaction sqlmux_page", "begin transaction", "commit transaction", "rollback transaction"} {
		if BreaksReadOnly(s, MSSQL) {
			t.Errorf("%q is kept from a read-only connection", s)
		}
	}
}

// The transaction the session is in, the server telling no one (§8.2,
// F18.6): a BEGIN puts it in, a COMMIT or a ROLLBACK of its own takes it
// out, one naming a savepoint or a nested transaction leaves it.
func TestMSSQLTx(t *testing.T) {
	for _, s := range []string{"begin transaction", "begin tran", "BEGIN distributed transaction"} {
		if MSSQLTx(s, 'I') != 'T' {
			t.Errorf("%q: %c", s, MSSQLTx(s, 'I'))
		}
	}
	for _, s := range []string{"commit", "commit transaction", "COMMIT WORK;", "rollback", "rollback tran"} {
		if MSSQLTx(s, 'T') != 'I' {
			t.Errorf("%q: %c", s, MSSQLTx(s, 'T'))
		}
	}
	for _, s := range []string{"rollback transaction sqlmux_save", "commit tran sqlmux_save"} {
		if MSSQLTx(s, 'T') != 'T' {
			t.Errorf("%q: %c", s, MSSQLTx(s, 'T'))
		}
	}
	if MSSQLTx("select 1", 'T') != 'T' || MSSQLTx("select 1", 'I') != 'I' {
		t.Error("a select changes nothing")
	}
}

// A name the T-SQL grammar reserves, or one with its oddities, takes its
// brackets; a literal is the quotes doubled (§9.6, F18.6).
func TestMSSQLQuoteAndLiteral(t *testing.T) {
	for _, name := range []string{"order", "user", "top", "a b", "transaction"} {
		if !MSSQL.NeedsQuote(name) {
			t.Errorf("%q needs its brackets", name)
		}
	}
	if MSSQL.NeedsQuote("order_id") || MSSQL.NeedsQuote("nvarchar") {
		t.Error("a plain name does not")
	}
	if Literal("it's", MSSQL) != "'it''s'" || Literal(`a\b`, MSSQL) != `'a\b'` {
		t.Error("the literal: a quote doubled, a backslash nothing")
	}
}
