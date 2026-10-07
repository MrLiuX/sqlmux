package app

import (
	"fmt"
	"image/color"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/charmbracelet/x/exp/golden"

	"sqlmux/internal/config"
	"sqlmux/internal/keymap"
	"sqlmux/internal/ui"
)

const managerFile = `# mine
[[connection]]
name = "doraemon"
engine = "postgres"
dsn = "postgres://u@h:5433/db?sslmode=disable"

# the second
[[connection]]
name = "second"
engine = "mysql"
dsn = "u:p@tcp(m:3306)/s"
read_only = true
`

// managed is an App over two connections of a connections.toml of its own:
// doraemon, connected and with the console on it, and second.
func managed(t *testing.T) *App { return managedWith(t, "ok") }

// managedWith is managed over the fake keychain of mode (keychain): PATH
// has no real tool for a test to reach.
func managedWith(t *testing.T, mode string) *App {
	t.Helper()
	keychain(t, mode)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	if err := os.MkdirAll(config.Dir(), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(config.Dir(), "connections.toml"), []byte(managerFile), 0o644); err != nil {
		t.Fatal(err)
	}
	a := sized(160, 45, "nerd")
	a.sess.Conns[0].Spec = config.Connection{Name: "doraemon", Engine: "postgres", DSN: "postgres://u@h:5433/db?sslmode=disable"}
	a.sess.Conns = append(a.sess.Conns, newConn(config.Connection{Name: "second", Engine: "mysql", DSN: "u:p@tcp(m:3306)/s", ReadOnly: true}))
	return a
}

func fieldText(a *App, i int) string { return a.form.fields[i].Input.Text }

// conn.manage opens the list on the connection in use with the keys; the
// form is filled from the one selected, its dsn split for the eye and
// dim; j and k move, Tab goes to the form, esc back and then out (F8.16).
func TestConnManagerKeys(t *testing.T) {
	a := managed(t)
	a.run("conn.manage", 0)
	if a.form == nil || !a.form.list || a.form.cur != 0 || a.mode() != keymap.Command || a.context().Overlay != "conns" {
		t.Fatalf("not open on the list: %+v", a.form)
	}
	if fieldText(a, fieldName) != "doraemon" || fieldText(a, fieldHost) != "h" || fieldText(a, fieldPort) != "5433" || fieldText(a, fieldUser) != "u" || fieldText(a, fieldDatabase) != "db" || !strings.HasPrefix(fieldText(a, fieldDSN), "postgres://u@h") {
		t.Errorf("the form of doraemon: %q %q %q %q %q", fieldText(a, fieldName), fieldText(a, fieldHost), fieldText(a, fieldPort), fieldText(a, fieldUser), fieldText(a, fieldDatabase))
	}
	if fs := a.formView().Fields; !fs[fieldHost].Dim || fs[fieldDSN].Hidden {
		t.Error("the parts of the dsn are dim, the dsn shown")
	}
	feed(t, a, "j")
	if a.form.cur != 1 || fieldText(a, fieldName) != "second" || fieldText(a, fieldHost) != "m" || fieldText(a, fieldDatabase) != "s" || a.form.fields[fieldReadOnly].On != 1 || a.form.fields[fieldEngine].On != 1 {
		t.Errorf("second: cur %d %q %q", a.form.cur, fieldText(a, fieldName), fieldText(a, fieldHost))
	}
	feed(t, a, "j")
	if a.form.cur != 2 || a.form.was != nil || fieldText(a, fieldName) != "" {
		t.Errorf("the row that makes a new one: cur %d, was %v", a.form.cur, a.form.was != nil)
	}
	feed(t, a, "jkk<Tab>")
	if a.form.cur != 0 || a.form.list || a.mode() != keymap.Insert {
		t.Fatalf("Tab: cur %d list %v", a.form.cur, a.form.list)
	}
	feed(t, a, "<Esc>")
	if a.form == nil || !a.form.list {
		t.Fatal("esc in the form of a connection goes back to the list")
	}
	feed(t, a, "<Esc>")
	if a.form != nil {
		t.Error("esc in the list closes it")
	}
	a.run("conn.new", 0)
	if a.form.cur != 2 || a.form.list || a.form.was != nil {
		t.Errorf("conn.new: on the new row, the form with the keys")
	}
	feed(t, a, "<Esc>")
	if a.form != nil {
		t.Error("esc in a new connection's form closes it")
	}
}

