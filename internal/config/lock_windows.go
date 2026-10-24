//go:build windows

package config

import (
	"os"

	"golang.org/x/sys/windows"
)

// flockExclusive is the advisory lock lockFile takes, in Windows' own
// shape: a byte-range lock on the file's first byte without
// LOCKFILE_FAIL_IMMEDIATELY, so it waits for its turn like flock LOCK_EX
// does; two writers lock the same span and queue. The lock dies with the
// handle, but funlockExclusive releases it for symmetry with the unix one.
func flockExclusive(f *os.File) error {
	ol := new(windows.Overlapped)
	return windows.LockFileEx(windows.Handle(f.Fd()), windows.LOCKFILE_EXCLUSIVE_LOCK, 0, 1, 0, ol)
}

func funlockExclusive(f *os.File) error {
	ol := new(windows.Overlapped)
	return windows.UnlockFileEx(windows.Handle(f.Fd()), 0, 1, 0, ol)
}
