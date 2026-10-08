package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"sqlmux/internal/config"
	"sqlmux/internal/db"
)

var errLost = errors.New("broken pipe")

// redialed makes a's Main dial again, to d2, a connection lost with errLost
// after what it sent.
func redialed(a *App, d2 *execDB) {
	w := a.sess.Conns[0].Main
	w.Redial = func(context.Context) (db.Conn, error) { return d2, nil }
	w.Broken = func(err error) db.Lost {
		if errors.Is(err, errLost) {
			return db.LostSent
		}
		return db.NotLost
	}
	w.Notify = func(e db.Event) { Send(connEvent{a.sess.Conns[0], false, e}) }
}

// heard is what the Workers tell the program while the test runs (Send).
func heard(t *testing.T) *[]connEvent {
	t.Helper()
	var evs []connEvent
	was := Send
	Send = func(m tea.Msg) { evs = append(evs, m.(connEvent)) }
	t.Cleanup(func() { Send = was })
	return &evs
}

// A run that only reads goes again on the connection dialed anew, which
// has its own search_path: the console's schema is set on it before; one
// that may write is not run again, its error the one it had, and the
// toast says to run it (F8.19).
func TestRunAfterLostConnection(t *testing.T) {
	evs := heard(t)
	d := &execDB{fail: map[string]error{"select 1": errLost, "update t set a = 1": errLost}}
	a, c := inRun(t, d, "select 1")
	d2 := &execDB{res: map[string]db.Result{"select 1": rows(1)}}
	redialed(a, d2)
	a.sess.Conns[0].startedPath = `"$user", public`
	c.schema = "agentable"
	press(t, a, "<CR>")
	if strings.Join(d2.ran, "; ") != `search_path "agentable" "$user", public; select 1`+"\nLIMIT 1001" || strings.Contains(logText(a), "broken pipe") {
		t.Fatalf("on the new connection: %q, log %q", d2.ran, logText(a))
	}
	if len(*evs) != 1 || !(*evs)[0].Rerun || (*evs)[0].Err != nil {
		t.Fatalf("events %+v", *evs)
	}
	// the new connection lost as well, and the write is not run again
	d2.fail = map[string]error{"update t set a = 1": errLost}
	d3 := &execDB{}
	redialed(a, d3)
	a.consoleDid(c, c.ed.Load("update t set a = 1"))
	press(t, a, "<CR>")
	if len(d3.ran) != 0 || !strings.Contains(logText(a), "broken pipe") {
		t.Fatalf("a write ran again: %q, log %q", d3.ran, logText(a))
	}
	last := (*evs)[len(*evs)-1]
	if last.Rerun {
		t.Fatalf("events %+v", *evs)
	}
	a.Update(last)
	if a.toast != "doraemon：已重连，请重新执行" {
		t.Errorf("toast %q", a.toast)
	}
}

// A redial that fails turns the connection's node red, a toast naming why,
// till one goes through; the connection stays connected for that try
// (F8.19).
func TestRedialFailureShows(t *testing.T) {
	a, _ := withConns()
	c := a.sess.Conns[0]
	red := func() bool {
		ns, _ := a.treeNodes()
		for _, n := range ns {
			if n.kind == nodeConn && n.conn == c {
				return n.TextFg == a.theme.Error
			}
		}
		t.Fatal("no node")
		return false
	}
	if red() {
		t.Fatal("red before")
	}
	a.Update(connEvent{c, true, db.Event{Err: errors.New("connection refused")}})
	if !red() || c.State != connReady || a.toast != "doraemon：connection refused" {
		t.Fatalf("failed: red %v, %v, toast %q", red(), c.State, a.toast)
	}
	a.Update(connEvent{c, false, db.Event{Rerun: true}}) // Main's: Meta's is still down
	if !red() {
		t.Error("Main's redial cleared Meta's")
	}
	a.Update(connEvent{c, true, db.Event{Rerun: true}})
	if red() {
		t.Error("still red after a redial went through")
	}
}

// A transaction lost with the connection: the status goes back to out of
// one, the mode stays, the user is told; a redial that ran the request
// again says nothing (F8.19).
func TestRedialTransactionLost(t *testing.T) {
	a, _ := withConns()
	c := a.sess.Conns[0]
	c.Manual, c.tx = true, 'T'
	a.Update(connEvent{c, false, db.Event{Rerun: true}})
	if c.tx != 'T' || a.toast != "" {
		t.Fatalf("a quiet redial: %c, toast %q", c.tx, a.toast)
	}
	a.Update(connEvent{c, false, db.Event{TxLost: true, Rerun: true}})
	if c.tx != 'I' || !c.Manual || a.toast != "doraemon：连接断开，事务已丢失" {
		t.Errorf("lost: %c, manual %v, toast %q", c.tx, c.Manual, a.toast)
	}
}

// A redial asks for the password as the first dial did, not from what the
// first one cached: password_cmd may give a token that has changed
// (F8.19).
func TestRedialAsksForThePasswordAgain(t *testing.T) {
	runs := filepath.Join(t.TempDir(), "runs")
	spec := config.Connection{Name: "redial-secret", Engine: "postgres", DSN: "postgres://u@127.0.0.1:1/x?connect_timeout=1", PasswordCmd: "echo >> " + runs + "; printf pw"}
	secrets.Store(spec.Name, "cached")
	t.Cleanup(func() { secrets.Delete(spec.Name) })
	c := newConn(spec)
	w := c.worker(noDB{}, spec, false, false)
	if _, err := w.Redial(context.Background()); err == nil {
		t.Fatal("nothing listens on port 1")
	}
	if b, _ := os.ReadFile(runs); strings.Count(string(b), "\n") != 1 {
		t.Errorf("password_cmd ran %q", b)
	}
}
