package app

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
	"slices"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"sqlmux/internal/editor"
	"sqlmux/internal/i18n"
	"sqlmux/internal/sqlkit"
)

type formatDone struct {
	t        *consoleTab
	ver      int // the console's as gq was pressed
	from, to editor.Pos
	text     string
	err      error
	// console.format's: the cursor goes to the start of the statement it
	// was in, the at-th of n; at is -1 for gq's
	at, n int
}

// consoleFormat lays out what gq covered in console t (§9.5): a selection
// as it is, its line break at the end left out; the statement at the
// cursor for gqq; else the statements the motion touches, the first's
// start to the last's end. It is formatted in a Cmd by sql-formatter or
// formatprg; what comes back after the text changed, or with the editor
// no longer in NORMAL, is dropped.
func (a *App) consoleFormat(t *consoleTab, f editor.FormatSpan, whole bool) tea.Cmd {
	lines := t.ed.Lines()
	text := strings.Join(lines, "\n")
	from, to := offsetOf(lines, f.From), offsetOf(lines, f.To)
	d := t.conn.eng.Dialect()
	stmts := sqlkit.Statements(text, d)
	switch {
	case f.Selected:
		to -= len(text[from:to]) - len(strings.TrimSuffix(text[from:to], "\n"))
	case f.Current:
		i := sqlkit.StmtAt(text, stmts, from)
		if i < 0 {
			return nil
		}
		from, to = stmts[i].Start, stmts[i].End
	default:
		var in []sqlkit.Stmt
		for _, s := range stmts {
			if s.Start < max(to, from+1) && s.End > from {
				in = append(in, s)
			}
		}
		if len(in) == 0 {
			return nil
		}
		from, to = in[0].Start, in[len(in)-1].End
	}
	if from >= to {
		return nil
	}
	at := -1
	if whole {
		at = sqlkit.StmtAt(text, stmts, offsetOf(lines, t.ed.Cursor()))
	}
	sql, ver := text[from:to], t.ver
	prg, opts := a.formatPrg, sqlkit.Options{KeywordCase: a.keywordCase, TabWidth: a.tabWidth}
	start, end := posOf(lines, from), posOf(lines, to)
	return func() tea.Msg {
		out, err := format(sql, d, prg, opts)
		return formatDone{t, ver, start, end, out, err, at, len(stmts)}
	}
}

// format is sql laid out by sql-formatter as d's, or by the command prg
// when it is set: sql on its stdin, what it writes to stdout, the first
// line of its stderr when it fails (§9.5). Nothing out is a failure too,
// not the SQL gone; past sqlkit.FormatTimeout it is sqlkit.ErrTimeout.
func format(sql string, d sqlkit.Dialect, prg string, o sqlkit.Options) (string, error) {
	out, err := "", error(nil)
	if prg == "" {
		out, err = sqlkit.Format(sql, d, o)
	} else {
		out, err = runPrg(sql, prg)
	}
	if err == nil && strings.TrimSpace(out) == "" {
		err = errors.New(i18n.T("没有输出"))
	}
	return out, err
}

// runPrg is formatprg: what it writes, sql on its stdin, in sh -c.
func runPrg(sql, prg string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), sqlkit.FormatTimeout)
	defer cancel()
	c := exec.CommandContext(ctx, "sh", "-c", prg)
	c.WaitDelay = 100 * time.Millisecond // a child of sh keeps stdout open: it is not waited for
	var out, stderr bytes.Buffer
	c.Stdin, c.Stdout, c.Stderr = strings.NewReader(sql), &out, &stderr
	if err := c.Run(); ctx.Err() != nil {
		return "", sqlkit.ErrTimeout
	} else if err != nil {
		if first, _, _ := strings.Cut(strings.TrimSpace(stderr.String()), "\n"); first != "" {
			return "", errors.New(first)
		}
		return "", err
	}
	return strings.TrimSuffix(out.String(), "\n"), nil
}

func (a *App) gotFormat(m formatDone) tea.Cmd {
	switch {
	case !slices.Contains(a.consoles(), m.t): // closed meanwhile, or closed and opened again on its file
		return nil
	case m.t.ver != m.ver, m.t.ed.Mode() != editor.Normal && !m.t.ed.Plain(): // the text changed meanwhile, or is being typed
		return nil
	case errors.Is(m.err, sqlkit.ErrTimeout):
		return a.showToast(i18n.T("格式化超时（%v）", sqlkit.FormatTimeout), toastTTL)
	case m.err != nil:
		return a.showToast(i18n.T("格式化失败：%s", m.err.Error()), toastTTL)
	}
	eff := m.t.ed.Replace(m.from, m.to, m.text)
	if m.at >= 0 {
		lines := m.t.ed.Lines()
		// the statements are as many as before, but for what the formatter did to them
		if stmts := sqlkit.Statements(strings.Join(lines, "\n"), m.t.conn.eng.Dialect()); len(stmts) == m.n {
			m.t.ed.Goto(posOf(lines, stmts[m.at].Start))
		}
	}
	return a.consoleDid(m.t, eff)
}

// formatAll is console.format: all the statements of console t laid out as
// gq does (§9.5, F5.16).
func (a *App) formatAll(t *consoleTab) tea.Cmd {
	if t.ed.Mode() != editor.Normal && !t.ed.Plain() {
		return nil
	}
	if from, to, ok := t.ed.SelectionSpan(); ok && t.ed.Plain() { // a selection: as it is, as gq in VISUAL does
		return a.consoleFormat(t, editor.FormatSpan{From: from, To: to, Selected: true}, false)
	}
	lines := t.ed.Lines()
	return a.consoleFormat(t, editor.FormatSpan{To: editor.Pos{Line: len(lines) - 1, Col: len(lines[len(lines)-1])}}, true)
}

// offsetOf is where pos is in lines joined by line breaks; posOf the
// other way.
func offsetOf(lines []string, pos editor.Pos) int {
	off := pos.Col
	for _, l := range lines[:pos.Line] {
		off += len(l) + 1
	}
	return off
}

func posOf(lines []string, off int) editor.Pos {
	for n, l := range lines {
		if off <= len(l) {
			return editor.Pos{Line: n, Col: off}
		}
		off -= len(l) + 1
	}
	return editor.Pos{Line: len(lines) - 1, Col: len(lines[len(lines)-1])}
}
