//go:build !windows

package app

import "testing"

// holdOpen is the windows test helper of the name; a directory chmod is
// what blocks a delete in the tests that call this.
func holdOpen(t *testing.T, path string) {}
