package db

import (
	"context"
	"errors"
	"slices"
	"testing"
)

// savepointConn is a Conn in a transaction, telling what it was asked: the
// statements a save's savepoint asks for, and none past them (§10.3). The
// one is a TxSaver, whose release is none; the other speaks the default
// words.
type savepointConn struct {
	asked []string
}

func (c *savepointConn) Exec(context.Context, string, int) ([]Result, error) { return nil, nil }
func (c *savepointConn) Query(_ context.Context, sql string, _ ...Val) (Result, error) {
	c.asked = append(c.asked, sql)
	return Result{Rows: [][]Val{{{S: "1"}}}}, nil
}
func (c *savepointConn) Close() error   { return nil }
func (c *savepointConn) TxStatus() byte { return 'T' }

func (c *savepointConn) Begin() string { return "begin transaction" }
func (c *savepointConn) Savepoint(name string) (set, rollback, release string) {
	return "save transaction " + name, "rollback transaction " + name, ""
}

// defaultConn is one in a transaction that is no TxSaver: the db package's
// own words.
type defaultConn struct{ asked []string }

func (c *defaultConn) Exec(context.Context, string, int) ([]Result, error) { return nil, nil }
func (c *defaultConn) Query(_ context.Context, sql string, _ ...Val) (Result, error) {
	c.asked = append(c.asked, sql)
	return Result{Rows: [][]Val{{{S: "1"}}}}, nil
}
func (c *defaultConn) Close() error   { return nil }
func (c *defaultConn) TxStatus() byte { return 'T' }

var errSavepoint = errors.New("the save's own")

// Guarded asks for the savepoint, and for its rollback when the save fails;
// a TxSaver's release being none, no third statement is asked of it, and
// the default words keep theirs (F18.6).
func TestGuardedSavepoint(t *testing.T) {
	c := &savepointConn{}
	if err := Guarded(context.Background(), c, "sqlmux_save", func() error { return nil }); err != nil {
		t.Fatal(err)
	}
	if want := []string{"save transaction sqlmux_save"}; !slices.Equal(c.asked, want) {
		t.Errorf("a save that works, asked %v, want %v", c.asked, want)
	}
	c = &savepointConn{}
	if err := Guarded(context.Background(), c, "sqlmux_save", func() error { return errSavepoint }); !errors.Is(err, errSavepoint) {
		t.Fatalf("a save that fails: %v", err)
	}
	if want := []string{"save transaction sqlmux_save", "rollback transaction sqlmux_save"}; !slices.Equal(c.asked, want) {
		t.Errorf("a save that fails, asked %v, want %v", c.asked, want)
	}
	// the default words: a release of their own, success and failure alike
	d := &defaultConn{}
	if err := Guarded(context.Background(), d, "sqlmux_save", func() error { return nil }); err != nil {
		t.Fatal(err)
	}
	if want := []string{"savepoint sqlmux_save", "release savepoint sqlmux_save"}; !slices.Equal(d.asked, want) {
		t.Errorf("the default words, asked %v, want %v", d.asked, want)
	}
	d = &defaultConn{}
	if err := Guarded(context.Background(), d, "sqlmux_save", func() error { return errSavepoint }); !errors.Is(err, errSavepoint) {
		t.Fatalf("a save that fails: %v", err)
	}
	if want := []string{"savepoint sqlmux_save", "rollback to savepoint sqlmux_save", "release savepoint sqlmux_save"}; !slices.Equal(d.asked, want) {
		t.Errorf("the default words, asked %v, want %v", d.asked, want)
	}
}
