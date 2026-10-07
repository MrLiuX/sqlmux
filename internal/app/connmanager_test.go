package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

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
func managed(t *testing.T) *App {
	t.Helper()
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
