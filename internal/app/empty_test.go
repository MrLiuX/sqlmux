package app

import (
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

// With no connection every command is safe to run, from the tree and from
// each pane, and the screen still draws: a command that needs a connection
// says so (F7.4).
func TestNoConnectionRunsEverything(t *testing.T) {
	for id := range actions {
		for _, focus := range []int{0, 1, 2} {
			a := emptyApp(t)
			a.form = nil
			a.win().focus(focus)
			func() {
				defer func() {
					if r := recover(); r != nil {
						t.Errorf("%s from pane %d: %v", id, focus, r)
					}
				}()
				a.run(id, 0)
				a.View()
			}()
		}
	}
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

// The start opens the form over a tree of one line saying what to do; esc
// closes it, the line and the ＋ in the sidebar's first row open it again,
// and saving the first connection connects it, names the session by it and
// opens console_1 beside the empty pane (F7.4).
func TestNoConnectionStart(t *testing.T) {
	keychain(t, "none")
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
	sb := a.sidebarRect()
	click(a, uv.Pos(sb.Max.X-3, sb.Min.Y+1)) // the ＋, by its place
	if a.form == nil {
		t.Error("the ＋ does not open the form")
	}
	feed(t, a, "<Esc>")
	click(a, uv.Pos(a.sidebarRect().Min.X+3, a.sidebarRect().Min.Y+3)) // the empty tree's line
	if a.form == nil {
		t.Error("the line does not open the form")
	}
	feed(t, a, "play<Tab><Tab>h<Tab><Tab>u<Tab><Tab>d<CR>")
	conn := a.sess.start
	if a.form != nil || conn == nil || a.sess.Name != "play" || a.conn() != conn || conn.State != connConnecting {
		t.Fatalf("saved: form %v, start %v, session %q", a.form, conn, a.sess.Name)
	}
	if c := consoleOf(a.win().pane(2)); c == nil || c.conn != conn || c.name != "console_1" {
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

// The palette's quick SQL needs a connection: typed, run, edited or sent to
// the result area with none, it says so (F7.4).
func TestNoConnectionQuickSQL(t *testing.T) {
	for _, keys := range []string{"<C-p>;", "<C-p>;select 1", "<C-p>;select 1<CR>", "<C-p>;select 1<C-e>", "<C-p>;select 1<C-t>", "<C-p>;select 1<Tab>"} {
		a := emptyApp(t)
		a.form = nil
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("%s: %v", keys, r)
				}
			}()
			feed(t, a, keys)
			a.View()
		}()
	}
}
