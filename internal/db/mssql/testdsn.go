//go:build integration

package mssql

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// IntegrationDSN is the seeded database from docker-compose.yml that the
// SQL Server integration tests run against (SQLMUX_TEST_MSSQL); unset, the
// test skips.
func IntegrationDSN(t *testing.T) string {
	t.Helper()
	dsn := os.Getenv("SQLMUX_TEST_MSSQL")
	if dsn == "" {
		t.Skip("SQLMUX_TEST_MSSQL is not set")
	}
	return dsn
}

// OwnDB is a database of the test's own on the test server, seeded as the
// shared one is but for agentable and the functions, and dropped when the
// test ends: for tests that write (AGENTS.md「集成测试环境」). It returns its DSN.
func OwnDB(t *testing.T) string {
	t.Helper()
	ctx := context.Background()
	admin, _, err := Connect(ctx, IntegrationDSN(t), "", false)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { admin.Close() })
	name := fmt.Sprintf("sqlmux_worker_%d_%s", os.Getpid(), strings.ToLower(strings.ReplaceAll(t.Name(), "/", "_")))
	drop := func() { _, _ = admin.Query(ctx, "drop database if exists ["+name+"]") }
	drop()
	if _, err := admin.Query(ctx, "create database ["+name+"]"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(drop)
	dsn, err := WithDatabase(IntegrationDSN(t), name)
	if err != nil {
		t.Fatal(err)
	}
	_, here, _, _ := runtime.Caller(0) // the seed by this file's path: tests of other packages call it
	seedFile, err := os.ReadFile(filepath.Join(filepath.Dir(here), "../../../testdata/seed/mssql.sql"))
	if err != nil {
		t.Fatal(err)
	}
	// the main database's part only: agentable's objects take their own
	// database, and the shared seed's second half is theirs
	seed := string(seedFile)
	if i := strings.Index(seed, "-- the second database"); i >= 0 {
		seed = seed[:i]
	}
	c, _, err := Connect(ctx, dsn, "", false)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	for _, batch := range strings.Split(seed, "\ngo\n") { // a create view, a create function, of their own batch (§9.2)
		for _, s := range strings.Split(batch, ";\n") {
			s = strings.TrimSpace(s)
			// the session's quoted_identifier (§9.1), and the use of the seed's
			// first line: OwnDB's database is the one the dsn names
			if s == "" || strings.HasPrefix(s, "set quoted_identifier") || strings.HasPrefix(s, "use ") {
				continue
			}
			if _, err := c.Exec(ctx, s, 0); err != nil {
				t.Fatalf("%v\n%s", err, s)
			}
		}
	}
	return dsn
}
