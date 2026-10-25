package mssql

import (
	"context"
	"fmt"
	"testing"
	"time"
)

// The names in brackets, the arguments the driver's named parameters, the
// conditions of a save as §10.3 has them (F18.6).
func TestQuoteArgSameEq(t *testing.T) {
	e := Engine{}
	if q := e.Quote("dbo", "t_user"); q != "[dbo].[t_user]" {
		t.Errorf("quote: %q", q)
	}
	if q := e.Quote(`a]b`); q != "[a]]b]" {
		t.Errorf("a ] in the name: %q", q)
	}
	if a := e.Arg(2); a != "@p2" {
		t.Errorf("arg: %q", a)
	}
	if eq := e.Eq("[id]", "@p1"); eq != "[id] = @p1" {
		t.Errorf("eq: %q", eq)
	}
	// Same is the column's text as the server's try_convert has it, the form
	// the values were read in; a NULL was read, IS NULL asks for it; a bit's
	// third arm, the grid's true and false and no other value any case for
	if same := e.Same("[amount]", "@p2"); same != "try_convert(nvarchar(max), [amount], 126) = @p2 or try_convert(nvarchar(max), [amount]) = @p2 or try_convert(nvarchar(max), [amount], 126) = case @p2 when 'true' then '1' when 'false' then '0' end" {
		t.Errorf("same: %q", same)
	}
}

// The DSNs: the URL and the keyword form, a password winning over the DSN's,
// a database named over the one the DSN names (§8.1, M15 F15.1).
func TestConfigAndWithDatabase(t *testing.T) {
	cfg, err := Config("sqlserver://sa:pw@localhost:51433?database=sqlmux&TrustServerCertificate=true", "other")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.User != "sa" || cfg.Password != "other" || cfg.Database != "sqlmux" || cfg.Port != 51433 {
		t.Errorf("the URL: %+v", cfg)
	}
	if _, err := Config("server=localhost,51433;user id=sa;database=sqlmux", ""); err != nil {
		t.Errorf("the keyword form: %v", err)
	}
	dsn, err := WithDatabase("sqlserver://sa:pw@localhost:51433?database=sqlmux", "other")
	if err != nil || dsn != "sqlserver://sa:pw@localhost:51433?database=other" {
		t.Errorf("with database: %q, %v", dsn, err)
	}
	if dsn, err = WithDatabase("server=localhost;user id=sa", "other"); err != nil || dsn != "server=localhost;user id=sa;database=other" {
		t.Errorf("the keyword form: %q, %v", dsn, err)
	}
}

// A value the driver decoded, as the type's text has it (§8.1, F18.6): the
// datetime family's fixed-scale T form, a bit as true or false, the GUID
// text the server writes, bytes by their size.
func TestCellOf(t *testing.T) {
	for _, c := range []struct {
		typ  fakeType
		v    any
		want string
	}{
		{fakeType{"bigint", 0}, int64(-3), "-3"},
		{fakeType{"bit", 0}, true, "true"},
		{fakeType{"bit", 0}, false, "false"},
		{fakeType{"nvarchar", 0}, "海", "海"},
		{fakeType{"datetime2", 3}, time.Date(2026, 1, 2, 3, 4, 5, 123000000, time.UTC), "2026-01-02T03:04:05.123"},
		{fakeType{"datetime2", 7}, time.Date(2026, 1, 2, 3, 4, 5, 12345000, time.UTC), "2026-01-02T03:04:05.0123450"},
		{fakeType{"datetime", 0}, time.Date(2026, 1, 2, 3, 4, 5, 123000000, time.UTC), "2026-01-02T03:04:05.123"},
		{fakeType{"date", 0}, time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC), "2026-01-02"},
		{fakeType{"time", 4}, time.Date(1, 1, 1, 3, 4, 5, 123400000, time.UTC), "03:04:05.1234"},
		{fakeType{"datetimeoffset", 7}, time.Date(2026, 1, 2, 3, 4, 5, 12345000, mustZone("+08:00")), "2026-01-02T03:04:05.0123450+08:00"},
		{fakeType{"uniqueidentifier", 0}, []byte{255, 25, 150, 111, 134, 139, 17, 208, 180, 45, 0, 192, 79, 201, 100, 255}, "6F9619FF-8B86-D011-B42D-00C04FC964FF"},
		{fakeType{"varbinary", 0}, []byte{0xAB, 0xCD}, "<binary 2 bytes>"},
		{fakeType{"varchar", 0}, []byte("plain"), "plain"},
	} {
		if got := cellText(c.v, c.typ); got.S != c.want || got.Null {
			t.Errorf("%s of %v = %+v, want %q", c.typ.name, c.v, got, c.want)
		}
	}
	if v := cellText(nil, fakeType{"int", 0}); !v.Null {
		t.Errorf("nil: %+v", v)
	}
}

// The bytes of an export, as the statement takes them back (F12.4).
func TestHexCellOf(t *testing.T) {
	if v := cellHexText([]byte{0xAB, 0xCD}, fakeType{"varbinary", 0}); v.S != "0xabcd" {
		t.Errorf("varbinary: %q", v.S)
	}
	if v := cellHexText(int64(3), fakeType{"int", 0}); v.S != "3" {
		t.Errorf("int: %q", v.S)
	}
}

// The transaction the words tell, and the words a save asks of the Conn
// (§10.3, F18.6).
func TestTxWords(t *testing.T) {
	c := &Conn{}
	if c.Begin() != "begin transaction" {
		t.Errorf("begin: %q", c.Begin())
	}
	set, to, release := c.Savepoint("sqlmux_save")
	if set != "save transaction sqlmux_save" || to != "rollback transaction sqlmux_save" || release != "" {
		t.Errorf("savepoint: %q, %q, %q", set, to, release)
	}
}

// The server's error, as the log and an error bar show it (§11, F18.6).
func TestErrorOf(t *testing.T) {
	if e := (Engine{}).ErrorOf(context.DeadlineExceeded); e.Severity != "ERROR" || e.Code != "" {
		t.Errorf("an error of no server's: %+v", e)
	}
}

// fakeType is a column type by name and scale, what cellOf's text keeps.
type fakeType struct {
	name  string
	scale int64
}

func (f fakeType) DatabaseTypeName() string { return f.name }
func (f fakeType) DecimalSize() (int64, int64, bool) {
	return 0, f.scale, f.scale > 0
}

// mustZone is a fixed offset zone, for the datetimeoffset's text.
func mustZone(name string) *time.Location {
	var h, m int
	_, _ = fmt.Sscanf(name, "+%d:%d", &h, &m)
	return time.FixedZone(name, h*3600+m*60)
}
