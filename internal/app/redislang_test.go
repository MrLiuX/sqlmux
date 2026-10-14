package app

import (
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/exp/golden"

	"github.com/kackerx/sqlmux/internal/config"
	"github.com/kackerx/sqlmux/internal/db"
	"github.com/kackerx/sqlmux/internal/db/redis"
)

// A command is a line: blank lines and # lines are none, a line is cut of the
// blanks around it, and the cursor is on a command when it is on its line.
func TestRedisLangStmts(t *testing.T) {
	l := redisLang{}
	text := "GET a\n\n# a comment\n  SET b \"x y\"  \r\n\t\nDEL c"
	var got []string
	stmts := l.Stmts(text)
	for _, s := range stmts {
		got = append(got, text[s.Start:s.End])
	}
	if want := []string{"GET a", `SET b "x y"`, "DEL c"}; !slices.Equal(got, want) {
		t.Fatalf("stmts %q", got)
	}
	for pos, want := range map[int]int{0: 0, 4: 0, 5: 0, 6: -1, 10: -1, 20: 1, 36: -1, 38: 2} { // the end of GET a; a blank line; the comment; SET; the tab line; DEL
		if i := l.At(text, stmts, pos); i != want {
			t.Errorf("pos %d: %d, want %d", pos, i, want)
		}
	}
	if i := l.At(text, stmts, len(text)); i != 2 {
		t.Errorf("at the end: %d", i)
	}
	if got := l.Stmts("# only\n\n"); len(got) != 0 {
		t.Errorf("no commands: %v", got)
	}
}

// What a read-only connection keeps back and what is asked about is the server's
// list, which the console's language has; the rest of the language's methods are
// what a connection with no transactions and no schema needs (§9.8).
func TestRedisLangJudges(t *testing.T) {
	cs := redis.Commands{}
	c := &Conn{cmds: cs}
	c.eng = nil
	l := c.lang()
	if _, ok := l.(redisLang); !ok {
		t.Fatalf("lang %T", l)
	}
	if !l.Writes("GET k") { // an empty list: no command is known, all are writes
		t.Error("an unknown command is a write")
	}
	if text, pageable := l.Page("GET k", 501, 0); text != "GET k" || pageable {
		t.Errorf("page %q %v", text, pageable)
	}
	if l.Resets("SELECT 1") || l.DDL("FLUSHALL") || l.EndsTx("EXEC") != "" || !l.NoBegin("SET a b") {
		t.Error("a Redis command has no schema, DDL or transaction of the SQL kind")
	}
}

// redisConsole is a query on a Redis connection whose Main answers from d, the
// server's list of commands cs.
func redisConsole(t *testing.T, d *execDB, cs redis.Commands, text string) (*App, *queryTab) {
	t.Helper()
	a, c := inRun(t, d, text)
	conn := a.sess.Conns[0]
	conn.Spec.Engine, conn.drv, conn.eng, conn.cmds, conn.DB = "redis", kvDriver{}, nil, cs, "0"
	return a, c
}

func someCommands() redis.Commands {
	cs := redis.Commands{}
	for name, cmd := range map[string]redis.Command{
		"get": {Flags: []string{"readonly"}}, "set": {Flags: []string{"write"}}, "flushall": {Flags: []string{"write"}, Categories: []string{"@dangerous"}},
	} {
		cs[name] = cmd
	}
	return cs
}

