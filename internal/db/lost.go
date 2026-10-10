package db

import (
	"context"
	"errors"
	"io"
	"net"

	"github.com/kackerx/sqlmux/internal/i18n"
)

// Lost says whether a request's error is a lost connection, and what the
// server may have done with the request first (§8.2「断线重连」).
type Lost int

const (
	NotLost    Lost = iota // a SQL error, or a cancel: the connection is fine
	LostUnsent             // gone before anything was sent: any request may run again
	LostSent               // gone with the request sent: the server may have run it
)

// NetworkError is whether err is the network's, which engines have in
// common: a closed or reset connection, an EOF where a reply was due, a
// timeout. A cancel of the request's ctx is not.
func NetworkError(err error) bool {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	var ne net.Error // an OpError: a closed, reset or broken connection, a timeout
	return errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) || errors.As(err, &ne)
}

// Event is what a Worker tells the app of a redial (Worker.Notify): it
// failed (Err), or it went through, with the transaction it had on the
// connection lost (TxLost) and the request that found it run again or not
// (Rerun: not, the user has to).
type Event struct {
	Err    error
	TxLost bool
	Rerun  bool
}

// closer is what a Conn may add to tell it is closed already, as PG's
// is once the server has dropped it or a cancel took too long (§8.3).
type closer interface{ Closed() bool }

// TxLost is the error of a request that may write and did not run, or not
// again: the connection it was to run on lost its transaction, and on the
// new one it would run alone, a write the user did not mean to commit by
// itself. Err is what the failed request got, nil when it was not run at
// all, kept apart (no Unwrap: an error bar shows the server's FATAL in its
// place); Sent whether it had gone to the server then, a COMMIT of which
// may have taken (§8.2「断线重连」).
type TxLost struct {
	Err  error
	Sent bool
}

func (e TxLost) Error() string { return i18n.T("连接断开，事务已丢失，这条没有执行") }

// What a COMMIT reports when its transaction is lost (§8.2「断线重连」): not
// committed, or, if it had gone to the server, not known to be.
var (
	ErrNotCommitted  = msgError(i18n.N("事务已丢失，没有提交"))
	ErrCommitUnknown = msgError(i18n.N("连接断开，提交结果未知：请先查一下数据"))
)
