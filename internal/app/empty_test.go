package app

import (
	"fmt"
	"os"
	"testing"

	tea "charm.land/bubbletea/v2"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/exp/golden"

	"sqlmux/internal/config"
	"sqlmux/internal/keymap"
	"sqlmux/internal/ui"
)

// emptyApp is the App of a start with no connection, on a 160 × 45 screen.
func emptyApp(t *testing.T) *App {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	c := config.Default()
	c.Icons = ui.IconSet("nerd")
	c.BaseIcons = c.Icons
	keys, _ := keymap.New(c)
	a := New(c, keys, OpenEmpty(), &config.State{}, "")
	a.Update(tea.WindowSizeMsg{Width: 160, Height: 45})
	return a
}

// panics is the commands that panic when run from each of the tree and the
// panes, with the message, as the connected App of the tests or the empty one.
func panics(t *testing.T, empty bool) map[string]string {
	t.Helper()
	out := map[string]string{}
	for id := range actions {
		for _, focus := range []int{0, 1, 2} {
			var a *App
			if empty {
				a = emptyApp(t)
				a.form = nil
			} else {
				a = sized(160, 45, "nerd")
			}
			a.win().focus(focus)
			func() {
				defer func() {
					if r := recover(); r != nil {
						out[fmt.Sprintf("%s from pane %d", id, focus)] = fmt.Sprint(r)
					}
				}()
				a.run(id, 0)
				a.View()
			}()
		}
	}
	return out
}

// Keys of every sort, typed in the tree and in each pane, do no harm either.
func TestNoConnectionKeys(t *testing.T) {
	keys := []string{"<CR>", "j", "k", "l", "h", "i", "<Esc>", "<C-p>", "<Space>", "gt", "o", "dd", "<Tab>", "/", "R", "c", "x", "<C-s>", "<C-c>", "<C-e>", "<C-w>", ":", "y", "p", "<C-j>", "<C-k>", "<C-l>", "<C-h>", "n", "t", "r", "?"}
	for _, focus := range []int{0, 1, 2} {
		a := emptyApp(t)
		a.Init()
		a.form = nil
		a.win().focus(focus)
		for _, k := range keys {
			func() {
				defer func() {
					if r := recover(); r != nil {
						t.Errorf("%s in pane %d: %v", k, focus, r)
					}
				}()
				a.Update(teaKey(keymap.Key(k)))
				a.View()
			}()
		}
	}
}

// With no connection every command is as safe to run as it is with one, from
// the tree and from each pane, and the screen still draws: a command that
// needs a connection says so (F7.4).
func TestNoConnectionRunsEverything(t *testing.T) {
	base := panics(t, false)
	for k, v := range panics(t, true) {
		if _, ok := base[k]; !ok {
			t.Errorf("%s: %s", k, v)
		}
	}
}

// The start opens the form over a tree of one line saying what to do; esc
// closes it, the line and the ＋ in the sidebar's first row open it again,
// and saving the first connection connects it, names the session by it and
// opens console_1 beside the empty pane (F7.4).
func TestNoConnectionStart(t *testing.T) {
	a := emptyApp(t)
	if a.form == nil || len(a.sess.Conns) != 0 || a.conn() != nil {
		t.Fatalf("form %v, conns %d", a.form, len(a.sess.Conns))
	}
	t.Run("form", func(t *testing.T) { golden.RequireEqual(t, a.render().String()) })
	feed(t, a, "<Esc>")
	if a.form != nil {
		t.Fatal("esc leaves the form open")
	}
	t.Run("tree", func(t *testing.T) { golden.RequireEqual(t, a.render().String()) })
	click(a, find(t, a, ui.Target{Kind: ui.KindButton, Pane: 0, Action: "conn.new"}).Min)
	if a.form == nil {
		t.Error("the ＋ does not open the form")
	}
	feed(t, a, "<Esc>")
	click(a, uv.Pos(a.sidebarRect().Min.X+3, a.sidebarRect().Min.Y+3))
	if a.form == nil {
		t.Error("the line does not open the form")
	}
	feed(t, a, "play<Tab><Tab>h<Tab><Tab>u<Tab><Tab>d<CR>")
	conn := a.sess.start
	if a.form != nil || conn == nil || a.sess.Name != "play" || a.conn() != conn || conn.State != connConnecting {
		t.Fatalf("saved: form %v, start %v, session %q", a.form, conn, a.sess.Name)
	}
	if c := consoleOf(a.win().pane(2)); c == nil || c.conn != conn || c.key() != "console_1" {
		t.Errorf("console_1: %+v", c)
	}
}

// There is nothing to save of a session with no connection: quitting leaves
// last.toml as it was, saving a layout says why not.
func TestNoConnectionSavesNothing(t *testing.T) {
	a := emptyApp(t)
	a.form = nil
	a.SaveLayoutOnExit(config.LastPath())
	a.saveLast()
	if _, err := os.Stat(config.LastPath()); err == nil {
		t.Error("last.toml written")
	}
	feed(t, a, "<C-p>layout.save<CR>work<CR>")
	if _, err := os.Stat(config.LayoutPath("work")); err == nil || a.toast != "还没有连接，先新建一个" {
		t.Errorf("layout.save: %v, toast %q", err, a.toast)
	}
}
