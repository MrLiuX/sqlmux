package app

import (
	"slices"
	"strings"
	"time"

	"github.com/kackerx/sqlmux/internal/config"
	"github.com/kackerx/sqlmux/internal/db"
	"github.com/kackerx/sqlmux/internal/i18n"
)

// recordHistory adds the statement stmt of run r to history.jsonl (§11): res
// is what it did, or failed is why it did not.
// ponytail: best effort; a history that cannot be written is not worth an
// error over the result the user is waiting for
func recordHistory(r *run, stmt string, res db.Result, failed string) {
	h := config.History{At: clock(), Conn: r.conn.Spec.Name, DB: r.conn.Spec.Database, Schema: r.schema, SQL: stmt, Tag: res.Tag, TookMS: res.Took.Milliseconds()}
	h.Error, _, _ = strings.Cut(failed, "\n")
	_ = config.AppendHistory(h)
}

// historyOf is the statements conn ran, the newest first (F14.2).
func historyOf(c *Conn) []config.History {
	hs, _ := config.LoadHistory()
	hs = slices.DeleteFunc(hs, func(h config.History) bool { return h.Conn != c.Spec.Name || h.DB != c.Spec.Database })
	slices.Reverse(hs)
	return hs
}

// historyLine is statement sql on one line, its runs of white space one
// blank, its first historyLineMax characters: the list filters and draws that.
// ponytail: a search does not see what is past them
func historyLine(sql string) string {
	line := []rune(strings.Join(strings.Fields(sql), " "))
	return string(line[:min(len(line), historyLineMax)])
}

const historyLineMax = 300

// historyNote is what the history list notes on h, dim: when, and what came
// of it.
func historyNote(h config.History) string {
	did := (time.Duration(h.TookMS) * time.Millisecond).String()
	switch {
	case h.Error != "":
		did = i18n.T("失败") + ": " + h.Error
	case h.Tag != "":
		did = h.Tag + " · " + did
	}
	return h.At.Local().Format("01-02 15:04") + " · " + did
}
