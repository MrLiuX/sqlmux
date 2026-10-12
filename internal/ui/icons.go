package ui

import (
	"fmt"
	"image/color"

	uv "github.com/charmbracelet/ultraviolet"
)

// Icon is a glyph and, when a theme file gives one, the color it keeps
// wherever it is drawn (§7.7). A nil Fg follows the text around it.
type Icon struct {
	Text string
	Fg   color.Color
}

// On is st with the icon's own color, if it has one.
func (i Icon) On(st uv.Style) uv.Style {
	if i.Fg != nil {
		st.Fg = i.Fg
	}
	return st
}

// Icons are the glyphs used in titles and the sidebar (§7.7).
type Icons struct {
	Schema, Table Icon
	Query         Icon
	Filter        Icon
	Postgres      Icon
	MySQL         Icon
	SQLite        Icon
	Search, Keys  Icon // status bar: palette entry, pending keys
	Conn          Icon
	Key           Icon // primary key columns
	Command       Icon // palette rows (K-03)
	Window        Icon // the tree's workspace (§7.8)
	Pane          Icon // not drawn since F10.5; a theme file that names it still loads (§7.7)
	// The query bar's buttons (Q-05) and ORDER's direction; nil Fg: info
	// and warn (§7.7)
	Save, Refresh, Transpose Icon
	RowAdd, RowDelete        Icon // the query bar's data group (§7.8「工具按钮」)
	AutoRefresh, Stop        Icon // and its query group
	SortAsc, SortDesc        Icon
	View, Column             Icon // the tree's (F1.12)
	Lock                     Icon // a read-only connection's (§13)
	Result, Pin, Export, Log Icon // the result area's (§11)
	Close                    Icon
	Star, History            Icon // a WHERE's favorites and history (§9.7)
	Zoom                     Icon // after the number of the pane zoomed (F9.6)
	// The connection form's rows (F7.4); it also has Postgres, MySQL, Key,
	// Lock and Query
	Tag, Host, Port, User Icon
	Database              Icon
	Advanced, Link, Group Icon
	// Labeled sets (ascii) say nothing by themselves, so the words beside
	// the icons stay: pane types in titles, C-p, the filter's / (§7.7).
	Labeled bool
}

// Nerd Font glyphs, all from the BMP private use area.
var NerdIcons = &Icons{
	Schema:      Icon{Text: "\uf0e8"}, // nf-fa-sitemap
	Table:       Icon{Text: "\uf0ce"}, // nf-fa-table
	Query:       Icon{Text: "\uf489"}, // nf-oct-terminal
	Filter:      Icon{Text: "\uf0b0"}, // nf-fa-filter
	Postgres:    Icon{Text: "\ue76e"}, // nf-dev-postgresql
	MySQL:       Icon{Text: "\ue704"}, // nf-dev-mysql
	SQLite:      Icon{Text: "\ue7c4"}, // nf-dev-sqllite
	Search:      Icon{Text: "\uf002"}, // nf-fa-search
	Keys:        Icon{Text: "\uf11c"}, // nf-fa-keyboard_o
	Conn:        Icon{Text: "\uf1e6"}, // nf-fa-plug
	Key:         Icon{Text: "\uf084"}, // nf-fa-key
	Command:     Icon{Text: "\uf0e7"}, // nf-fa-bolt
	Window:      Icon{Text: "\uf2d2"}, // nf-fa-window_restore
	Save:        Icon{Text: "\uf0c7"}, // nf-fa-save
	Refresh:     Icon{Text: "\uf021"}, // nf-fa-refresh
	Transpose:   Icon{Text: "\uf0ec"}, // nf-fa-exchange
	RowAdd:      Icon{Text: "\uf067"}, // nf-fa-plus
	RowDelete:   Icon{Text: "\uf068"}, // nf-fa-minus
	AutoRefresh: Icon{Text: "\uf017"}, // nf-fa-clock_o
	Stop:        Icon{Text: "\uf04d"}, // nf-fa-stop
	SortAsc:     Icon{Text: "\uf160"}, // nf-fa-sort_amount_asc
	SortDesc:    Icon{Text: "\uf161"}, // nf-fa-sort_amount_desc
	View:        Icon{Text: "\uf06e"}, // nf-fa-eye
	Column:      Icon{Text: "\ueb5f"}, // nf-cod-symbol_field
	Lock:        Icon{Text: "\uf023"}, // nf-fa-lock
	Result:      Icon{Text: "\uf022"}, // nf-fa-list_alt
	Pin:         Icon{Text: "\uf435"}, // nf-oct-pin
	Export:      Icon{Text: "\uf019"}, // nf-fa-download
	Close:       Icon{Text: "\uf00d"}, // nf-fa-times
	Log:         Icon{Text: "\uf0f6"}, // nf-fa-file_text_o
	Star:        Icon{Text: "\uf005"}, // nf-fa-star
	Zoom:        Icon{Text: "\uf065"}, // nf-fa-expand
	History:     Icon{Text: "\uf1da"}, // nf-fa-history
	Tag:         Icon{Text: "\uf02b"}, // nf-fa-tag
	Host:        Icon{Text: "\uf233"}, // nf-fa-server
	Port:        Icon{Text: "\uf1e6"}, // nf-fa-plug
	User:        Icon{Text: "\uf007"}, // nf-fa-user
	Database:    Icon{Text: "\uf1c0"}, // nf-fa-database
	Advanced:    Icon{Text: "\uf013"}, // nf-fa-cog
	Group:       Icon{Text: "\uf02c"}, // nf-fa-tags
	Link:        Icon{Text: "\uf0c1"}, // nf-fa-link
}

