//go:build integration

package db

import (
	"io"
	"net"
	"sync"
	"sync/atomic"
	"testing"
)

// Proxy forwards TCP to a target, as a tsh proxy does, and drops what goes
// through it on demand, for tests of what a lost connection does (F8.19).
type Proxy struct {
	ln     net.Listener
	mu     sync.Mutex
	pairs  [][2]net.Conn
	refuse atomic.Bool
}

// NewProxy listens on a port of its own, until the test ends, forwarding to
// target (host:port).
func NewProxy(t *testing.T, target string) *Proxy {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	p := &Proxy{ln: ln}
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			up, err := net.Dial("tcp", target)
			if err != nil || p.refuse.Load() {
				c.Close()
				if up != nil {
					up.Close()
				}
				continue
			}
			p.mu.Lock()
			p.pairs = append(p.pairs, [2]net.Conn{c, up})
			p.mu.Unlock()
			go func() { _, _ = io.Copy(up, c); up.Close() }()
			go func() { _, _ = io.Copy(c, up); c.Close() }()
		}
	}()
	t.Cleanup(func() { ln.Close(); p.Cut(false) })
	return p
}

// Addr is where to connect, host:port.
func (p *Proxy) Addr() string { return p.ln.Addr().String() }

// Cut ends every connection through it, with an RST or a FIN: tsh dies
// either way.
func (p *Proxy) Cut(rst bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, pr := range p.pairs {
		if rst {
			_ = pr[0].(*net.TCPConn).SetLinger(0)
		}
		pr[0].Close()
		pr[1].Close()
	}
	p.pairs = nil
}

// Refuse makes it close new connections at once: a CancelRequest that
// cannot get through.
func (p *Proxy) Refuse(on bool) { p.refuse.Store(on) }
