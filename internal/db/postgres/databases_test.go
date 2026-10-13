//go:build integration

package postgres

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// madeDB creates database name (a quoted identifier's text) on the test
// instance, with options, dropped when the test ends.
func madeDB(t *testing.T, admin *Conn, name, options string) {
	t.Helper()
	ctx := context.Background()
	quoted := Engine{}.Quote(name)
	drop := func() { _, _ = admin.Query(ctx, "drop database if exists "+quoted+" with (force)") }
	drop()
	if _, err := admin.Query(ctx, "create database "+quoted+" "+options); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(drop)
}

// The list is the databases one may connect to: a superuser sees all but the
// templates, a role without CONNECT on one, or one that takes no connections,
// does not see it (§8.4).
func TestDatabases(t *testing.T) {
	ctx := context.Background()
	admin, _, err := Connect(ctx, IntegrationDSN(t), "", false)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { admin.Close() }) // after the drops
	suffix := strconv.Itoa(os.Getpid())
	closed, locked, role := "sqlmux_f151_closed_"+suffix, "sqlmux_f151_locked_"+suffix, "sqlmux_f151_role_"+suffix
	madeDB(t, admin, closed, "allow_connections false")
	madeDB(t, admin, locked, "")
	for _, q := range []string{"revoke connect on database " + locked + " from public", "create role " + role + " login password 'x'"} {
		if _, err := admin.Query(ctx, q); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() { _, _ = admin.Query(ctx, "drop role if exists "+role) })

	got, err := pg.Databases(ctx, admin)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(got, "sqlmux") || !slices.Contains(got, locked) || slices.Contains(got, closed) || slices.ContainsFunc(got, func(n string) bool { return strings.HasPrefix(n, "template") }) {
		t.Fatalf("a superuser's databases %v", got)
	}
	if !slices.IsSorted(got) {
		t.Errorf("not by name: %v", got)
	}

	u, _ := url.Parse(IntegrationDSN(t))
	u.User = url.UserPassword(role, "x")
	c, _, err := Connect(ctx, u.String(), "", false)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	got, err = pg.Databases(ctx, c)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(got, "sqlmux") || slices.Contains(got, locked) || slices.Contains(got, closed) {
		t.Fatalf("a role without CONNECT on %s sees %v", locked, got)
	}
}

// A DSN of either form, a URL or keywords, connects to the database WithDatabase
// names, whatever it holds: a quote, a backslash and a blank in a name are one
// name.
func TestWithDatabase(t *testing.T) {
	ctx := context.Background()
	admin, _, err := Connect(ctx, IntegrationDSN(t), "", false)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { admin.Close() }) // after the drops
	u, _ := url.Parse(IntegrationDSN(t))
	pw, _ := u.User.Password()
	keywords := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=sqlmux sslmode=disable", u.Hostname(), u.Port(), u.User.Username(), pw)
	for _, name := range []string{fmt.Sprintf("sqlmux_f151_%d", os.Getpid()), `sqlmux_f151_a'b\c d`} {
		madeDB(t, admin, name, "")
		for _, dsn := range []string{IntegrationDSN(t), keywords} {
			with, err := WithDatabase(dsn, name)
			if err != nil {
				t.Fatal(err)
			}
			c, info, err := Connect(ctx, with, "", true)
			if err != nil {
				t.Fatalf("%q: %v", with, err)
			}
			c.Close()
			if info.Database != name {
				t.Errorf("%q connected to %q", with, info.Database)
			}
		}
	}
}
