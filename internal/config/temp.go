package config

import (
	"cmp"
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"slices"
	"strings"

	"github.com/kackerx/sqlmux/internal/i18n"
)

// IsTarget is whether arg, what the command line gives, is a file or a URL
// to open, not the name of a connection: it has a /, or a . that no connection
// of connections.toml, which are known, is called so (F14.1).
func IsTarget(arg string, known []string) bool {
	return strings.Contains(arg, "/") || strings.Contains(arg, ".") && !slices.Contains(known, arg)
}

// TempConnection is the connection a file or a URL on the command line, or
// DATABASE_URL, stands for (F14.1): postgres:// or mysql://, else a SQLite
// file. It is nowhere written, not connections.toml, not the keychain: the
// password a URL holds stays in it. Its name is the file's without its
// extension, or <database>@<host>; there is no / in it, which the names that
// state.json and the palette's ids go by cannot have.
func TempConnection(target string) (Connection, error) {
	scheme, _, isURL := strings.Cut(target, "://")
	if !isURL {
		path, _, _ := strings.Cut(target, "?")
		base := filepath.Base(path)
		return Connection{Name: strings.TrimSuffix(base, filepath.Ext(base)), Engine: "sqlite", DSN: target}, nil
	}
	engine, ok := map[string]string{"postgres": "postgres", "postgresql": "postgres", "mysql": "mysql"}[scheme]
	if !ok {
		return Connection{}, fmt.Errorf(i18n.T("不认识的 URL %s://：只有 postgres://、mysql:// 和 SQLite 的文件"), scheme)
	}
	u, err := url.Parse(target)
	if ue := (*url.Error)(nil); errors.As(err, &ue) {
		return Connection{}, fmt.Errorf("%s: %w", scheme, ue.Err) // not the URL: it holds the password
	}
	database, host := strings.TrimPrefix(u.Path, "/"), u.Hostname()
	name := strings.Join(slices.DeleteFunc([]string{database, host}, func(s string) bool { return s == "" }), "@")
	return Connection{Name: strings.ReplaceAll(cmp.Or(name, engine), "/", "_"), Engine: engine, DSN: target}, nil
}