// A click on a row of the list selects it and gives the list the keys; one
// on a field gives them back to the form (F8.16).
func TestConnManagerClicks(t *testing.T) {
	a := managed(t)
	a.run("conn.manage", 0)
	click(a, find(t, a, ui.Target{Kind: ui.KindConn, I: 1}).Min)
	if a.form.cur != 1 || !a.form.list || fieldText(a, fieldName) != "second" {
		t.Fatalf("a row: cur %d list %v", a.form.cur, a.form.list)
	}
	click(a, find(t, a, ui.Target{Kind: ui.KindRow, I: fieldName}).Min)
	if a.form.list {
		t.Error("a field: the form has the keys")
	}
}

// Saving the form of a connection rewrites its lines in connections.toml,
// and nothing else of the file; the sessions' connections take it; a name
// carries the state; one connected is told it takes effect when it connects
// next (F8.16).
func TestConnManagerEdit(t *testing.T) {
	a := managed(t)
	a.run("conn.manage", 0)
	feed(t, a, "j<Tab>")
	a.form.fields[fieldDSN].Input = ui.Input{Text: "u:p@tcp(other:3307)/s"}
	a.form.fields[fieldReadOnly].On = 0
	feed(t, a, "<C-s>")
	want := strings.Replace(managerFile, "tcp(m:3306)", "tcp(other:3307)", 1)
	want = strings.Replace(want, "read_only = true\n", "", 1)
	if got := connections(); got != want {
		t.Fatalf("the file:\n%s", got)
	}
	if c := a.sess.Conns[1]; c.Spec.DSN != "u:p@tcp(other:3307)/s" || c.Spec.ReadOnly || a.form.was.DSN != c.Spec.DSN || fieldText(a, fieldHost) != "other" {
		t.Errorf("the session's connection and the form: %+v", c.Spec)
	}
	if a.toast != "" {
		t.Errorf("second is not connected: toast %q", a.toast)
	}

	a.state.Tables = map[string]*config.TableState{"doraemon/public.t": {}}
	a.state.Consoles = map[string]config.ConsoleState{"console_1": {Conn: "doraemon"}}
	feed(t, a, "<Esc>k<Tab><C-u>pg<C-s>")
	if got := connections(); !strings.Contains(got, `name = "pg"`) || strings.Contains(got, `name = "doraemon"`) {
		t.Fatalf("a rename:\n%s", got)
	}
	if a.sess.Conns[0].Spec.Name != "pg" || a.state.Tables["pg/public.t"] == nil || a.state.Consoles["console_1"].Conn != "pg" {
		t.Errorf("the state follows: %v %v", a.state.Tables, a.state.Consoles)
	}
	if a.toast != "下次连接时生效" {
		t.Errorf("connected: toast %q", a.toast)
	}

	feed(t, a, "<Esc><Esc>")
	a.run("conn.manage", 0)
	feed(t, a, "<Tab><C-u>second<C-s>") // a name another has
	if a.form.err == "" || !a.form.fields[fieldName].Bad {
		t.Errorf("a name taken: %q", a.form.err)
	}
}

