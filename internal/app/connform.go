package app

import (
	"net"
	"net/url"
	"slices"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"

	"sqlmux/internal/config"
	"sqlmux/internal/i18n"
	"sqlmux/internal/keymap"
	"sqlmux/internal/ui"
)

// connForm is the open new connection form (§7.8「新建连接表单」): its
// fields in the order below, the one with the keys, why ↵ did not save.
type connForm struct {
	fields      []ui.FormField
	sel         int
	err         string
	portTouched bool // typed in: the engine no longer sets it
}

// The form's fields, in order; the last two are in the 高级 fold.
const (
	fieldName = iota
	fieldEngine
	fieldHost
	fieldPort
	fieldUser
	fieldPassword
	fieldDatabase
	fieldReadOnly
	fieldAdvanced
	fieldDSN
	fieldPasswordCmd
)

// connFields are the form's rows by the names they carry, which are the
// toml keys they write (§14).
var connFields = []string{fieldName: "name", fieldEngine: "engine", fieldHost: "host", fieldPort: "port", fieldUser: "user",
	fieldPassword: "password", fieldDatabase: "database", fieldReadOnly: "read_only", fieldAdvanced: "", fieldDSN: "dsn", fieldPasswordCmd: "password_cmd"}

// defaultPort is the port of an engine's server.
var defaultPort = map[string]string{"postgres": "5432", "mysql": "3306"}

// openConnForm is conn.new: the form, empty but for the port, on name.
func (a *App) openConnForm() {
	fs := make([]ui.FormField, len(connFields))
	for i, name := range connFields {
		fs[i].Label = name
	}
	fs[fieldAdvanced].Label = i18n.T("高级")
	fs[fieldEngine].Kind, fs[fieldEngine].Choices = ui.FormSegments, []string{"postgres", "mysql"}
	fs[fieldReadOnly].Kind = ui.FormCheck
	fs[fieldAdvanced].Kind = ui.FormFold
	fs[fieldPassword].Mask = true
	fs[fieldPort].Input = ui.Input{Text: defaultPort["postgres"], Pos: len(defaultPort["postgres"])}
	a.form = &connForm{fields: fs}
}

// advanced is whether the fold is open; dsnGiven whether a dsn is typed in
// it: then host, port, user and database take no part.
func (f *connForm) advanced() bool { return f.fields[fieldAdvanced].On == 1 }
func (f *connForm) dsnGiven() bool {
	return f.advanced() && strings.TrimSpace(f.fields[fieldDSN].Input.Text) != ""
}

// skipped is whether Tab goes past field i.
func (f *connForm) skipped(i int) bool {
	switch i {
	case fieldDSN, fieldPasswordCmd:
		return !f.advanced()
	case fieldHost, fieldPort, fieldUser, fieldDatabase:
		return f.dsnGiven()
	}
	return false
}

// clearErr forgets why ↵ did not save, as the form is touched.
func (f *connForm) clearErr() {
	f.err = ""
	for i := range f.fields {
		f.fields[i].Bad = false
	}
}

// formMove is Tab (1) and S-Tab (-1): the next or previous field, around
// the ends.
func (a *App) formMove(d int) {
	f := a.form
	f.clearErr()
	for range f.fields {
		f.sel = (f.sel + d + len(f.fields)) % len(f.fields)
		if !f.skipped(f.sel) {
			return
		}
	}
}

// formKey is a key no binding takes: text for an input, space (and the
// arrows for the engine) for a switch.
func (a *App) formKey(k keymap.Key) {
	f := a.form
	f.clearErr()
	fd := &f.fields[f.sel]
	switch fd.Kind {
	case ui.FormInput:
		was := fd.Input.Text
		editInput(&fd.Input, k)
		if f.sel == fieldPort && fd.Input.Text != was {
			f.portTouched = true
		}
	case ui.FormSegments:
		switch k {
		case "<Space>", "<Right>":
			a.formSwitch(1)
		case "<Left>":
			a.formSwitch(-1)
		}
	case ui.FormCheck, ui.FormFold:
		if k == "<Space>" {
			fd.On = 1 - fd.On
		}
	}
}

// formSwitch takes the engine's next (1) or previous (-1) value; the port
// goes with it, while it is the default one.
func (a *App) formSwitch(d int) {
	f := a.form
	fd := &f.fields[fieldEngine]
	fd.On = (fd.On + d + len(fd.Choices)) % len(fd.Choices)
	if !f.portTouched {
		p := defaultPort[fd.Choices[fd.On]]
		f.fields[fieldPort].Input = ui.Input{Text: p, Pos: len(p)}
	}
}

// formClick is a click on field i: the keys go to it; a switch also goes
// on to its next value.
func (a *App) formClick(i int) {
	f := a.form
	if f.skipped(i) {
		return
	}
	f.clearErr()
	f.sel = i
	if f.fields[i].Kind != ui.FormInput {
		a.formKey("<Space>")
	}
}

// fail says why ↵ did not save, field i at fault and having the keys.
func (a *App) fail(i int, why string) tea.Cmd {
	a.form.err, a.form.sel = why, i
	a.form.fields[i].Bad = true
	return nil
}

// composeDSN is the dsn of host, port, user and database as a URL, the
// user's and the database's characters escaped; no password in it (§13).
func composeDSN(engine, host, port, user, database string) string {
	u := url.URL{Scheme: engine, User: url.User(user), Host: net.JoinHostPort(host, port), Path: "/" + database}
	if engine == "postgres" {
		u.RawQuery = "sslmode=prefer"
	}
	return u.String()
}

