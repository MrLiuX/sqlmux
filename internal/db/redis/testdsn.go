//go:build integration

package redis

import (
	"context"
	"math/rand/v2"
	"net/url"
	"os"
	"strconv"
	"testing"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

// IntegrationDSN is the seeded server of docker-compose.yml, db0, that every
// integration test reads (SQLMUX_TEST_REDIS); unset, the test skips. Never write
// to it: OwnDB.
func IntegrationDSN(t *testing.T) string {
	t.Helper()
	dsn := os.Getenv("SQLMUX_TEST_REDIS")
	if dsn == "" {
		t.Skip("SQLMUX_TEST_REDIS is not set")
	}
	return dsn
}

// OwnDB is an empty db of the test's own, from 6 to 15 (the seed has 0 and 1),
// claimed by a key that no other test takes it by, and flushed when the test
// ends: for tests that write (AGENTS.md「集成测试环境」). It returns its DSN.
func OwnDB(t *testing.T) string {
	t.Helper()
	dsn := IntegrationDSN(t)
	u, err := url.Parse(dsn)
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range rand.Perm(10) {
		n += 6
		u.Path = "/" + strconv.Itoa(n)
		opt, err := goredis.ParseURL(u.String())
		if err != nil {
			t.Fatal(err)
		}
		cl := goredis.NewClient(opt)
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		ok, err := cl.SetNX(ctx, "sqlmux:test-claim", t.Name(), 10*time.Minute).Result()
		cancel()
		if err != nil {
			cl.Close()
			t.Fatal(err)
		}
		if ok {
			t.Cleanup(func() {
				defer cl.Close()
				cl.FlushDB(context.Background())
			})
			return u.String()
		}
		cl.Close()
	}
	t.Fatal("no db of 6 to 15 is free")
	return ""
}
