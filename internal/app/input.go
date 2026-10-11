package app

import (
	"github.com/kackerx/sqlmux/internal/editor"
	"github.com/kackerx/sqlmux/internal/keymap"
	"github.com/kackerx/sqlmux/internal/ui"
)

// editInput applies an input's own editing keys to in (they are no
// bindings) and reports whether its text changed.
func editInput(in *ui.Input, k keymap.Key) bool {
	switch k {
	case "<Left>":
		in.Left()
		return false
	case "<Right>":
		in.Right()
		return false
	case "<BS>", "<C-h>": // C-h is a backspace in every typing, as a query's and the command line's are (F13.11)
		in.Backspace()
		return true
	case "<C-w>", "<M-BS>": // M-BS is C-w in all typing (§7.9)
		in.DeleteBack(editor.WordStart(in.Text, in.Pos))
		return true
	case "<C-u>":
		in.DeleteBack(0)
		return true
	}
	if keymap.Text(k) == "" {
		return false
	}
	in.Insert(keymap.Text(k))
	return true
}
