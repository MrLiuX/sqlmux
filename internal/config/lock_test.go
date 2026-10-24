package config

import (
	"bufio"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// The state.json lock excludes: while one process holds it another's
// flockExclusive blocks, and a holder that dies releases it (§14; the
// mutation check of F18.1 — shared flags or a missing lock make the second
// lock go through).
func TestStateLockExcludes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	if p := os.Getenv("SQLMUX_LOCK_HELPER"); p != "" { // the child: hold the lock and stay
		f, err := os.OpenFile(p, os.O_CREATE|os.O_RDWR, 0o600)
		if err != nil {
			t.Fatal(err)
		}
		if err := flockExclusive(f); err != nil {
			t.Fatal(err)
		}
		os.Stdout.WriteString("ready\n")
		select {}
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if err := flockExclusive(f); err != nil {
		t.Fatal(err)
	}
	f.Close() // the lock dies with it: the child starts from a free file

	cmd := exec.Command(os.Args[0], "-test.run=^TestStateLockExcludes$", "-test.timeout=2m")
	cmd.Env = append(os.Environ(), "SQLMUX_LOCK_HELPER="+path)
	out, _ := cmd.StdoutPipe()
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	if line, _ := bufio.NewReader(out).ReadString('\n'); line != "ready\n" {
		t.Fatalf("the child said %q", line)
	}
	done := make(chan error, 1)
	go func() { // the parent's own take: it has to wait the child out
		f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
		if err != nil {
			done <- err
			return
		}
		done <- flockExclusive(f)
		_ = funlockExclusive(f)
		f.Close()
	}()
	select {
	case err := <-done:
		t.Fatalf("a second holder slipped in: %v", err)
	case <-time.After(300 * time.Millisecond):
	}
	if err := cmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	_ = cmd.Wait()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the dead holder kept the lock")
	}
}
