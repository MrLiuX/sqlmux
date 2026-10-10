package config

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// legacyQuery is the name a query file had while queries were called
// consoles (F10.11): console_<n>.
var legacyQuery = regexp.MustCompile(`^console_(\d+)$`)

// currentName is name as the queries are called now: console_<n> is
// query_<n>, any other name, a renamed file's, is its own.
func currentName(name string) string {
	if m := legacyQuery.FindStringSubmatch(name); m != nil {
		return "query_" + m[1]
	}
	return name
}

// MigrateConsoles moves what a start of before F10.11 left to where the
// queries are, once: consoles/ becomes queries/, console_<n>.sql in it
// query_<n>.sql, a file that is renamed staying as it is. Nothing is done
// when queries/ is there, or when a file of the new name is: the user's own
// are not written over.
func MigrateConsoles() error {
	dir := filepath.Join(DataDir(), "queries")
	if _, err := os.Stat(dir); err == nil {
		return nil
	}
	if err := os.Rename(filepath.Join(DataDir(), "consoles"), dir); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return err
	}
	files, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	var errs []error
	for _, f := range files {
		name, ok := strings.CutSuffix(f.Name(), ".sql")
		if !ok {
			continue
		}
		to := filepath.Join(dir, currentName(name)+".sql")
		if _, err := os.Stat(to); err == nil {
			continue
		}
		errs = append(errs, os.Rename(filepath.Join(dir, f.Name()), to))
	}
	return errors.Join(errs...)
}
