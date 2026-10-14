package db

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// DialFunc is how a connection reaches its server: the dial pgx and the MySQL
// driver take, to addr, "host:port".
type DialFunc func(ctx context.Context, network, addr string) (net.Conn, error)

// Dialer is a DialFunc with a name, which the MySQL driver registers it by.
// Failed is why the last connection it made ended, when it did and with an
// error, as the driver, which tells no more than "bad connection", needs it
// said; nil for a dialer that has none to tell.
type Dialer struct {
	Name   string
	Dial   DialFunc
	Failed func() error
}

type dialKey struct{}

// WithDialer is ctx asking Connect to reach the server by d, not by the
// network itself (F15.2); DialerOf reads it.
func WithDialer(ctx context.Context, d Dialer) context.Context {
	return context.WithValue(ctx, dialKey{}, d)
}

func DialerOf(ctx context.Context) (Dialer, bool) {
	d, ok := ctx.Value(dialKey{}).(Dialer)
	return d, ok
}

// SSHDialer reaches the server through the machine dest ("user@host", the rest
// of it ~/.ssh/config's) as the local ssh does (§8.4「SSH 隧道」, F15.2): each
// dial starts `ssh -W addr dest`, whose stdin and stdout are the connection, so
// there is no local port to find free or to clean up, and the process ends with
// the connection. ssh cannot ask for a password here: BatchMode makes it fail,
// and what it said on stderr is the error (`ssh-add` first).
func SSHDialer(dest string) Dialer {
	var mu sync.Mutex
	var last *sshConn
	d := Dialer{Name: "ssh:" + dest}
	d.Failed = func() error {
		mu.Lock()
		defer mu.Unlock()
		if last == nil {
			return nil
		}
		return last.failure()
	}
	d.Dial = func(_ context.Context, _, addr string) (net.Conn, error) {
		cmd := exec.Command("ssh", "-W", addr, "-o", "BatchMode=yes", "-o", "ExitOnForwardFailure=yes", dest)
		inR, inW, err := os.Pipe()
		if err != nil {
			return nil, err
		}
		outR, outW, err := os.Pipe()
		if err != nil {
			inR.Close()
			inW.Close()
			return nil, err
		}
		stderr := &tail{}
		cmd.Stdin, cmd.Stdout, cmd.Stderr = inR, outW, stderr
		cmd.WaitDelay = 500 * time.Millisecond // a ProxyCommand of ssh's may hold its stderr open after it is gone
		err = cmd.Start()
		inR.Close() // the child's ends
		outW.Close()
		if err != nil {
			inW.Close()
			outR.Close()
			return nil, fmt.Errorf("ssh %s: %w", dest, err)
		}
		c := &sshConn{dest: dest, addr: addr, cmd: cmd, in: inW, out: outR, stderr: stderr, done: make(chan struct{})}
		go func() {
			c.exit = cmd.Wait()
			close(c.done)
		}()
		mu.Lock()
		last = c
		mu.Unlock()
		return c, nil
	}
	return d
}

// sshConn is a net.Conn over an ssh process's stdin and stdout.
type sshConn struct {
	dest    string // the machine ssh goes through
	addr    string // the server, "host:port"
	cmd     *exec.Cmd
	in, out *os.File // our ends of the pipes
	stderr  *tail
	done    chan struct{} // closed when the process has ended, exit its status
	exit    error
	killed  atomic.Bool // Close ended it: that is no failure of ssh's
}

// failure is what ssh said when it failed, nil when it has not, or it ended
// well, or Close ended it.
func (c *sshConn) failure() error {
	if c.killed.Load() {
		return nil
	}
	select {
	case <-c.done:
	case <-time.After(2 * time.Second): // it is on its way out, or it is not out
		return nil
	}
	var ee *exec.ExitError
	if !errors.As(c.exit, &ee) {
		return nil
	}
	msg := strings.TrimSpace(c.stderr.String())
	if msg == "" {
		msg = c.exit.Error()
	}
	return sshFailure("ssh " + c.dest + ": " + msg)
}

// sshFailure is the error of a tunnel that died, one NetworkError knows. It has
// no Unwrap: go-redis unwraps an error of its handshake once, the message with it.
type sshFailure string

func (e sshFailure) Error() string { return string(e) }

// exitError is why the connection ended: what ssh said when it failed, else
// err itself.
func (c *sshConn) exitError(err error) error {
	if f := c.failure(); f != nil {
		return f
	}
	return err
}

func (c *sshConn) Read(p []byte) (int, error) {
	n, err := c.out.Read(p)
	if errors.Is(err, io.EOF) {
		err = c.exitError(err)
	}
	return n, err
}

func (c *sshConn) Write(p []byte) (int, error) {
	n, err := c.in.Write(p)
	if err != nil && !errors.Is(err, os.ErrDeadlineExceeded) {
		err = c.exitError(err)
	}
	return n, err
}

// Close ends the process: nothing of the connection is worth waiting for.
func (c *sshConn) Close() error {
	select {
	case <-c.done: // it has ended by itself: what it said is the error
	default:
		c.killed.Store(true)
	}
	c.in.Close()
	c.out.Close()
	_ = c.cmd.Process.Kill() // gone already is what it is for
	<-c.done
	return nil
}

func (c *sshConn) SetDeadline(t time.Time) error {
	return errors.Join(c.in.SetWriteDeadline(t), c.out.SetReadDeadline(t))
}
func (c *sshConn) SetReadDeadline(t time.Time) error  { return c.out.SetReadDeadline(t) }
func (c *sshConn) SetWriteDeadline(t time.Time) error { return c.in.SetWriteDeadline(t) }
func (c *sshConn) LocalAddr() net.Addr                { return tcpName("ssh") }

// RemoteAddr is the server's, as pgx dials it again from it for a cancel
// request.
func (c *sshConn) RemoteAddr() net.Addr { return tcpName(c.addr) }

// tcpName is a "host:port" that need not be an IP.
type tcpName string

func (tcpName) Network() string  { return "tcp" }
func (a tcpName) String() string { return string(a) }

// tail keeps the last 4 KB written to it: what ssh says on stderr, which is
// short but not bounded.
type tail struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (t *tail) Write(p []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.buf.Write(p)
	if over := t.buf.Len() - 4096; over > 0 {
		t.buf.Next(over)
	}
	return len(p), nil
}

func (t *tail) String() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.buf.String()
}
