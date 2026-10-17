package app

import (
	"fmt"

	tea "charm.land/bubbletea/v2"

	"github.com/kackerx/sqlmux/internal/i18n"
)

// writeBox is the question put before a write on connection c, text what it is
// about and yes the answer's name: on a production connection the first line is
// "<name> 是生产连接" and the box is a danger's, whatever confirm_dangerous says
// (§13, F17.9); callers ask only where they would anyway, or c is one.
func writeBox(c *Conn, text, yes string, danger bool, then func() tea.Cmd) *confirmBox {
	if c.Spec.Production {
		text, danger = fmt.Sprintf(i18n.T("%s 是生产连接"), c.Spec.Name)+"\n"+text, true
	}
	return &confirmBox{text: text, yes: yes, danger: danger, then: then}
}
