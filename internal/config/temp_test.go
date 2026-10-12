package config

import (
	"strings"
	"testing"
)

func TestTempConnection(t *testing.T) {
	for target, want := range map[string]Connection{
		"./x.db":                          {Name: "x", Engine: "sqlite", DSN: "./x.db"},
		"/tmp/data/my.app.sqlite?mode=ro": {Name: "my.app", Engine: "sqlite", DSN: "/tmp/data/my.app.sqlite?mode=ro"},
		"~/notes":                         {Name: "notes", Engine: "sqlite", DSN: "~/notes"},
		"postgres://u:pw@db.example:5432/shop?x=1": {Name: "shop@db.example", Engine: "postgres", DSN: "postgres://u:pw@db.example:5432/shop?x=1"},
		"postgresql://localhost/app":               {Name: "app@localhost", Engine: "postgres", DSN: "postgresql://localhost/app"},
		"mysql://root@127.0.0.1:3306/":             {Name: "127.0.0.1", Engine: "mysql", DSN: "mysql://root@127.0.0.1:3306/"},
		"mysql:///tmp/sock":                        {Name: "tmp_sock", Engine: "mysql", DSN: "mysql:///tmp/sock"},
	} {
		got, err := TempConnection(target)
		if err != nil || got != want {
			t.Errorf("%s: %+v %v, want %+v", target, got, err, want)
		}
		if f, p := got.Check(nil); p != "" {
			t.Errorf("%s: %s %s", target, f, p)
		}
	}
	if _, err := TempConnection("oracle://x/y"); err == nil {
		t.Error("an engine there is not")
	}
	// the password in a URL that does not parse is not in the error
	if _, err := TempConnection("postgres://u:secret@h:99999999999999/db%zz"); err != nil && strings.Contains(err.Error(), "secret") {
		t.Errorf("the error says the password: %v", err)
	}
}

func TestIsTarget(t *testing.T) {
	known := []string{"prod", "prod.eu"}
	for arg, want := range map[string]bool{"prod": false, "prod.eu": false, "x.db": true, "./prod": true, "a/b": true, "postgres://h/d": true, "other": false} {
		if got := IsTarget(arg, known); got != want {
			t.Errorf("%q: %v", arg, got)
		}
	}
}
