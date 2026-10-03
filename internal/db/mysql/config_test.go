package mysql

import (
	"slices"
	"strings"
	"testing"
)

// A URL or the driver's own DSN; whatever either says, values stay the
// server's text, columns their names, a statement one (§8.1). A URL that
// does not parse says why, its password left out.
func TestConfig(t *testing.T) {
	u, err := Config("mysql://u:p@db.example/x%2Fy?tls=skip-verify&parseTime=true&columnsWithAlias=true&multiStatements=true", "pw")
	if err != nil || u.Addr != "db.example:3306" || u.User != "u" || u.Passwd != "pw" || u.DBName != "x/y" || u.TLSConfig != "skip-verify" ||
		u.ParseTime || u.ColumnsWithAlias || u.MultiStatements || !u.InterpolateParams || !u.ClientFoundRows {
		t.Errorf("a URL: %+v %v", u, err)
	}
	if d, err := Config("u:p@tcp(h:1)/x?parseTime=true", ""); err != nil || d.Passwd != "p" || d.Addr != "h:1" || d.ParseTime {
		t.Errorf("the driver's: %+v %v", d, err)
	}
	if _, err := Config("mysql://u:secret@h:x/db", ""); err == nil || strings.Contains(err.Error(), "secret") {
		t.Errorf("a bad URL: %v", err)
	}
}

// COLUMN_TYPE doubles a quote and a backslash in an enum's labels.
func TestEnumOf(t *testing.T) {
	if got := enumOf(`enum('a','it''s','a\\b','')`); !slices.Equal(got, []string{"a", "it's", `a\b`, ""}) {
		t.Errorf("%q", got)
	}
	if enumOf("set('a')") != nil || enumOf("varchar(8)") != nil {
		t.Error("no enum")
	}
}

// A BIT's bytes are a number, big end first.
func TestBits(t *testing.T) {
	for want, v := range map[string][]byte{"0": {0}, "1": {1}, "5": {5}, "256": {1, 0}, "18446744073709551615": {255, 255, 255, 255, 255, 255, 255, 255}} {
		if got := bits(v); got != want {
			t.Errorf("%v: %s, want %s", v, got, want)
		}
	}
}
