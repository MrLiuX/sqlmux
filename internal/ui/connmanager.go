package ui

import (
	uv "github.com/charmbracelet/ultraviolet"
)

// ConnRow is a connection of the manager's list (§7.8「连接管理」): its
// engine's icon, its name, a lock when it is read only; the last row is the
// one that makes a new one.
type ConnRow struct {
	Icon Icon
	Name string
	Lock Icon // Text "" for none
}

// ConnManager is the connections as a list on the left and the form of the
// one selected on the right, in a box of its own (F8.16).
type ConnManager struct {
	Title     string
	Rows      []ConnRow
	Sel, Top  int  // Top is the first row showing
	ListFocus bool // the list has the keys, else the form's selected field
	Form      Form
}

const (
	connManagerWidth = 100
	connListWidth    = 26
	narrowListWidth  = 22 // when the screen does not give the manager its width: the form's buttons and keys under it need the room (F7.4)
)

// ListRows is how many rows of the list show: the form's height, which does
// not grow with the connections (F8.16).
func (m ConnManager) ListRows() int { return m.Form.height() - 2 }

// Draw dims what is behind, paints the manager centered and returns where
// the form's input has the cursor, else (-1, -1).
func (m ConnManager) Draw(f *Frame, screen uv.Rectangle) uv.Position {
	th := f.Theme
	w, h := min(connManagerWidth, screen.Dx()-4), m.Form.height()
	box := uv.Rect(screen.Min.X+(screen.Dx()-w)/2, screen.Min.Y+max(screen.Dy()-h, 0)/2, w, h)
	f.Dim()
	f.Region(f.Bounds(), Target{}) // no click goes past it
	listW := connListWidth
	if w < connManagerWidth {
		listW = narrowListWidth
	}
	if box.Dx() < listW+10 || box.Dy() > screen.Dy() {
		return uv.Pos(-1, -1)
	}
	bg := uv.Style{Fg: th.Fg, Bg: th.PaneBg}
	f.Fill(box, bg)
	border := f.Border.Style(uv.Style{Fg: th.Focus, Bg: th.PaneBg})
	border.Draw(f.Buf, box)
	f.Text(box.Min.X+2, box.Min.Y, box.Max.X-1, " "+m.Title+" ", uv.Style{Fg: th.Focus, Bg: th.PaneBg, Attrs: uv.AttrBold})
	in := uv.Rect(box.Min.X+1, box.Min.Y+1, box.Dx()-2, box.Dy()-2)
	rule := in.Min.X + listW
	top := max(min(m.Top, m.Sel), m.Sel-in.Dy()+1, 0) // the selection in sight, whatever Top says
	for i := top; i < min(len(m.Rows), top+in.Dy()); i++ {
		r := m.Rows[i]
		line := uv.Rect(in.Min.X, in.Min.Y+i-top, listW, 1)
		st := bg
		switch hover := f.Region(line, Target{Kind: KindConn, I: i}); {
		case i == m.Sel && m.ListFocus:
			st.Bg = th.Select
		case i == m.Sel:
			st.Bg = th.Row
		case hover:
			st.Bg = th.Row
		}
		f.Fill(line, st)
		x := line.Min.X + 1
		if r.Icon.Text != "" {
			x = f.Text(x, line.Min.Y, line.Max.X-1, r.Icon.Text+" ", r.Icon.On(st))
		}
		f.Text(x, line.Min.Y, line.Max.X-1-Width(r.Lock.Text)-1, r.Name, st)
		if r.Lock.Text != "" {
			f.Text(line.Max.X-1-Width(r.Lock.Text), line.Min.Y, line.Max.X, r.Lock.Text, r.Lock.On(uv.Style{Fg: th.Dim, Bg: st.Bg}))
		}
	}
	for y := in.Min.Y; y < in.Max.Y; y++ {
		f.Text(rule, y, rule+1, "│", uv.Style{Fg: th.Sep, Bg: th.PaneBg})
	}
	return m.Form.drawIn(f, uv.Rect(rule+1, in.Min.Y, in.Max.X-rule-1, in.Dy()), !m.ListFocus)
}
