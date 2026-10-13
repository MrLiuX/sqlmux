package postgres

import (
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

// pgconn reads the database of a DSN of either form as WithDatabase means it,
// whatever the DSN said before: a URL's path, and its query's dbname and
// database, which pgx reads after the path and so win over it; the keywords
// form's later dbname (F15.1).
func TestWithDatabaseParses(t *testing.T) {
	for _, dsn := range []string{
		"postgres://u@h/base",
		"postgres://u@h/base?dbname=base",
		"postgres://u@h/base?database=base&sslmode=disable",
		"postgresql://u@h:5433/base?sslmode=disable&dbname=base",
		"postgres://u@h/ba%2Fse?dbname=base",
		"postgres://u@h?dbname=base",
		"host=h user=u dbname=base",
		"host=h user=u",
	} {
		for _, name := range []string{"alpha", "a b?c", `it's`} {
			with, err := WithDatabase(dsn, name)
			if err != nil {
				t.Fatal(err)
			}
			cfg, err := pgconn.ParseConfig(with)
			if err != nil || cfg.Database != name || cfg.Host != "h" || cfg.User != "u" {
				t.Errorf("%q with %q is %q: database %q, host %q, user %q, %v", dsn, name, with, cfg.Database, cfg.Host, cfg.User, err)
			}
		}
	}
}