// ↵ runs the line the cursor is on, as it is: no LIMIT, no search_path, one result
// a command, a comment or a blank line none; a read-only connection keeps a write
// back, and a dangerous command is asked about (F16.4).
func TestRedisConsoleRuns(t *testing.T) {
	d := &execDB{res: map[string]db.Result{"GET k": {Cols: []db.Col{{Name: "value"}}, Rows: [][]db.Val{{{S: "v"}}}, Tag: "GET"}}}
	a, c := redisConsole(t, d, someCommands(), "# note\nGET k\n\nSET k 1\nFLUSHALL")
	feed(t, a, "gg") // on the comment: nothing
	press(t, a, "<CR>")
	if len(d.ran) != 0 {
		t.Fatalf("a comment ran: %q", d.ran)
	}
	feed(t, a, "j")
	press(t, a, "<CR>")
	if !slices.Equal(d.ran, []string{"GET k"}) {
		t.Fatalf("ran %q", d.ran)
	}
	if got := logText(a); got != "query_1  GET k  1 行 · 0s" {
		t.Errorf("log %q", got)
	}
	// read only: a write is kept back for the whole run
	a.sess.Conns[0].readOnly = true
	c.ed.Load("GET k\nSET k 1")
	d.ran = nil
	feed(t, a, "ggVG")
	deliver(a, a.run("query.run", 0))
	if len(d.ran) != 0 || !strings.Contains(logText(a), "只读连接，整次都没有执行") {
		t.Errorf("read only: ran %q, log %q", d.ran, logText(a))
	}
	a.sess.Conns[0].readOnly = false
	// a dangerous one is asked about
	a.confirmDangerous = true
	c.ed.Load("FLUSHALL")
	feed(t, a, "gg")
	press(t, a, "<CR>")
	if a.confirm == nil || !strings.Contains(a.confirm.text, "FLUSHALL") || len(d.ran) != 0 {
		t.Errorf("a dangerous command: confirm %+v, ran %q", a.confirm, d.ran)
	}
}

// A reply longer than a page is held whole: it is cut to a page, the next one is
// cut out of it as ] says and the command is not sent again; everything is what
// an export of all has (F16.4).
func TestRedisReplyPagesInMemory(t *testing.T) {
	var big db.Result
	big.Cols = []db.Col{{Name: "value"}}
	for i := range 1200 {
		big.Rows = append(big.Rows, []db.Val{{S: strconv.Itoa(i)}})
	}
	d := &execDB{res: map[string]db.Result{"LRANGE l 0 -1": big}}
	a, _ := redisConsole(t, d, someCommands(), "LRANGE l 0 -1")
	inDir(t)
	press(t, a, "<CR>")
	rt := resultOf(a.win().Result)
	if rt == nil || rt.mem == nil || len(rt.page.Rows) != 500 || !rt.page.Truncated || rt.page.Rows[0][0].S != "0" || !slices.Equal(d.rows, []int{0}) {
		t.Fatalf("first page: %v, asked for %v rows", rt != nil && rt.mem != nil, d.rows)
	}
	a.focusPane(a.win().Result.ID)
	for _, rows := range []struct {
		first string
		n     int
	}{{"500", 500}, {"1000", 200}} {
		deliver(a, a.resultPage(1))
		if rt.page.Rows[0][0].S != rows.first || len(rt.page.Rows) != rows.n || rt.offset == 0 {
			t.Fatalf("next page: %q %d offset %d", rt.page.Rows[0][0].S, len(rt.page.Rows), rt.offset)
		}
	}
	if rt.page.Truncated || a.resultPage(1) != nil {
		t.Error("the last page has a next")
	}
	deliver(a, a.resultPage(-1))
	if rt.page.Rows[0][0].S != "500" {
		t.Errorf("back: %q", rt.page.Rows[0][0].S)
	}
	if len(d.ran) != 1 {
		t.Errorf("the command went again: %q", d.ran)
	}
	exportAt(t, a, exportCSVAll)
	files, _ := filepath.Glob("*.csv")
	if len(files) != 1 {
		t.Fatalf("files %v", files)
	}
	if b, _ := os.ReadFile(files[0]); strings.Count(string(b), "\n") != 1201 || len(d.ran) != 1 {
		t.Errorf("export: %d lines, ran %q", strings.Count(string(b), "\n"), d.ran)
	}
}

// A line with a password in it runs but is not kept in the history (F16.4).
func TestRedisHistoryKeepsNoPassword(t *testing.T) {
	d := &execDB{res: map[string]db.Result{"GET k": {Tag: "GET"}}}
	a, c := redisConsole(t, d, someCommands(), "AUTH hunter2\nGET k\nCONFIG SET requirepass x")
	feed(t, a, "gg")
	press(t, a, "<CR>")
	feed(t, a, "j")
	press(t, a, "<CR>")
	feed(t, a, "j")
	press(t, a, "<CR>")
	_ = c
	hs, err := config.LoadHistory()
	if err != nil || len(hs) != 1 || hs[0].SQL != "GET k" || len(d.ran) != 3 {
		t.Errorf("history %+v %v, ran %q", hs, err, d.ran)
	}
}

