//go:build !windows

package main

import "testing"

// blockRename is the windows test helper of the name; a directory chmod is
// what blocks there, in the test that calls this.
func blockRename(t *testing.T, dir string) {}
