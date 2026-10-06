package app

import (
	"slices"

	tea "charm.land/bubbletea/v2"

	"sqlmux/internal/config"
	"sqlmux/internal/i18n"
	"sqlmux/internal/keymap"
	"sqlmux/internal/ui"
)

// connForm is the open new connection form (§7.8「新建连接表单」): its
// fields in connFields' order, the one with the keys, why ↵ did not save.
type connForm struct {
	fields []ui.FormField
	sel    int
	err    string
}

// The form's fields, in order.
const (
	fieldName = iota
	fieldEngine
	fieldDSN
	fieldPassword
	fieldPasswordCmd
	fieldReadOnly
)

// connFields are the form's fields by the toml keys they write (§14).
var connFields = []string{fieldName: "name", fieldEngine: "engine", fieldDSN: "dsn", fieldPassword: "password", fieldPasswordCmd: "password_cmd", fieldReadOnly: "read_only"}

// openConnForm is conn.new: the form, empty, on name.
func (a *App) openConnForm() {
	fs := make([]ui.FormField, len(connFields))
	for i, name := range connFields {
		fs[i].Label = name
	}
	fs[fieldPassword].Mask = true
	fs[fieldEngine].Choices = []string{"postgres", "mysql"}
	fs[fieldReadOnly].Choices = []string{"false", "true"}
	a.form = &connForm{fields: fs}
}

// formMove is Tab (1) and S-Tab (-1): the next or previous field, around
// the ends.
func (a *App) formMove(d int) {
	f := a.form
	f.sel = (f.sel + d + len(f.fields)) % len(f.fields)
}

// formKey is a key no binding takes: text for an input, space for a
// switch, which goes on to its next value.
func (a *App) formKey(k keymap.Key) {
	fd := &a.form.fields[a.form.sel]
	switch {
	case fd.Choices == nil:
		editInput(&fd.Input, k)
	case k == "<Space>":
		fd.On = (fd.On + 1) % len(fd.Choices)
	}
}

// formClick is a click on field i: the keys go to it; a switch also goes
// on to its next value.
func (a *App) formClick(i int) {
	a.form.sel = i
	if a.form.fields[i].Choices != nil {
		a.formKey("<Space>")
	}
}

// saveConnForm is ↵: the connection, checked as connections.toml is when
// read, goes at its end and into every session's tree, not connected; one
// that doesn't check says why, its field getting the keys (§7.8).
func (a *App) saveConnForm() tea.Cmd {
	fs := a.form.fields
	c := config.Connection{
		Name: fs[fieldName].Input.Text, Engine: fs[fieldEngine].Choices[fs[fieldEngine].On], DSN: fs[fieldDSN].Input.Text,
		PasswordCmd: fs[fieldPasswordCmd].Input.Text, ReadOnly: fs[fieldReadOnly].On == 1,
	}
	var names []string
	for _, conn := range a.sess.Conns {
		names = append(names, conn.Spec.Name)
	}
	if field, problem := c.Check(names); problem != "" {
		a.form.err, a.form.sel = problem, slices.Index(connFields, field)
		return nil
	}
	var toast tea.Cmd
	if pw := fs[fieldPassword].Input.Text; pw != "" {
		if c.PasswordCmd != "" { // two sources: which one is wanted is not for us to say
			a.form.err, a.form.sel = i18n.T("password 和 password_cmd 填一个就够了"), fieldPassword
			return nil
		}
		cmd, stored, err := config.StorePassword(c.Name, pw)
		switch {
		case err != nil: // not a keychain without a tool: a locked one the user can unlock and ↵ again
			a.form.err = err.Error()
			return nil
		case stored:
			c.PasswordCmd = cmd
		default: // connections.toml is 0600 (§13)
			c.Password = pw
			toast = a.showToast(i18n.T("没有钥匙串工具，密码以明文存进了 connections.toml"), toastTTL)
		}
	}
	if err := config.AppendConnection(c); err != nil {
		a.form.err = err.Error()
		return nil
	}
	for _, s := range a.sessions {
		s.Conns = append(s.Conns, newConn(c))
	}
	a.form = nil
	return toast
}

func (a *App) formView() ui.Form {
	f := a.form
	return ui.Form{Title: i18n.T("新建连接"), Fields: f.fields, Sel: f.sel, Err: f.err, Hints: bound(
		ui.Hint{Key: a.keys.Hint("form.next", "form"), Label: i18n.T("下一项"), Action: "form.next"},
		ui.Hint{Key: a.keys.Hint("form.save", "form"), Label: i18n.T("保存"), Action: "form.save"},
		ui.Hint{Key: a.keys.Hint("form.cancel", "form"), Label: i18n.T("取消"), Action: "form.cancel"},
	)}
}