// The command names the server lists are keywords, with the subcommand that follows;
// strings, numbers and # lines have their colors; a name it does not list has none;
// a server that said nothing colors no name (F16.4).
func TestRedisLangColors(t *testing.T) {
	a := wide(160, 45)
	th := a.theme
	cs := someCommands()
	cs["config"] = redis.Command{}
	cs["config|get"] = redis.Command{}
	text := "set k \"a b\" 12\n  CONFIG get get\nget set\nnosuch 'q\\'s' #x\n# set 1"
	at := func(s string) int { return strings.Index(text, s) }
	got := redisLang{cmds: cs}.Colors(th, text, nil)
	for _, c := range []struct {
		at   string
		want any
	}{
		{"set k", th.Keyword}, {"k \"a", th.Fg}, {"\"a b\"", th.SQLString}, {"b\" 12", th.SQLString}, {"12", th.Number},
		{"CONFIG", th.Keyword}, {"get get", th.Keyword}, {"get\nget set", th.Fg}, {"set\nnosuch", th.Fg}, {"nosuch", th.Fg}, {"'q\\'s'", th.SQLString}, {"s' #x", th.SQLString}, {"#x", th.Fg},
		{"# set 1", th.Comment}, {"set 1", th.Comment},
	} {
		i := at(c.at)
		if got[i] != c.want {
			t.Errorf("%q at %d: %v, want %v", c.at, i, got[i], c.want)
		}
	}
	for i, col := range (redisLang{}).Colors(th, text, nil) {
		if col == th.Keyword {
			t.Fatalf("a keyword at %d with no list of commands", i)
		}
	}
}

// A Redis console says its db in the title, which opens the dbs of the server as
// db<n>, the one it is on first; picking another puts the query on that db's own
// connection, and the one it is on does nothing (F16.4).
func TestRedisConsoleDBDropdown(t *testing.T) {
	a, c := redisTree(t, someKeys())
	c.State = connReady
	a.run("query.new", 0)
	q := queryOf(a.focused())
	if !strings.Contains(a.render().String(), "db0 ▾") {
		t.Error("no db in the title")
	}
	c.Databases = []string{"0"} // one db is a list too
	a.openQueryDrop(dropSchema)
	if a.drop == nil || a.drop.kind != dropDatabase {
		t.Fatalf("one db: drop %+v", a.drop)
	}
	a.drop, c.Databases = nil, []string{"0", "1"}
	a.openQueryDrop(dropSchema)
	if a.drop == nil || a.drop.kind != dropDatabase {
		t.Fatalf("drop %+v", a.drop)
	}
	if items, cur := a.dropItems(); !slices.Equal(items, []string{"db0", "db1"}) || cur != 0 {
		t.Fatalf("items %v current %d", items, cur)
	}
	if cmd := a.dropPick(0); cmd != nil || a.drop != nil || q.conn != c {
		t.Errorf("the db it is on: %v %v", cmd, q.conn == c)
	}
	a.openQueryDrop(dropSchema)
	cmd := a.dropPick(1)
	if cmd == nil || q.want != c.inDB("1") {
		t.Errorf("db1: want %v", q.want)
	}
}

// The palette lists the formatting of a query for a SQL connection and not for
// Redis's, which has none (F16.4).
func TestPaletteHidesSQLOnlyCommands(t *testing.T) {
	has := func(a *App) bool {
		a.openPalette(">")
		defer func() { a.palette = nil }()
		return slices.ContainsFunc(a.paletteItems(), func(it paletteItem) bool { return it.kind == itemCommand && it.id == "query.format" })
	}
	a := wide(160, 45)
	if !has(a) {
		t.Error("SQL: not listed")
	}
	c := a.sess.Conns[0]
	c.Spec.Engine, c.drv, c.eng = "redis", kvDriver{}, nil
	if has(a) {
		t.Error("Redis: listed")
	}
}