// x on the list, or the form's delete, asks, then takes the connection out of
// the file with its comment and out of every session; one with tabs is
// refused (F8.16).
func TestConnManagerDelete(t *testing.T) {
	a := managed(t)
	a.run("conn.manage", 0)
	feed(t, a, "x")
	if a.confirm != nil || a.toast != "先关掉用它的 tab" || len(a.sess.Conns) != 2 {
		t.Fatalf("doraemon has the console: confirm %v, toast %q", a.confirm != nil, a.toast)
	}
	feed(t, a, "jx")
	if a.confirm == nil || len(a.sess.Conns) != 2 {
		t.Fatal("not asked")
	}
	feed(t, a, "n")
	if len(a.sess.Conns) != 2 || strings.Contains(connections(), "# the second") == false {
		t.Fatal("no: nothing went")
	}
	feed(t, a, "x")
	feed(t, a, "y")
	if got := connections(); strings.Contains(got, "second") || strings.Contains(got, "# the second") || !strings.HasPrefix(got, "# mine\n[[connection]]\nname = \"doraemon\"") {
		t.Errorf("the file:\n%s", got)
	}
	if len(a.sess.Conns) != 1 || a.form == nil || !a.form.list || a.form.cur != 1 || a.form.was != nil {
		t.Errorf("after: %d conns, selected %d", len(a.sess.Conns), a.form.cur)
	}
}

// The manager as it shows: the list, the form of the connection selected
// with its dsn and a delete button (F8.16).
func TestGoldenConnManager(t *testing.T) {
	a := managed(t)
	a.run("conn.manage", 0)
	golden.RequireEqual(t, a.render().String())
}

// withOwnPassword makes the password of second an item of the fake keychain,
// as the form does: the command in the file and in the session.
func withOwnPassword(t *testing.T, a *App, pw string) {
	t.Helper()
	cmd, stored, err := config.StorePassword("second", pw)
	if err != nil || !stored {
		t.Fatalf("stored %v, %v", stored, err)
	}
	was, err := config.ReadConnection("second")
	if err != nil {
		t.Fatal(err)
	}
	c := was
	c.PasswordCmd = cmd
	if _, err := config.UpdateConnection("second", was, c); err != nil {
		t.Fatal(err)
	}
	a.sess.Conns[1].setSpec(c)
}

// kcCalled is the lines of the fake keychain's log that start with one of
// verbs, in the order they came.
func kcCalled(verbs ...string) (calls []string) {
	for _, l := range kcLog() {
		for _, v := range verbs {
			if strings.HasPrefix(l, v) {
				calls = append(calls, l)
			}
		}
	}
	return calls
}

func hasHit(a *App, want ui.Target) bool {
	a.View()
	return slices.ContainsFunc(a.hits, func(h ui.Hit) bool { return h.Target == want })
}

// A connection renamed takes its keychain item with it: the password is read
// with the old command, put under the new name, the file written to say so,
// and only then the old item is taken out; the cached password of both names
// is dropped (F8.16).
func TestConnManagerRenameMovesPassword(t *testing.T) {
	a := managed(t)
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	withOwnPassword(t, a, "pw1")
	_ = os.Remove(filepath.Join(os.Getenv("FAKE"), "log")) // the setup's calls
	a.state.Tables = map[string]*config.TableState{"second/public.t": {}}
	secrets.Store("second", "cached")
	secrets.Store("third", "cached")
	a.run("conn.manage", 0)
	feed(t, a, "j<Tab><C-u>third<C-s>")
	if pw, ok := kcItem("third"); !ok || pw != "pw1" {
		t.Errorf("the item of the new name: %q %v", pw, ok)
	}
	if _, ok := kcItem("second"); ok {
		t.Error("the old item is still there")
	}
	c := a.sess.Conns[1].Spec
	if got, err := c.Secret(); c.PasswordCmd != config.KeychainCmd("third", "pw1") || err != nil || got != "pw1" || !strings.Contains(connections(), config.KeychainCmd("third", "")[:30]) {
		t.Errorf("the connection reads %q, %v: %+v", got, err, c)
	}
	add, del := kcCalled("add-generic-password", "store"), kcCalled("delete-generic-password", "clear")
	if len(add) != 1 || len(del) != 1 {
		t.Fatalf("calls: %q %q", add, del)
	}
	if i, j := slices.Index(kcLog(), add[0]), slices.Index(kcLog(), del[0]); i > j {
		t.Errorf("the old item went before the new was kept: %d, %d", i, j)
	}
	if _, ok := secrets.Load("second"); ok {
		t.Error("the old name's cached password")
	}
	if _, ok := secrets.Load("third"); ok {
		t.Error("the new name's cached password")
	}
	if st, err := config.LoadState(); err != nil || st.Tables["third/public.t"] == nil {
		t.Errorf("the state written: %v %v", st, err)
	}
}