// saveConnForm is ↵: the connection, checked as connections.toml is when
// read, goes at its end and into every session's tree, not connected; one
// that doesn't check says why, its field getting the keys (§7.8). The first
// connection of a start with none is connected, shown and given a console
// (F7.4).
func (a *App) saveConnForm() tea.Cmd {
	f := a.form
	fs := f.fields
	text := func(i int) string { return strings.TrimSpace(fs[i].Input.Text) }
	c := config.Connection{Name: text(fieldName), Engine: fs[fieldEngine].Choices[fs[fieldEngine].On], DSN: "-", ReadOnly: fs[fieldReadOnly].On == 1}
	if f.advanced() {
		c.PasswordCmd = text(fieldPasswordCmd)
	}
	var names []string
	for _, conn := range a.sess.Conns {
		names = append(names, conn.Spec.Name)
	}
	if field, problem := c.Check(names); problem != "" && field != "dsn" {
		return a.fail(slices.Index(connFields, field), problem)
	}
	if f.dsnGiven() {
		c.DSN = text(fieldDSN)
	} else {
		for _, i := range []int{fieldHost, fieldUser, fieldDatabase} {
			if text(i) == "" {
				return a.fail(i, i18n.T("缺少 %s", connFields[i]))
			}
		}
		if p, err := strconv.Atoi(text(fieldPort)); err != nil || p < 1 || p > 65535 {
			return a.fail(fieldPort, i18n.T("port 要是 1–65535 的数字"))
		}
		c.DSN = composeDSN(c.Engine, text(fieldHost), text(fieldPort), text(fieldUser), text(fieldDatabase))
	}
	var toast tea.Cmd
	if pw := fs[fieldPassword].Input.Text; pw != "" {
		if c.PasswordCmd != "" { // two sources: which one is wanted is not for us to say
			return a.fail(fieldPassword, i18n.T("password 和 password_cmd 填一个就够了"))
		}
		cmd, stored, err := config.StorePassword(c.Name, pw)
		switch {
		case err != nil: // not a keychain without a tool: a locked one the user can unlock and ↵ again
			return a.fail(fieldPassword, err.Error())
		case stored:
			c.PasswordCmd = cmd
		default: // connections.toml is 0600 (§13)
			c.Password = pw
			toast = a.showToast(i18n.T("没有钥匙串工具，密码以明文存进了 connections.toml"), toastTTL)
		}
	}
	if err := config.AppendConnection(c); err != nil {
		f.err = err.Error()
		return nil
	}
	first := len(a.sess.Conns) == 0
	for _, s := range a.sessions {
		s.Conns = append(s.Conns, newConn(c))
	}
	a.form = nil
	if first {
		return tea.Batch(toast, a.startFirst(a.sess.Conns[0]))
	}
	return toast
}

// startFirst makes conn, the first connection of a start with none, what
// the session started with, as if it had been named on the command line: it
// connects, and console_1 opens beside the empty pane on it (F7.4).
func (a *App) startFirst(conn *Conn) tea.Cmd {
	s := a.sess
	s.start, s.last, s.Name = conn, conn, conn.Spec.Name
	cmds := []tea.Cmd{a.connect(conn)}
	for _, w := range s.Windows {
		if p := w.pane(2); p != nil && len(p.Tabs) == 0 {
			cons, err := openConsole(conn, a.freeConsole())
			if err != nil {
				cmds = append(cmds, a.showToast(err.Error(), toastTTL))
				break
			}
			putTab(p, Tab{Name: cons.key(), Console: cons}, false)
			cmds = append(cmds, a.keepConsole(cons))
			break
		}
	}
	return tea.Batch(cmds...)
}

// formView is the form as it is drawn: each row's icon, the rows of the
// closed fold hidden, those that take no part dim.
func (a *App) formView() ui.Form {
	f := a.form
	fields := slices.Clone(f.fields)
	ic := a.icons
	icons := []ui.Icon{fieldName: ic.Tag, fieldEngine: ic.Engine(fields[fieldEngine].Choices[fields[fieldEngine].On]), fieldHost: ic.Host,
		fieldPort: ic.Port, fieldUser: ic.User, fieldPassword: ic.Key, fieldDatabase: ic.Database, fieldReadOnly: ic.Lock,
		fieldAdvanced: ic.Advanced, fieldDSN: ic.Link, fieldPasswordCmd: ic.Console}
	for i := range fields {
		fields[i].Icon = icons[i]
		fields[i].Hidden = (i == fieldDSN || i == fieldPasswordCmd) && !f.advanced()
		fields[i].Dim = f.skipped(i) && !fields[i].Hidden
	}
	return ui.Form{Title: i18n.T("新建连接"), Fields: fields, Sel: f.sel, Err: f.err, Save: i18n.T("保存"), Cancel: i18n.T("取消"), Hints: bound(
		ui.Hint{Key: a.keys.Hint("form.next", "form"), Label: i18n.T("下一项"), Action: "form.next"},
		ui.Hint{Key: a.keys.Hint("form.save", "form"), Label: i18n.T("保存"), Action: "form.save"},
		ui.Hint{Key: a.keys.Hint("form.cancel", "form"), Label: i18n.T("取消"), Action: "form.cancel"},
	)}
}
