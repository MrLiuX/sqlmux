package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"

	tea "charm.land/bubbletea/v2"

	"sqlmux/internal/app"
	"sqlmux/internal/config"
	"sqlmux/internal/i18n"
	"sqlmux/internal/keymap"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "keys" {
		os.Exit(keysCmd(os.Args[2:], os.Stdout, os.Stderr))
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
	if err := flags.Parse(args); err != nil {
		return err
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
	_, err = tea.NewProgram(a).Run()
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
