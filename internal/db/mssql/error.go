package mssql

import (
	"database/sql/driver"
	"errors"
	"strconv"

	mssqldb "github.com/microsoft/go-mssqldb"

	"github.com/kackerx/sqlmux/internal/db"
)

// Broken says whether err is a lost connection (§8.2「断线重连」):
// database/sql's ErrBadConn is the driver's for one that sent nothing; the
// rest is the network's, or the server's error numbers of a session it
// dropped or a path that is gone (F18.6).
func (Engine) Broken(err error) db.Lost {
	var me mssqldb.Error
	switch {
	case errors.Is(err, driver.ErrBadConn):
		return db.LostUnsent
	case db.NetworkError(err):
		return db.LostSent
	case errors.As(err, &me) && brokenNumbers[me.Number]:
		return db.LostSent
	}
	return db.NotLost
}

// brokenNumbers are the server's error numbers that say the connection is
// gone: the network's, and the session's the server dropped. A login that
// failed (18456) or a database it cannot open (4060) is the user's to fix,
// the connection stands (F18.6).
var brokenNumbers = map[int32]bool{
	64:    true, // the named-pipe path to the server is gone
	121:   true, // the semaphore timeout of a network path
	233:   true, // the server dropped the session at its init
	10053: true, // the host dropped an established connection
	10054: true, // the peer reset it
	10060: true, // timed out reaching it
	10061: true, // nothing listens there any more
	17809: true, // no more sessions to be had
}

// ErrorOf is err as the log and an error bar show it (§11): the server's
// error number and message, the procedure and the line it names when it
// does (F18.6).
func (Engine) ErrorOf(err error) db.ServerError {
	var me mssqldb.Error
	if !errors.As(err, &me) {
		return db.ServerError{Severity: "ERROR", Message: err.Error()}
	}
	e := db.ServerError{Severity: "ERROR", Code: strconv.Itoa(int(me.Number)), Message: me.Message}
	if me.ProcName != "" {
		e.More = append(e.More, me.ProcName)
	}
	if me.LineNo > 1 {
		e.More = append(e.More, "line "+strconv.Itoa(int(me.LineNo)))
	}
	return e
}