// A password typed again under the same name replaces the item: it is not
// taken out after (F8.16).
func TestConnManagerRetypedPassword(t *testing.T) {
	a := managed(t)
	withOwnPassword(t, a, "pw1")
	a.run("conn.manage", 0)
	feed(t, a, "j<Tab>")
	a.form.fields[fieldPassword].Input = ui.Input{Text: "pw2", Pos: 3}
	feed(t, a, "<C-s>")
	if pw, ok := kcItem("second"); !ok || pw != "pw2" || a.form.err != "" {
		t.Errorf("the item: %q %v, %q", pw, ok, a.form.err)
	}
	if del := kcCalled("delete-generic-password", "clear"); len(del) != 0 {
		t.Errorf("deleted: %q", del)
	}
}

// Nothing goes into the keychain for an edit the file would not take: a name
// another connection has since been given is found out first, and a
// keychain that refuses leaves the file as it was (F8.16).
func TestConnManagerStoresOnlyWhatTheFileTakes(t *testing.T) {
	a := managed(t)
	a.run("conn.manage", 0)
	feed(t, a, "j<Tab>")
	external := managerFile + "\n[[connection]]\nname = \"third\"\nengine = \"postgres\"\ndsn = \"postgres://x/y\"\n"
	if err := os.WriteFile(filepath.Join(config.Dir(), "connections.toml"), []byte(external), 0o600); err != nil {
		t.Fatal(err)
	}
	a.form.fields[fieldName].Input = ui.Input{Text: "third", Pos: 5}
	a.form.fields[fieldPassword].Input = ui.Input{Text: "pw", Pos: 2}
	feed(t, a, "<C-s>")
	if a.form.err == "" || len(kcCalled("add-generic-password", "store")) != 0 || connections() != external {
		t.Errorf("err %q, stored %q, file changed %v", a.form.err, kcCalled("add-generic-password", "store"), connections() != external)
	}

	a = managedWith(t, "locked")
	a.run("conn.manage", 0)
	feed(t, a, "j<Tab>")
	a.form.fields[fieldPassword].Input = ui.Input{Text: "pw", Pos: 2}
	a.form.fields[fieldReadOnly].On = 0
	feed(t, a, "<C-s>")
	if a.form.err == "" || !a.form.fields[fieldPassword].Bad || connections() != managerFile {
		t.Errorf("a locked keychain: err %q, file changed %v", a.form.err, connections() != managerFile)
	}

	a = managed(t)
	a.run("conn.manage", 0)
	feed(t, a, "j<Tab>")
	a.form.fields[fieldPassword].Input = ui.Input{Text: "pw", Pos: 2}
	a.form.fields[fieldPasswordCmd].Input = ui.Input{Text: "pass show x"}
	feed(t, a, "<C-s>")
	if !a.form.fields[fieldPassword].Bad || connections() != managerFile || len(kcCalled("add-generic-password", "store")) != 0 {
		t.Errorf("two sources: err %q", a.form.err)
	}
}

// With no keychain tool the password goes into the file, which reads
// password_cmd first: the command and the variable it would name are
// cleared, or the password typed would never be used (F8.16).
func TestConnManagerPlainPasswordClearsOtherSources(t *testing.T) {
	a := managedWith(t, "none")
	was, _ := config.ReadConnection("second")
	c := was
	c.PasswordCmd, c.PasswordEnv = "pass show second", "SECOND_PW"
	if _, err := config.UpdateConnection("second", was, c); err != nil {
		t.Fatal(err)
	}
	a.run("conn.manage", 0)
	feed(t, a, "j<Tab>")
	a.form.fields[fieldPassword].Input = ui.Input{Text: "plain", Pos: 5}
	feed(t, a, "<C-s>")
	got, err := config.ReadConnection("second")
	if want := (config.Connection{Name: "second", Engine: "mysql", DSN: "u:p@tcp(m:3306)/s", ReadOnly: true, Password: "plain"}); err != nil || got != want {
		t.Errorf("the file says %+v, %v", got, err)
	}
	if !strings.Contains(a.toast, "明文") {
		t.Errorf("toast %q", a.toast)
	}
}

