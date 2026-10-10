package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	_ "net/http/pprof" // registers on DefaultServeMux: only served when SQLMUX_PPROF is set
	"os"
	"runtime/debug"

	tea "charm.land/bubbletea/v2"

	"github.com/kackerx/sqlmux/internal/app"
	"github.com/kackerx/sqlmux/internal/config"
	"github.com/kackerx/sqlmux/internal/i18n"
	"github.com/kackerx/sqlmux/internal/keymap"
)

// version, commit and date are what the build says (ldflags -X, F11.1);
// a build without them, `go install`'s, asks what Go recorded.
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

// versionLine is what --version prints: "sqlmux v0.2.0 (3f2a1b9, 2026-10-11)".
func versionLine() string {
	v, c, d := version, commit, date
	if info, ok := debug.ReadBuildInfo(); ok && v == "dev" {
		if mv := info.Main.Version; mv != "" && mv != "(devel)" {
			v = mv // go install ...@v0.2.0
		}
		for _, s := range info.Settings {
			switch {
			case s.Key == "vcs.revision" && c == "none":
				c = s.Value[:min(len(s.Value), 7)]
			case s.Key == "vcs.time" && d == "unknown":
				d = s.Value
			}
		}
	}
	return fmt.Sprintf("sqlmux %s (%s, %s)", v, c, d)
}

func main() {
	if len(os.Args) > 1 && os.Args[1] == "keys" {
		os.Exit(keysCmd(os.Args[2:], os.Stdout, os.Stderr))
	}
	if addr := os.Getenv("SQLMUX_PPROF"); addr != "" { // §15「性能」: unset, nothing listens
		ln, err := net.Listen("tcp", addr) // here, so a taken port or a bad address is said before the UI takes the terminal
		if err != nil {
			fmt.Fprintln(os.Stderr, "sqlmux: SQLMUX_PPROF:", err)
			os.Exit(1)
		}
		go func() { _ = http.Serve(ln, nil) }()
	}
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "sqlmux:", err)
		os.Exit(1)
	}
}

// run is `sqlmux [--layout name-or-path] [connection]`. Whatever fails before
// the UI is up ends the program with the error on the terminal (§14「启动时找不到连接」).
func run(args []string) error {
	flags := flag.NewFlagSet("sqlmux", flag.ContinueOnError)
	layout := flags.String("layout", "", "open this layout (a name or a path)")
	showVersion := flags.Bool("version", false, "print the version and exit")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *showVersion {
		fmt.Println(versionLine())
		return nil
	}
	name := ""
	switch flags.NArg() {
	case 0:
	case 1:
		name = flags.Arg(0)
	default:
		return errors.New("usage: sqlmux [--layout name-or-path] [connection]")
	}
	if name != "" && *layout != "" {
		return errors.New("give a connection name or --layout, not both")
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	i18n.Set(cfg.Language) // from here on, what is shown follows it (§14)
	var warning string
	if err := config.MigrateConsoles(); err != nil { // the queries were consoles before F10.11
		warning = err.Error()
	}
	l, err := startLayout(name, *layout, &warning)
	if err != nil {
		return err
	}
	sess, restored, err := startSession(name, *layout, l)
	if err != nil {
		return err
	}
	st, err := config.LoadState()
	if err != nil { // the broken file is kept aside; start empty (§14)
		warning = app.JoinNotes(warning, err.Error())
	}
	keys, problems := keymap.New(cfg)
	a := app.New(cfg, keys, sess, st, warning)
	if restored != nil {
		a.Adopt(restored)
	}
	a.SaveLayoutOnExit(config.LastPath())
	a.KeyProblems(problems)
	defer a.Close()
	p := tea.NewProgram(a)
	app.Send = p.Send
	_, err = p.Run()
	return err
}

// startSession is the session the program starts with: over the layout l, the
// default one over connection name, or, with no connections.toml or none in
// it and none asked for, an empty one with the form open (§14「启动时找不到
// 连接」).
func startSession(name, layout string, l *config.Layout) (*app.Session, *app.Restored, error) {
	if l != nil {
		conns, _, err := config.LoadConnections("")
		switch {
		case config.NoConnections(err) && layout == "": // last.toml of connections gone: none to restore
			return app.OpenEmpty(), nil, nil
		case err != nil:
			return nil, nil, err
		}
		restored := app.Restore(conns, l)
		if err := restored.DialStart(context.Background()); err != nil {
			return nil, nil, err
		}
		return restored.Sessions[0], restored, nil
	}
	sess, err := open(name)
	switch {
	case config.NoConnections(err) && name == "":
		return app.OpenEmpty(), nil, nil
	case err != nil:
		return nil, nil, err
	}
	return sess, nil, nil
}

// startLayout is the layout the program starts from, nil for the default
// one (§14「启动」): --layout's, which has to be there; else last.toml when no
// connection is named, which is kept aside, said in warning, when it does
// not read.
func startLayout(name, layout string, warning *string) (*config.Layout, error) {
	switch {
	case layout != "":
		return config.LoadLayout(config.LayoutArg(layout))
	case name != "":
		return nil, nil
	}
	l, err := config.LoadLayout(config.LastPath())
	switch {
	case err == nil:
		return l, nil
	case errors.Is(err, fs.ErrNotExist):
		return nil, nil
	}
	*warning = err.Error()
	return nil, config.BrokenLast()
}

// open is a session over connections.toml, connected to connection
// name, or the first one when name is "" (§5).
func open(name string) (*app.Session, error) {
	conns, start, err := config.LoadConnections(name)
	if err != nil {
		return nil, err
	}
	sess, err := app.Open(context.Background(), conns, start)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", conns[start].Name, err)
	}
	return sess, nil
}

// keysCmd is `sqlmux keys [--format md|toml] [--check]` (§6.7).
func keysCmd(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("sqlmux keys", flag.ContinueOnError)
	fs.SetOutput(stderr)
	format := fs.String("format", "md", "output format: md | toml")
	check := fs.Bool("check", false, "only check for conflicts; exit 1 when there are some")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintln(stderr, "sqlmux:", err)
		return 1
	}
	i18n.Set(cfg.Language)
	keys, problems := keymap.New(cfg)
	for _, p := range problems {
		fmt.Fprintln(stderr, p)
	}
	if *check {
		if len(problems) > 0 {
			return 1
		}
		return 0
	}
	switch *format {
	case "md":
		fmt.Fprint(stdout, keys.Markdown())
	case "toml":
		fmt.Fprint(stdout, keys.TOML(app.Titles()))
	default:
		fmt.Fprintf(stderr, "sqlmux keys: unsupported format %q\n", *format)
		return 2
	}
	return 0
}
