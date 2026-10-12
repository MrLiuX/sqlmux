package main

import (
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

// versionLine is what --version prints: "sqlmux v0.2.1 (3f2a1b9, 2026-10-11T00:06:40Z)".
// A build without a version of its own takes the module's that Go recorded in
// info, which is the tag of a `go install ...@v0.2.1`, and is dev with none.
func versionLine(info *debug.BuildInfo) string {
	v := version
	if v == "dev" && info != nil && info.Main.Version != "" && info.Main.Version != "(devel)" {
		v = info.Main.Version
	}
	return fmt.Sprintf("sqlmux %s (%s, %s)", v, commit, date)
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

// run is `sqlmux [--layout name-or-path] [connection | file | url]`. Whatever fails before
// the UI is up ends the program with the error on the terminal (§14「启动时找不到连接」).
func run(args []string) error {
	flags := flag.NewFlagSet("sqlmux", flag.ContinueOnError)
	layout := flags.String("layout", "", "open this layout (a name or a path)")
	showVersion := flags.Bool("version", false, "print the version and exit")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *showVersion {
		info, _ := debug.ReadBuildInfo()
		fmt.Println(versionLine(info))
		return nil
	}
	name := ""
	switch flags.NArg() {
	case 0:
	case 1:
		name = flags.Arg(0)
	default:
		return errors.New("usage: sqlmux [--version] [--layout name-or-path] [connection | file | url]")
	}
	if name != "" && *layout != "" {
		return errors.New("give a connection name or --layout, not both")
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	i18n.Set(cfg.Language) // from here on, what is shown follows it (§14)
	warning := startNotes(cfg)
	temp, err := tempConnection(name, *layout, os.Getenv("DATABASE_URL"))
	if err != nil {
		return err
	}
	var l *config.Layout
	var sess *app.Session
	var restored *app.Restored
	if temp != nil { // a file or a URL: its own connection and its own session, kept nowhere
		sess = app.OpenTemp(*temp)
	} else {
		if l, err = startLayout(name, *layout, &warning); err != nil {
			return err
		}
		if sess, restored, err = startSession(name, *layout, l); err != nil {
			return err
		}
	}
	st, err := config.LoadState()
	if err != nil { // the broken file is kept aside; start empty (§14)
		warning = app.JoinNotes(warning, err.Error())
	}
	keys, problems := keymap.New(cfg.Leader, cfg.Vim, cfg.Timeoutlen, cfg.Bindings)
	a := app.New(cfg, keys, sess, st, warning)
	if restored != nil {
		a.Adopt(restored)
	}
	if temp == nil {
		a.SaveLayoutOnExit(config.LastPath())
	}
	a.KeyProblems(problems)
	defer a.Close()
	p := tea.NewProgram(a)
	app.Send = p.Send
	_, err = p.Run()
	return err
}

// tempConnection is the connection to open that connections.toml has not
// (F14.1): the file or URL name is; with no name and no layout asked for,
// DATABASE_URL, but only when connections.toml has no connection, as many keep
// it exported all the time and a user with a config wants last.toml back; nil
// when there is none, or when name is a connection's. A connections.toml that
// does not read is the error, not a reason to take name for a file.
func tempConnection(name, layout, databaseURL string) (*config.Connection, error) {
	conns, _, err := config.LoadConnections("")
	if err != nil && !config.NoConnections(err) {
		return nil, err
	}
	var known []string
	for _, c := range conns {
		known = append(known, c.Name)
	}
	target := name
	switch {
	case name != "":
		if !config.IsTarget(name, known) {
			return nil, nil
		}
	case layout == "" && config.NoConnections(err):
		target = databaseURL
	}
	if target == "" {
		return nil, nil
	}
	c, err := config.TempConnection(target, known)
	return &c, err
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

// startNotes is what the start has to say that is no reason to stay out: a
// misspelt key (F13.5), the queries that were consoles before F10.11 not moved.
func startNotes(cfg *config.Config) string {
	warning := app.JoinNotes(config.UnknownKeys(cfg)...)
	for _, err := range []error{config.MigrateConsoles(), config.TrimHistory(cfg.HistoryMax)} {
		if err != nil {
			warning = app.JoinNotes(warning, err.Error())
		}
	}
	return warning
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
	*warning = app.JoinNotes(*warning, err.Error())
	return nil, config.BrokenLast()
}

// open is a session over connections.toml, started on connection name, or the
// first one when name is "" (§5); none is connected till the program runs.
func open(name string) (*app.Session, error) {
	conns, start, err := config.LoadConnections(name)
	if err != nil {
		return nil, err
	}
	return app.Open(conns, start), nil
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
	keys, problems := keymap.New(cfg.Leader, cfg.Vim, cfg.Timeoutlen, cfg.Bindings)
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