// A connection connected, or being, keeps the engine and the lock it was
// dialed with till it is dialed again, the rest of the spec being the new
// one; one that is not follows at once (F8.16).
func TestConnManagerRunningValuesWaitForTheNextDial(t *testing.T) {
	a := managed(t)
	doraemon, second := a.sess.Conns[0], a.sess.Conns[1]
	doraemon.State = connReady
	a.run("conn.manage", 0)
	a.form.fields[fieldReadOnly].On = 1
	feed(t, a, "<C-s>")
	if !doraemon.Spec.ReadOnly || doraemon.readOnly || a.toast != "下次连接时生效" {
		t.Errorf("connected: spec %v, lock %v, toast %q", doraemon.Spec.ReadOnly, doraemon.readOnly, a.toast)
	}
	if lock := a.managerView().Rows[0].Lock.Text; lock != "" {
		t.Errorf("the list shows the lock of the dial, not the file's: %q", lock)
	}
	doraemon.State = connFailed
	a.connect(doraemon)
	if !doraemon.readOnly {
		t.Error("dialed again: the lock")
	}

	feed(t, a, "j<Tab>")
	a.form.fields[fieldReadOnly].On = 0
	feed(t, a, "<C-s>")
	if second.Spec.ReadOnly || second.readOnly {
		t.Error("idle: the lock follows at once")
	}
	wasMySQL := second.eng
	second.State = connConnecting
	a.formSegment(fieldEngine, 0)
	feed(t, a, "<C-s>")
	if second.Spec.Engine != "postgres" || second.eng != wasMySQL || a.toast != "下次连接时生效" {
		t.Errorf("connecting: engine %q, eng changed %v, toast %q", second.Spec.Engine, second.eng != wasMySQL, a.toast)
	}
	second.State = connFailed
	a.form.fields[fieldReadOnly].On = 1
	feed(t, a, "<C-s>")
	if !second.readOnly {
		t.Error("failed: the lock follows at once")
	}
	second.State = connIdle
	a.formSegment(fieldEngine, 1)
	a.formSegment(fieldEngine, 0)
	feed(t, a, "<C-s>")
	if second.eng == wasMySQL {
		t.Error("idle: the engine follows at once")
	}
}

// The file is read where it is shown, and only what the form changed is
// written: a dsn edited meanwhile stays, and is what the connection takes
// (F8.16).
func TestConnManagerReadsTheFileAgain(t *testing.T) {
	a := managed(t)
	a.run("conn.manage", 0)
	edited := strings.Replace(managerFile, "tcp(m:3306)", "tcp(edited:3306)", 1)
	if err := os.WriteFile(filepath.Join(config.Dir(), "connections.toml"), []byte(edited), 0o600); err != nil {
		t.Fatal(err)
	}
	feed(t, a, "j")
	if fieldText(a, fieldHost) != "edited" {
		t.Errorf("selected: the form has %q", fieldText(a, fieldHost))
	}
	again := strings.Replace(edited, "tcp(edited:3306)", "tcp(again:3306)", 1)
	if err := os.WriteFile(filepath.Join(config.Dir(), "connections.toml"), []byte(again), 0o600); err != nil {
		t.Fatal(err)
	}
	feed(t, a, "<Tab>")
	a.form.fields[fieldReadOnly].On = 0
	feed(t, a, "<C-s>")
	if want := strings.Replace(again, "read_only = true\n", "", 1); connections() != want {
		t.Errorf("the file:\n%s", connections())
	}
	if a.sess.Conns[1].Spec.DSN != "u:p@tcp(again:3306)/s" || fieldText(a, fieldHost) != "again" {
		t.Errorf("what the connection took: %q", a.sess.Conns[1].Spec.DSN)
	}
}