var ASCIIIcons = &Icons{
	Schema: Icon{Text: "#"}, Table: Icon{Text: "+"}, Query: Icon{Text: ">"},
	Filter:      Icon{Text: "?"},
	Postgres:    Icon{Text: "pg"},
	MySQL:       Icon{Text: "my"},
	SQLite:      Icon{Text: "sq"},
	Search:      Icon{Text: "~"},
	Keys:        Icon{Text: "kb"},
	Conn:        Icon{Text: "@"},
	Key:         Icon{Text: "*"},
	Command:     Icon{Text: ":"},
	Window:      Icon{Text: "[]"},
	Save:        Icon{Text: "[S]"},
	Refresh:     Icon{Text: "[R]"},
	Transpose:   Icon{Text: "[T]"},
	RowAdd:      Icon{Text: "+"},
	RowDelete:   Icon{Text: "-"},
	AutoRefresh: Icon{Text: "@"},
	Stop:        Icon{Text: "#"},
	SortAsc:     Icon{Text: "↑"},
	SortDesc:    Icon{Text: "↓"},
	View:        Icon{Text: "v"},
	Column:      Icon{Text: "-"},
	Lock:        Icon{Text: "RO"},
	Result:      Icon{Text: "="},
	Pin:         Icon{Text: "*"},
	Export:      Icon{Text: ">"},
	Close:       Icon{Text: "x"},
	Log:         Icon{Text: "L"},
	Star:        Icon{Text: "*"},
	Zoom:        Icon{Text: "[Z]"},
	History:     Icon{Text: "h"},
	Tag:         Icon{Text: "#"},
	Host:        Icon{Text: "@"},
	Port:        Icon{Text: ":"},
	User:        Icon{Text: "~"},
	Database:    Icon{Text: "="},
	Advanced:    Icon{Text: "+"},
	Group:       Icon{Text: "%"},
	Link:        Icon{Text: "&"},
	Labeled:     true,
}

// IconSet maps the config value `icons = "nerd" | "ascii"`.
func IconSet(name string) *Icons {
	if name == "ascii" {
		return ASCIIIcons
	}
	return NerdIcons
}

// Engine is the icon of a connection to engine, as connections.toml
// names it (§7.7).
func (ic *Icons) Engine(engine string) Icon {
	switch engine {
	case "mysql":
		return ic.MySQL
	case "sqlite":
		return ic.SQLite
	}
	return ic.Postgres
}

// Number writes pane n's ⟨n⟩ (§7.8): beside Nerd icons ⓪ ① … ⑳, one
// column each; with ascii icons, or past 20, ⟨n⟩.
func (ic *Icons) Number(n int) string {
	switch {
	case ic.Labeled || n < 0 || n > 20:
		return fmt.Sprintf("⟨%d⟩", n)
	case n == 0:
		return "\u24ea" // ⓪
	}
	return string(rune(0x2460 + n - 1)) // ① is U+2460
}

// byName names the icons the way a theme file's [icon] table does (§7.7).
func (ic *Icons) byName() map[string]*Icon {
	return map[string]*Icon{
		"schema": &ic.Schema, "table": &ic.Table, "query": &ic.Query,
		"filter": &ic.Filter, "search": &ic.Search, "keys": &ic.Keys, "conn": &ic.Conn,
		"key": &ic.Key, "postgres": &ic.Postgres, "mysql": &ic.MySQL, "sqlite": &ic.SQLite, "command": &ic.Command, "window": &ic.Window,
		"save": &ic.Save, "refresh": &ic.Refresh, "transpose": &ic.Transpose,
		"row_add": &ic.RowAdd, "row_delete": &ic.RowDelete, "auto_refresh": &ic.AutoRefresh, "stop": &ic.Stop,
		"sort_asc": &ic.SortAsc, "sort_desc": &ic.SortDesc, "view": &ic.View, "column": &ic.Column, "lock": &ic.Lock,
		"result": &ic.Result, "pin": &ic.Pin, "export": &ic.Export, "close": &ic.Close,
		"pane": &ic.Pane, "log": &ic.Log, "star": &ic.Star, "history": &ic.History, "zoom": &ic.Zoom,
		"tag": &ic.Tag, "host": &ic.Host, "port": &ic.Port, "user": &ic.User, "database": &ic.Database,
		"advanced": &ic.Advanced, "link": &ic.Link, "group": &ic.Group,
	}
}