// someDocs is what a server says of SET and CONFIG, as COMMAND DOCS gives it.
func someDocs() redis.Docs {
	tok := func(t string) redis.Arg { return redis.Arg{Name: strings.ToLower(t), Type: "pure-token", Token: t} }
	return redis.Docs{
		"set": {Args: []redis.Arg{
			{Name: "key", Type: "key"}, {Name: "value", Type: "string"},
			{Name: "condition", Type: "oneof", Optional: true, Args: []redis.Arg{tok("NX"), tok("XX")}},
			{Name: "seconds", Type: "integer", Token: "EX", Optional: true},
		}},
		"get":    {Summary: "Returns the string value of a key.", Args: []redis.Arg{{Name: "key", Type: "key"}}},
		"foo":    {Args: []redis.Arg{{Name: "key", Type: "key"}, {Name: "field", Type: "string"}}}, // a field of something else
		"hget":   {Args: []redis.Arg{{Name: "key", Type: "key"}, {Name: "field", Type: "string"}}},
		"config": {Subs: map[string]redis.Doc{"set": {Args: []redis.Arg{{Name: "parameter", Type: "string"}, {Name: "value", Type: "string"}}}}},
	}
}

// The signature of a Redis command is the line of its arguments with the one the
// cursor is at lit: by the words before it, a subcommand's by its own; none for a
// command the server has no docs of, or before the name is done (F16.5).
func TestRedisSignature(t *testing.T) {
	a, c := redisConsole(t, &execDB{}, someCommands(), "")
	a.sess.Conns[0].docs = someDocs()
	a.valueHints = true
	for _, tc := range []struct {
		typed  string
		fields string
		cur    int
	}{
		{"SET ", "SET key value [NX | XX] [EX seconds]", 1},
		{"set k", "SET key value [NX | XX] [EX seconds]", 1},
		{"set k ", "SET key value [NX | XX] [EX seconds]", 2},
		{"set k v ", "SET key value [NX | XX] [EX seconds]", 3},
		{"set k v nx ", "SET key value [NX | XX] [EX seconds]", 4},
		{"set k v nx ex 10 ", "SET key value [NX | XX] [EX seconds]", 0},
		{"config set ", "CONFIG SET parameter value", 1},
		{"config set maxmemory ", "CONFIG SET parameter value", 2},
		{"config ", "", 0}, {"set", "", 0}, {"nosuch k ", "", 0}, {"", "", 0}, {"# set k ", "", 0},
	} {
		c.ed.Load(tc.typed)
		feed(t, a, "A") // INSERT at the end
		s := a.signature(c)
		if got := strings.Join(s.Fields, " "); got != tc.fields || tc.fields != "" && (s.Cur != tc.cur || s.Sep != " ") {
			t.Errorf("%q: %q cur %d, want %q %d", tc.typed, got, s.Cur, tc.fields, tc.cur)
		}
		feed(t, a, "<Esc>")
	}
	c.ed.Load("set k v nx")
	feed(t, a, "0llllli") // the cursor after "set k": only what is before it counts
	if s := a.signature(c); s.Cur != 1 {
		t.Errorf("in the middle of a line: %d", s.Cur)
	}
	feed(t, a, "<Esc>")
	a.valueHints = false
	c.ed.Load("set k ")
	if len(a.signature(c).Fields) != 0 {
		t.Error("hints are off")
	}
}

// typedAt types text in INSERT at the end of a console's line and gives the labels
// of the candidates the list has.
func typedAt(t *testing.T, a *App, c *queryTab, text string, manual bool) (labels []string) {
	t.Helper()
	c.ed.Load(text)
	feed(t, a, "A")
	a.queryComplete(c, manual)
	if c.comp != nil {
		for _, it := range c.comp.items {
			labels = append(labels, it.label)
		}
	}
	feed(t, a, "<Esc>")
	return labels
}