// esc in the form of the new row, reached down the list, goes back to the
// list; ctrl-s on the list saves the form's changes, not the table behind;
// the keys under the list are the list's (F8.16).
func TestConnManagerListKeys(t *testing.T) {
	a := managed(t)
	a.run("conn.manage", 0)
	feed(t, a, "jj<Tab><Esc>")
	if a.form == nil || !a.form.list {
		t.Fatal("esc from the new row's form, entered from the list, closed the manager")
	}
	var keys []string
	for _, h := range a.formView().Hints {
		keys = append(keys, h.Key)
	}
	if want := []string{"Tab", "C-s", "esc"}; !slices.Equal(keys, want) {
		t.Errorf("the list's hints %v, want %v", keys, want)
	}
	a.form = nil
	a.run("conn.new", 0)
	click(a, find(t, a, ui.Target{Kind: ui.KindConn, I: 1}).Min)
	if feed(t, a, "<Tab><Esc>"); a.form == nil || !a.form.list {
		t.Fatal("a form of a new connection, then a row of the list chosen: esc closed the manager")
	}
	feed(t, a, "k<Tab>")
	a.form.fields[fieldReadOnly].On = 1
	feed(t, a, "<Esc><C-s>")
	if !strings.Contains(connections(), "name = \"doraemon\"\nengine = \"postgres\"\ndsn = \"postgres://u@h:5433/db?sslmode=disable\"\nread_only = true\n") {
		t.Errorf("not saved from the list:\n%s", connections())
	}
	for _, h := range a.formView().Hints {
		if h.Label == "保存" && h.Key != "C-s" {
			t.Errorf("save is shown as %q on the list", h.Key)
		}
	}
	feed(t, a, "<Tab>")
	for _, h := range a.formView().Hints {
		if h.Action == "form.save" && h.Key != "↵" {
			t.Errorf("save is shown as %q in the form", h.Key)
		}
	}
}

// The list takes the height of the form, not the connections': a long one
// scrolls with the selection, and every row is reached (F8.16).
func TestConnManagerLongList(t *testing.T) {
	a := managed(t)
	for i := range 30 {
		a.sess.Conns = append(a.sess.Conns, newConn(config.Connection{Name: fmt.Sprintf("extra_%d", i), Engine: "postgres", DSN: "postgres://h/x"}))
	}
	a.Update(tea.WindowSizeMsg{Width: 160, Height: 24})
	a.run("conn.manage", 0)
	if !hasHit(a, ui.Target{Kind: ui.KindConn, I: 0}) || hasHit(a, ui.Target{Kind: ui.KindConn, I: 25}) {
		t.Fatal("the first rows show, the last do not")
	}
	rows := a.managerView().ListRows()
	feed(t, a, strings.Repeat("j", 25))
	if !hasHit(a, ui.Target{Kind: ui.KindConn, I: 25}) || hasHit(a, ui.Target{Kind: ui.KindConn, I: 25 - rows}) {
		t.Errorf("scrolled to the selection: rows %d", rows)
	}
	feed(t, a, strings.Repeat("k", 3))
	if !hasHit(a, ui.Target{Kind: ui.KindConn, I: 22}) || !hasHit(a, ui.Target{Kind: ui.KindConn, I: 25}) {
		t.Error("moving up a little does not scroll")
	}
	feed(t, a, strings.Repeat("j", 40))
	if a.form.cur != 32 || fieldText(a, fieldName) != "" || !hasHit(a, ui.Target{Kind: ui.KindConn, I: 32}) {
		t.Errorf("the new row at the end: %d", a.form.cur)
	}
}

