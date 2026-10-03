package postgres

import (
	"errors"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"

	"sqlmux/internal/db"
)

// ErrorOf is err as the log and an error bar show it (§11, §7.8「错误栏」):
// a PgError's parts, pgconn's dressing off.
func (Engine) ErrorOf(err error) db.ServerError {
	var pe *pgconn.PgError
	if !errors.As(err, &pe) {
		return db.ServerError{Severity: "ERROR", Message: err.Error()}
	}
	e := db.ServerError{Severity: pe.Severity, Code: pe.Code, Message: pe.Message, Position: int(pe.Position)}
	for _, l := range [][2]string{{"DETAIL", pe.Detail}, {"HINT", pe.Hint}} {
		if l[1] != "" {
			e.More = append(e.More, strings.Split(l[0]+": "+l[1], "\n")...)
		}
	}
	return e
}
