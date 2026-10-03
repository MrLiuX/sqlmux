//go:build integration

package mysql

import (
	"os"
	"testing"
)

// IntegrationDSN is the seeded database from docker-compose.yml that the
// MySQL integration tests run against (SQLMUX_TEST_MYSQL); unset, the
// test skips.
func IntegrationDSN(t *testing.T) string {
	t.Helper()
	dsn := os.Getenv("SQLMUX_TEST_MYSQL")
	if dsn == "" {
		t.Skip("SQLMUX_TEST_MYSQL is not set")
	}
	return dsn
}
