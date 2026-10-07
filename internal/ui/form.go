package ui

import (
	"strings"

	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
)

// FormKind is what a form row holds.
type FormKind int

const (
	FormInput    FormKind = iota // a line of text
	FormSegments                 // one of Choices, all shown
	FormCheck                    // [x] or [ ]
	FormFold                     // a row that opens and closes the rows after it
)

// FormField is a row of a form (§7.8「新建连接表单」): an icon, a name and
// what it holds. On is the segment, 1 for checked, or 1 for open.
type FormField struct {
	Label   string
	Icon    Icon
	Kind    FormKind
	Input   Input
	Choices []string
	On      int
	Mask    bool // the input is drawn as * (a password)
	Dim     bool // not taking part: drawn dim
	Bad     bool // its name in the error color: why ↵ did not save
	Hidden  bool // not drawn
}

// Form is a box of fields over the screen: Sel is the field with the keys,
// Err why it did not save, in error above the buttons.
type Form struct {
	Fields       []FormField
	Sel          int
	Err          string
	Save, Cancel string // the buttons' words
	Delete       string // a third button's word, when a connection there already is can go (F8.16); "" for none
	Hints        []Hint
}

// rows is how many fields show.
func (fm Form) rows() int {
	n := 0
	for _, fd := range fm.Fields {
		if !fd.Hidden {
			n++
		}
	}
	return n
}

// height is the form's rows, its border in: fields, error, rule, buttons.
func (fm Form) height() int { return fm.rows() + 5 }

// drawIn paints the fields, the error, the rule and the buttons inside in,
// the border's inner area, the field Sel having the keys when focus; it
// returns where its input has the cursor, else (-1, -1).
func (fm Form) drawIn(f *Frame, area uv.Rectangle, focus bool) uv.Position {
	th := f.Theme
	cursor := uv.Pos(-1, -1)
	bg, dim := uv.Style{Fg: th.Fg, Bg: th.PaneBg}, uv.Style{Fg: th.Dim, Bg: th.PaneBg}
	x0, x1 := area.Min.X+1, area.Max.X-1
	iconW, labelW := 0, 0
	for _, fd := range fm.Fields {
		iconW, labelW = max(iconW, Width(fd.Icon.Text)), max(labelW, Width(fd.Label)) // the hidden ones too: nothing shifts as the fold opens
	}
	y := area.Min.Y
	for i, fd := range fm.Fields {
		if fd.Hidden {
			continue
		}
		f.Region(uv.Rect(area.Min.X, y, area.Dx(), 1), Target{Kind: KindRow, I: i})
		sel := focus && i == fm.Sel
		name, icon := dim, dim
		if sel {
			name, icon = bg, bg
		}
		if fd.Bad {
			name.Fg = th.Error
		}
		f.Text(x0, y, x1, fd.Icon.Text, fd.Icon.On(icon))
		f.Text(x0+iconW+1+labelW-Width(fd.Label), y, x1, fd.Label, name)
		r := uv.Rect(x0+iconW+labelW+2, y, max(x1-x0-iconW-labelW-2, 0), 1)
		in := uv.Style{Fg: th.Fg, Bg: th.Sep}
		if sel {
			in.Bg = th.Select
		}
		if fd.Dim {
			in.Fg = th.Dim
		}
		switch fd.Kind {
		case FormInput:
			f.Fill(r, in)
			if fd.Mask {
				fd.Input = fd.Input.masked()
			}
			if c := fd.Input.Draw(f, r, in); sel {
				cursor = c
			}
		case FormSegments:
			x := r.Min.X
			for j, ch := range fd.Choices {
				st := uv.Style{Fg: th.Dim, Bg: th.Sep}
				if j == fd.On {
					st = uv.Style{Fg: th.Fg, Bg: th.Select}
				}
				if j > 0 {
					x = f.Text(x, y, r.Max.X, "│", dim)
				}
				f.Region(uv.Rect(x, y, Width(ch)+2, 1), Target{Kind: KindSegment, Pane: i, I: j})
				x = f.Text(x, y, r.Max.X, " "+ch+" ", st)
			}
		case FormCheck:
			mark := "[ ]"
			if fd.On == 1 {
				mark = "[x]"
			}
			f.Text(r.Min.X, y, r.Max.X, mark, in)
		case FormFold:
			mark := "▸"
			if fd.On == 1 {
				mark = "▾"
			}
			f.Text(r.Min.X, y, r.Max.X, mark, name)
		}
		y++
	}
	f.Text(x0, y, x1, fm.Err, uv.Style{Fg: th.Error, Bg: th.PaneBg})
	f.Text(area.Min.X, y+1, area.Max.X, strings.Repeat("─", area.Dx()), uv.Style{Fg: th.Sep, Bg: th.PaneBg})
	x, y := x0, y+2
	buttons := []struct{ text, action string }{{fm.Save, "form.save"}, {fm.Cancel, "form.cancel"}}
	if fm.Delete != "" {
		buttons = append(buttons, struct{ text, action string }{fm.Delete, "conns.delete"})
	}
	for _, b := range buttons {
		st := bg
		if f.Region(uv.Rect(x, y, Width(b.text)+4, 1), Target{Kind: KindButton, Action: b.action}) {
			st.Bg = th.Select
		}
		edge := st
		edge.Fg = th.Focus
		x = f.Text(x, y, x1, "[", edge)
		x = f.Text(x, y, x1, " "+b.text+" ", st)
		x = f.Text(x, y, x1, "]", edge) + 1
	}
	if hx := x1 - hintRowWidth(fm.Hints); hx > x+1 {
		hintRow(f, hx, y, x1, fm.Hints, dim, Target{Kind: KindButton})
	}
	return cursor
}

// masked is in with a * for each of its graphemes, the cursor after as many
// as it was.
func (in Input) masked() Input {
	count := func(s string) (n int) {
		for ; s != ""; n++ {
			gr, _ := ansi.FirstGraphemeCluster(s, ansi.GraphemeWidth)
			s = s[len(gr):]
		}
		return n
	}
	return Input{Text: strings.Repeat("*", count(in.Text)), Pos: count(in.Text[:in.Pos])}
}
