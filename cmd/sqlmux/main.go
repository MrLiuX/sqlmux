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
	layout := flags.String("layout", "", "打开这个布局（名字或路径）")
	if err := flags.Parse(args); err != nil {
		return err
	}
	name := ""
	switch flags.NArg() {
	case 0:
	case 1:
		name = flags.Arg(0)
	default:
		return errors.New("用法：sqlmux [--layout 名字或路径] [连接名]")
	}
	if name != "" && *layout != "" {
		return errors.New("连接名和 --layout 只能给一个")
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
	var sess *app.Session
	var restored *app.Restored
	if l != nil {
		conns, _, w, err := config.LoadConnections("")
		if err != nil {
			return err
		}
		warning = app.JoinNotes(warning, w)
		restored = app.Restore(conns, l)
		if err := restored.DialStart(context.Background()); err != nil {
			return err
		}
		sess = restored.Sessions[0]
	} else {
		var w string
		if sess, w, err = open(name); err != nil {
			return err
		}
		warning = app.JoinNotes(warning, w)
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
func open(name string) (*app.Session, string, error) {
	conns, start, warning, err := config.LoadConnections(name)
	if err != nil {
		return nil, "", err
	}
	sess, err := app.Open(context.Background(), conns, start)
	if err != nil {
		return nil, "", fmt.Errorf("%s: %w", conns[start].Name, err)
	}
	return sess, warning, nil
}

// keysCmd is `sqlmux keys [--format md|toml] [--check]` (§6.7).
func keysCmd(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("sqlmux keys", flag.ContinueOnError)
	fs.SetOutput(stderr)
	format := fs.String("format", "md", "输出格式：md | toml")
	check := fs.Bool("check", false, "只检查冲突；有冲突时退出码为 1")
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
		fmt.Fprintf(stderr, "sqlmux keys：不支持的格式 %q\n", *format)
		return 2
	}
	return 0
}
