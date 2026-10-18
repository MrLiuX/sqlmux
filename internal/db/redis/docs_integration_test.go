//go:build integration

package redis

import (
	"context"
	"strings"
	"testing"
)

// What the server's COMMAND DOCS says of SET, ZADD, XADD and CONFIG GET is the line
// the signature shows, and a position in it is the argument's (§9.8).
func TestLoadDocs(t *testing.T) {
	docs, err := LoadDocs(context.Background(), connectTo(t, IntegrationDSN(t)))
	if err != nil || len(docs) < 200 {
		t.Fatalf("%d commands, %v", len(docs), err)
	}
	line := func(name string) string { return strings.Join(docs[name].Fields(name), " ") }
	for name, want := range map[string]string{
		"set":  "SET key value [NX | XX", // newer servers add IFEQ and the like after XX; the start is the same
		"get":  "GET key",
		"hset": "HSET key field value …",
		"del":  "DEL key …",
	} {
		if got := line(name); !strings.HasPrefix(got, want) || name != "set" && got != want {
			t.Errorf("%s: %q", name, got)
		}
	}
	if zadd := line("zadd"); !strings.HasPrefix(zadd, "ZADD key [NX | XX] [GT | LT] [CH] [INCR] score member") {
		t.Errorf("zadd: %q", zadd)
	}
	if xadd := line("xadd"); !strings.HasPrefix(xadd, "XADD key [NOMKSTREAM] [") || !strings.Contains(xadd, "field value …") {
		t.Errorf("xadd: %q", xadd)
	}
	cfg, ok := docs["config"].Subs["get"]
	if !ok || strings.Join(cfg.Fields("config get"), " ") != "CONFIG GET parameter …" || cfg.Summary == "" {
		t.Errorf("config get: %+v", cfg)
	}
	set := docs["set"]
	if p := set.At([]string{"k", "v", "EX"}); p.Field != 5 || p.Leaf == nil || p.Leaf.Name != "seconds" {
		t.Errorf("SET k v EX: %+v", p)
	}
	if p := docs["zadd"].At([]string{"k", "nx", "1", "a"}); p.Leaf == nil || p.Leaf.Name != "score" {
		t.Errorf("ZADD k nx 1 a: %+v", p)
	}
	if p := docs["xadd"].At([]string{"s", "*"}); p.Leaf == nil || p.Leaf.Name != "field" {
		t.Errorf("XADD s *: %+v", p)
	}
}