// The console completes the name of a command, a subcommand, the tokens of an
// argument, a key and a hash's field from what the server gave: a list opens when a
// word is typed, or for C-n; not in a quote or a comment; the keys come from the
// server when the typing stops, and the list is made again with them (F16.5).
func TestRedisCompletion(t *testing.T) {
	a, c := redisConsole(t, &execDB{}, someCommands(), "")
	conn := a.sess.Conns[0]
	conn.docs, conn.hints = someDocs(), map[string][]string{"keys\x00us": {"user:1", "user:two words"}, "fields\x00h": {"field1", "other", "Field2"}}
	for _, tc := range []struct {
		typed  string
		manual bool
		want   string
	}{
		{"se", false, "SET"}, {"GE", false, "GET"}, {"", false, ""}, {"", true, "CONFIG FOO GET HGET SET"},
		{"set k v ", true, "NX XX EX"}, {"set k v e", false, "EX"}, {"config s", false, "SET"}, {"config ", true, "SET"},
		{"get us", false, "user:1 user:two words"}, {"hget h fi", false, "field1 Field2"}, {"foo h f", false, ""}, {"hget h ", true, "field1 other Field2"},
		{`get "us`, false, ""}, {"# se", false, ""}, {"# ", true, ""}, {"nosuch k ", true, ""},
	} {
		got := typedAt(t, a, c, tc.typed, tc.manual)
		if strings.Join(got, " ") != tc.want {
			t.Errorf("%q manual %v: %q, want %q", tc.typed, tc.manual, got, tc.want)
		}
	}
	// the word is replaced, and a key with a blank in it is quoted
	c.ed.Load("get us")
	feed(t, a, "A")
	a.queryComplete(c, false)
	c.comp.sel = 1
	a.queryAccept(c)
	if got := strings.Join(c.ed.Lines(), "\n"); got != `get "user:two words" ` { // a blank after it: the next word comes
		t.Errorf("accepted: %q", got)
	}
	feed(t, a, "<Esc>")
	c.ed.Load("get a\n  get us") // on a line that is not the first
	feed(t, a, "GA")
	a.queryComplete(c, false)
	c.comp.sel = 0
	a.queryAccept(c)
	if got := strings.Join(c.ed.Lines(), "\n"); got != "get a\n  get user:1 " {
		t.Errorf("accepted on the second line: %q", got)
	}
	feed(t, a, "<Esc>")
	// a key that is not in yet is asked for after the typing stops, the last word typed only
	seq := c.hintSeq
	typedAt(t, a, c, "get zz", false)
	if c.hintSeq != seq+1 {
		t.Errorf("not asked: seq %d, was %d", c.hintSeq, seq)
	}
	if a.gotHintDue(hintDue{c, seq, "keys", "zz"}) != nil {
		t.Error("an older word asks")
	}
	typedAt(t, a, c, "get us", false)   // what is known is not asked again
	typedAt(t, a, c, `get "zz"`, false) // nor what is quoted
	typedAt(t, a, c, "foo h f", false)  // nor the fields of what is no hash
	if c.hintSeq != seq+1 {
		t.Errorf("asked of what is known: %d", c.hintSeq)
	}
}

// The signature line of a command with its current argument lit, and the box of the
// keys that complete the word (F16.5).
func TestGoldenRedisHints(t *testing.T) {
	a, c := redisConsole(t, &execDB{}, someCommands(), "")
	conn := a.sess.Conns[0]
	conn.docs, conn.hints = someDocs(), map[string][]string{"keys\x00us": {"user:1", "user:2"}}
	a.valueHints = true
	pane := func() string {
		r := a.layout()[a.focused().ID]
		var lines []string
		for _, l := range strings.Split(a.render().String(), "\n")[r.Min.Y : r.Min.Y+7] {
			lines = append(lines, ansi.Cut(l, r.Min.X, r.Max.X))
		}
		return strings.Join(lines, "\n")
	}
	c.ed.Load("set k ")
	feed(t, a, "A")
	t.Run("signature", func(t *testing.T) { golden.RequireEqual(t, pane()) })
	feed(t, a, "<Esc>")
	c.ed.Load("get us")
	feed(t, a, "A")
	a.queryComplete(c, false)
	t.Run("keys", func(t *testing.T) { golden.RequireEqual(t, pane()) })
	feed(t, a, "<Esc>")
}
