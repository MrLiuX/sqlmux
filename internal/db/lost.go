package db

import (
	"context"
	"errors"
	"io"
	"net"
	"syscall"
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
	var ne net.Error
	return errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, net.ErrClosed) ||
		errors.Is(err, syscall.EPIPE) || errors.Is(err, syscall.ECONNRESET) || errors.As(err, &ne)
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
