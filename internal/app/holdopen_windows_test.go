//go:build windows

package app

import (
	"path/filepath"
	"testing"

	"golang.org/x/sys/windows"
)

// holdOpen keeps the file at path open without FILE_SHARE_DELETE: os.Remove
// of it fails while it is, windows's undeletable file.
func holdOpen(t *testing.T, path string) {
	t.Helper()
	p, err := windows.UTF16PtrFromString(filepath.Clean(path))
	if err != nil {
		t.Fatal(err)
	}
	h, err := windows.CreateFile(p, windows.GENERIC_READ, 0, nil, windows.OPEN_EXISTING, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = windows.CloseHandle(h) })
}
