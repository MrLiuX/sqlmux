package main

import (
	"errors"
	"flag"
	"slices"
	"strings"

	"github.com/kackerx/sqlmux/internal/app"
	"github.com/kackerx/sqlmux/internal/config"
	"github.com/kackerx/sqlmux/internal/db"
)

// tableFlags are the --table mode's options (§14「sqlmux --table」): one table
// over one connection, everything else is the program's own.
type tableFlags struct {
	table     string
	where     string
	cols      string // only these columns
	hide      string // hide these columns
	order     string
	db        string
	limit     int
	zoom      bool
	readonly  bool
	transpose bool
	sidebar   bool
}

// register adds the flags to fs.
func (tf *tableFlags) register(fs *flag.FlagSet) {
	fs.StringVar(&tf.table, "table", "", "open this table (schema.table) over the connection")
	fs.StringVar(&tf.where, "where", "", "apply this WHERE condition")
	fs.StringVar(&tf.cols, "cols", "", "show only these columns, in this order (a,b,c)")
	fs.StringVar(&tf.hide, "hide", "", "hide these columns (a,b,c)")
	fs.StringVar(&tf.order, "order", "", "sort by these columns (\"col desc, col2\")")
	fs.StringVar(&tf.db, "db", "", "the database (PG / Redis / SQL Server)")
	fs.IntVar(&tf.limit, "limit", 0, "rows per page (1..10000)")
	fs.BoolVar(&tf.zoom, "zoom", false, "fullscreen the pane")
	fs.BoolVar(&tf.readonly, "readonly", false, "force the connection read-only")
	fs.BoolVar(&tf.transpose, "transpose", false, "transpose the table")
	fs.BoolVar(&tf.sidebar, "sidebar", false, "show the connection tree")
}

// set is whether the table mode is on.
func (tf *tableFlags) set() bool { return tf.table != "" }

// validate says why the flags cannot go together or are not enough. It runs
// before config.Load, so it is English like the other usage errors (§14).
func (tf *tableFlags) validate(name string) error {
	if tf.cols != "" && tf.hide != "" {
		return errors.New("give --cols or --hide, not both")
	}
	if _, table := splitTable(tf.table); table == "" {
		return errors.New("--table needs a table name, schema.table or table")
	}
	if name == "" {
		return errors.New("--table needs a connection name, file or url")
	}
	return nil
}

// session is the --table mode's session: the connection described by the
// flags, over a layout with a single table tab, restored as --layout's.
func (tf *tableFlags) session(name string) (*app.Session, *app.Restored, error) {
	specs, target, err := tf.specs(name)
	if err != nil {
		return nil, nil, err
	}
	if tf.readonly {
		for i := range specs {
			if specs[i].Name == target {
				specs[i].ReadOnly = true
			}
		}
	}
	r := app.Restore(specs, tf.layout(target))
	return r.Sessions[0], r, nil
}

// specs is the connections Restore builds over and the name the layout refers
// to: [temp] for a file or url, else connections.toml's all.
func (tf *tableFlags) specs(name string) ([]config.Connection, string, error) {
	conns, _, err := config.LoadConnections("")
	if err != nil && !config.NoConnections(err) {
		return nil, "", err
	}
	known := make([]string, len(conns))
	for i, c := range conns {
		known[i] = c.Name
	}
	if config.IsTarget(name, known) {
		c, err := config.TempConnection(name, known)
		if err != nil {
			return nil, "", err
		}
		return []config.Connection{c}, c.Name, nil
	}
	if !slices.Contains(known, name) {
		// LoadConnections's own error names the path and the connections
		_, _, err := config.LoadConnections(name)
		return nil, "", err
	}
	return conns, name, nil
}

// layout is the one-session, one-window, one-pane, one-table layout the flags
// describe.
func (tf *tableFlags) layout(target string) *config.Layout {
	schema, table := splitTable(tf.table)
	zoom := 0
	if tf.zoom {
		zoom = 1
	}
	return &config.Layout{
		Version: config.LayoutVersion,
		Active:  0,
		Sessions: []config.LayoutSession{{
			Name:       target,
			Connection: target,
			Windows: []config.LayoutWindow{{
				Layout:      &config.LayoutNode{Pane: 1},
				Focus:       1,
				Zoom:        zoom,
				SidebarOpen: tf.sidebar,
				Panes: []config.LayoutPane{{
					ID:      1,
					Current: 0,
					Tabs: []config.LayoutTab{{
						Kind:       "table",
						Connection: target,
						DB:         tf.db,
						Schema:     schema,
						Table:      table,
						Where:      tf.where,
						Order:      parseOrder(tf.order),
						Limit:      tf.limit,
						Cols:       splitCSV(tf.hide),
						Show:       splitCSV(tf.cols),
						Transpose:  tf.transpose,
					}},
				}},
			}},
		}},
	}
}

// splitTable splits schema.table; with no dot the table is the whole thing.
func splitTable(s string) (schema, table string) {
	if i := strings.IndexByte(s, '.'); i >= 0 {
		return s[:i], s[i+1:]
	}
	return "", s
}

// parseOrder reads "col [desc], col2" into the layout's order.
func parseOrder(s string) config.LayoutOrder {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	var out config.LayoutOrder
	for _, part := range strings.Split(s, ",") {
		f := strings.Fields(part)
		if len(f) == 0 {
			continue
		}
		oc := db.OrderCol{Col: f[0]}
		if len(f) > 1 && strings.EqualFold(f[1], "desc") {
			oc.Desc = true
		}
		out = append(out, oc)
	}
	return out
}

// splitCSV splits a, b ,c into its columns, dropping blanks.
func splitCSV(s string) []string {
	if s == "" {
		return nil
	}
	var out []string
	for _, f := range strings.Split(s, ",") {
		if f = strings.TrimSpace(f); f != "" {
			out = append(out, f)
		}
	}
	return out
}