// What a connection deleted leaves behind goes with it: its keychain item
// (one not there is no error), the tabs closed on it, the session's way back
// to it, the password kept for the name (F8.16).
func TestConnManagerDeleteTakesWhatIsItsOwn(t *testing.T) {
	a := managed(t)
	withOwnPassword(t, a, "pw1")
	second := a.sess.Conns[1]
	a.sess.last = second
	a.sess.closed = []closedTab{{conn: second}, {conn: a.sess.Conns[0]}}
	secrets.Store("second", "cached")
	a.run("conn.manage", 0)
	feed(t, a, "xy") // the list opens on the connection in use, second
	if _, ok := kcItem("second"); ok || len(a.sess.Conns) != 1 {
		t.Errorf("the item is there %v, conns %d", ok, len(a.sess.Conns))
	}
	if len(a.sess.closed) != 1 || a.sess.closed[0].conn != a.sess.Conns[0] || a.sess.last != a.sess.start {
		t.Errorf("closed %d, last %v", len(a.sess.closed), a.sess.last == a.sess.start)
	}
	if _, ok := secrets.Load("second"); ok {
		t.Error("the cached password stays")
	}
}

// A connection is in use by a tab on it, a console that waits for it, the
// result of a run on it, or being what the session started with (F8.16).
func TestConnInUse(t *testing.T) {
	a := managed(t)
	second := a.sess.Conns[1]
	if a.connInUse("second") {
		t.Fatal("in use with nothing on it")
	}
	if a.sess.start = second; !a.connInUse("second") {
		t.Error("the start")
	}
	a.sess.start = a.sess.Conns[0]
	c := consoleOf(a.win().pane(2))
	if c.want = second; !a.connInUse("second") {
		t.Error("a console on its way")
	}
	c.want = nil
	tab := loadOrders(t, a, 3)
	if tab.conn = second; !a.connInUse("second") {
		t.Error("a table tab")
	}
	tab.conn = a.sess.Conns[0]
	p := a.showResult(a.win())
	if resultOf(p).run = (&run{conn: second}); !a.connInUse("second") {
		t.Error("a result")
	}
	resultOf(p).run = nil
	if a.connInUse("second") {
		t.Error("in use with the tabs gone from it")
	}
}

// conn.manage selects the connection in use, whichever row it is; the row
// selected has the list's color when the list has the keys (F8.16).
func TestConnManagerSelection(t *testing.T) {
	a := managed(t)
	a.sess.last = a.sess.Conns[1] // the one in use, with none connected
	a.run("conn.manage", 0)
	if a.form.cur != 1 || fieldText(a, fieldName) != "second" {
		t.Errorf("selected %d %q", a.form.cur, fieldText(a, fieldName))
	}
	f := a.render()
	at := func(i int) color.Color {
		r := find(t, a, ui.Target{Kind: ui.KindConn, I: i})
		return f.Buf.CellAt(r.Min.X+1, r.Min.Y).Style.Bg
	}
	if got := at(1); got != a.theme.Select {
		t.Errorf("the selected row %v, want %v", got, a.theme.Select)
	}
	if got := at(0); got != a.theme.PaneBg {
		t.Errorf("another row %v, want %v", got, a.theme.PaneBg)
	}
}

// A dsn is split for the eye as far as it is a URL, which is what the form
// writes for both engines, or the go-sql-driver's, with or without a port
// (F8.16).
func TestSplitDSN(t *testing.T) {
	for _, c := range []struct {
		engine, dsn            string
		host, port, user, base string
	}{
		{"postgres", "postgres://u@h:5433/db?sslmode=disable", "h", "5433", "u", "db"},
		{"mysql", "mysql://u@h:3306/db", "h", "3306", "u", "db"},
		{"mysql", "u:p@tcp(m:3307)/s?parseTime=true", "m", "3307", "u", "s"},
		{"mysql", "u@tcp(m)/s", "m", "", "u", "s"},
		{"mysql", "tcp(m:3306)/s", "m", "3306", "", "s"},
		{"mysql", "not a dsn", "", "", "", ""},
	} {
		if host, port, user, base := splitDSN(c.engine, c.dsn); host != c.host || port != c.port || user != c.user || base != c.base {
			t.Errorf("%s %q: %q %q %q %q", c.engine, c.dsn, host, port, user, base)
		}
	}
}
