//go:build integration

package sqlite

import (
	"database/sql"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// OwnDB is a file of the test's own, seeded from testdata/seed/sqlite.sql, for
// the tests of other packages that run on every engine; it is gone when the
// test ends, and its path is the connection's DSN. SQLite needs no server and no
// environment, which the other engines' IntegrationDSN is.
func OwnDB(t *testing.T) string {
	t.Helper()
	_, here, _, _ := runtime.Caller(0) // the seed by this file's path: tests of other packages call it
	seed, err := os.ReadFile(filepath.Join(filepath.Dir(here), "../../../testdata/seed/sqlite.sql"))
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "seed.db")
	d, err := sql.Open("sqlite", path+"?mode=rwc")
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	if _, err := d.Exec(string(seed)); err != nil {
		t.Fatal(err)
	}
	return path
}
