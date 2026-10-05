//go:build integration

package postgres

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// IntegrationDSN is the seeded database from docker-compose.yml that every
// integration test runs against (SQLMUX_TEST_PG); unset, the test skips.
// It blanks the user's PG* variables and ~/.pgpass for the test, so none of
// them can connect in place of what the test sets up.
func IntegrationDSN(t *testing.T) string {
	t.Helper()
	dsn := os.Getenv("SQLMUX_TEST_PG")
	if dsn == "" {
		t.Skip("SQLMUX_TEST_PG is not set")
	}
	for _, kv := range os.Environ() {
		if k, _, _ := strings.Cut(kv, "="); strings.HasPrefix(k, "PG") {
			t.Setenv(k, "") // pgconn reads an empty variable as unset
		}
	}
	t.Setenv("PGPASSFILE", filepath.Join(t.TempDir(), "none")) // not ~/.pgpass
	return dsn
}

// OwnDB is a database of the test's own on the test instance, seeded as
// the shared one is and dropped when the test ends: for tests that write
// (AGENTS.md「集成测试环境」). It returns its DSN.
func OwnDB(t *testing.T) string {
	t.Helper()
	ctx := context.Background()
	admin, err := Connect(ctx, IntegrationDSN(t), "", false)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { admin.Close() })
	name := fmt.Sprintf("sqlmux_worker_%d_%s", os.Getpid(), strings.ToLower(strings.ReplaceAll(t.Name(), "/", "_")))
	drop := func() { _, _ = admin.Query(ctx, "drop database if exists "+name+" with (force)") }
	drop()
	if _, err := admin.Query(ctx, "create database "+name); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(drop)
	u, err := url.Parse(IntegrationDSN(t))
	if err != nil {
		t.Fatal(err)
	}
	u.Path = "/" + name
	_, here, _, _ := runtime.Caller(0) // the seed by this file's path: tests of other packages call it
	seed, err := os.ReadFile(filepath.Join(filepath.Dir(here), "../../../testdata/seed/pg.sql"))
	if err != nil {
		t.Fatal(err)
	}
	c, err := Connect(ctx, u.String(), "", false)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if _, err := c.Exec(ctx, string(seed), 0); err != nil {
		t.Fatal(err)
	}
	return u.String()
}
