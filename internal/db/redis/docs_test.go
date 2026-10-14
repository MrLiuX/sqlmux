package redis

import (
	"slices"
	"strings"
	"testing"
)

func av(name, typ string) Arg              { return Arg{Name: name, Type: typ} }
func pt(name string) Arg                   { return Arg{Name: strings.ToLower(name), Type: "pure-token", Token: name} }
func optl(a Arg) Arg                       { a.Optional = true; return a }
func multi(a Arg) Arg                      { a.Multiple = true; return a }
func oneOf(name string, alts ...Arg) Arg   { return Arg{Name: name, Type: "oneof", Args: alts} }
func blockOf(name string, args ...Arg) Arg { return Arg{Name: name, Type: "block", Args: args} }
func tokened(a Arg, t string) Arg          { a.Token = t; return a }

var (
	setDoc = Doc{Args: []Arg{
		av("key", "key"), av("value", "string"),
		optl(oneOf("condition", pt("NX"), pt("XX"))), optl(pt("GET")),
		optl(oneOf("expiration", tokened(av("seconds", "integer"), "EX"), tokened(av("milliseconds", "integer"), "PX"), pt("KEEPTTL"))),
	}}
	hsetDoc = Doc{Args: []Arg{av("key", "key"), multi(blockOf("data", av("field", "string"), av("value", "string")))}}
	delDoc  = Doc{Args: []Arg{multi(av("key", "key"))}}
	zaddDoc = Doc{Args: []Arg{
		av("key", "key"), optl(oneOf("condition", pt("NX"), pt("XX"))), optl(pt("CH")),
		multi(blockOf("data", av("score", "double"), av("member", "string"))),
	}}
	getDoc = Doc{Args: []Arg{av("key", "key")}}
	// a block that has a token of its own, and one argument with a token
	limitDoc = Doc{Args: []Arg{av("key", "key"), optl(tokened(blockOf("limit", av("offset", "integer"), av("count", "integer")), "LIMIT")), optl(tokened(av("pattern", "string"), "BY"))}}
	// what is given again, then a token of what follows
	storeDoc = Doc{Args: []Arg{multi(av("key", "key")), optl(tokened(av("destination", "key"), "STORE"))}}
	// an optional block with no token of its own, begun by a token inside it
	trimDoc = Doc{Args: []Arg{av("key", "key"), optl(blockOf("trim", oneOf("strategy", pt("MAXLEN"), pt("MINID")), av("threshold", "string"))), av("id", "string")}}
	// optional tokens after what is given again, up to what has to be there
	afterDoc = Doc{Args: []Arg{multi(av("key", "key")), optl(pt("A")), av("x", "string"), optl(pt("B"))}}
	// a token that has to be there
	countDoc = Doc{Args: []Arg{av("key", "key"), tokened(av("count", "integer"), "COUNT")}}
)

// The arguments of a command as the line shows them, and where the next word of a
// line goes among them: what is optional is in [], what is given again has …, a
// oneof is its alternatives, a token is read with its value, a block is repeated as
// a whole (§9.8).
func TestDocSignatureAndPlace(t *testing.T) {
	if got := strings.Join(setDoc.Fields("set"), " "); got != "SET key value [NX | XX] [GET] [EX seconds | PX milliseconds | KEEPTTL]" {
		t.Errorf("SET: %q", got)
	}
	if got := strings.Join(hsetDoc.Fields("hset"), " "); got != "HSET key field value …" {
		t.Errorf("HSET: %q", got)
	}
	if got := strings.Join(limitDoc.Fields("sort"), " "); got != "SORT key [LIMIT offset count] [BY pattern]" {
		t.Errorf("limit: %q", got)
	}
	if got := strings.Join(delDoc.Fields("del"), " "); got != "DEL key …" {
		t.Errorf("DEL: %q", got)
	}
	for _, c := range []struct {
		name   string
		doc    Doc
		words  string
		field  int
		tokens string
		leaf   string
	}{
		{"set, nothing", setDoc, "", 1, "", "key"},
		{"set, a key", setDoc, "k", 2, "", "value"},
		{"set, a value", setDoc, "k v", 3, "NX XX GET EX PX KEEPTTL", ""},
		{"set, NX", setDoc, "k v nx", 4, "GET EX PX KEEPTTL", ""},
		{"set, GET with no NX", setDoc, "k v get", 5, "EX PX KEEPTTL", ""},
		{"set, EX", setDoc, "k v EX", 5, "", "seconds"},
		{"set, EX 10", setDoc, "k v EX 10", -1, "", ""},
		{"set, NX GET EX 10", setDoc, "k v NX GET ex 10", -1, "", ""},
		{"set, KEEPTTL", setDoc, "k v keepttl", -1, "", ""},
		{"hset, a key", hsetDoc, "k", 2, "", "field"},
		{"hset, a field", hsetDoc, "k f", 2, "", "value"},
		{"hset, a pair", hsetDoc, "k f v", 2, "", "field"},
		{"hset, a pair and a field", hsetDoc, "k f v g", 2, "", "value"},
		{"del, one", delDoc, "a", 1, "", "key"},
		{"del, three", delDoc, "a b c", 1, "", "key"},
		{"zadd, a key", zaddDoc, "k", 2, "NX XX CH", "score"},
		{"zadd, past NX", zaddDoc, "k nx", 3, "CH", "score"},
		{"zadd, a score", zaddDoc, "k 1", 4, "", "member"},
		{"zadd, a pair", zaddDoc, "k 1 a", 4, "", "score"},
		{"zadd, CH after pairs is a member's", zaddDoc, "k 1 a 2", 4, "", "member"},
		{"limit, a key", limitDoc, "k", 2, "LIMIT BY", ""},
		{"limit, its token", limitDoc, "k limit", 2, "", "offset"},
		{"limit, an offset", limitDoc, "k LIMIT 0", 2, "", "count"},
		{"limit, read", limitDoc, "k LIMIT 0 10", 3, "BY", ""},
		{"limit, BY", limitDoc, "k LIMIT 0 10 by", 3, "", "pattern"},
		{"store after keys", storeDoc, "a b STORE", 2, "", "destination"},
		{"keys before STORE", storeDoc, "a b", 1, "STORE", "key"},
		{"a token that has to be there", countDoc, "k", 2, "COUNT", ""},
		{"that token's value", countDoc, "k count", 2, "", "count"},
		{"limit, BY is not LIMIT", limitDoc, "k by", 3, "", "pattern"},
		{"a block that is not begun", trimDoc, "k 1-0", -1, "", ""},
		{"a block that is begun", trimDoc, "k maxlen", 2, "", "threshold"},
		{"optional tokens after keys", afterDoc, "k", 1, "A", "key"},
		{"get, done", getDoc, "k", -1, "", ""},
		{"get, more than it takes", getDoc, "k extra", -1, "", ""},
	} {
		p := c.doc.At(strings.Fields(c.words))
		leaf := ""
		if p.Leaf != nil {
			leaf = p.Leaf.Name
		}
		if p.Field != c.field || strings.Join(p.Tokens, " ") != c.tokens || leaf != c.leaf {
			t.Errorf("%s: field %d tokens %q leaf %q, want %d %q %q", c.name, p.Field, strings.Join(p.Tokens, " "), leaf, c.field, c.tokens, c.leaf)
		}
	}
}

// What COMMAND DOCS gives, a map in RESP3, is what Doc is: the arguments with their
// tokens and flags, the subcommands by their own names (§9.8).
func TestParseDocs(t *testing.T) {
	reply := map[any]any{
		"set": map[any]any{
			"summary": "Sets the string value of a key.",
			"arguments": []any{
				map[any]any{"name": "key", "type": "key"},
				map[any]any{"name": "expiration", "type": "oneof", "flags": []any{"optional"}, "arguments": []any{
					map[any]any{"name": "seconds", "type": "integer", "token": "EX"},
					map[any]any{"name": "keepttl", "type": "pure-token", "token": "KEEPTTL"},
				}},
			},
		},
		"config": map[any]any{"summary": "x", "subcommands": map[any]any{
			"config|set": map[any]any{"summary": "Sets configuration.", "arguments": []any{
				map[any]any{"name": "data", "type": "block", "flags": []any{"multiple"}, "arguments": []any{map[any]any{"name": "parameter", "type": "string"}}},
			}},
		}},
	}
	docs := parseDocs(reply)
	set := docs["set"]
	if set.Summary == "" || len(set.Args) != 2 || !set.Args[1].Optional || set.Args[1].Type != "oneof" || set.Args[1].Args[0].Token != "EX" {
		t.Errorf("set: %+v", set)
	}
	sub, ok := docs["config"].Subs["set"]
	if !ok || sub.Summary != "Sets configuration." || len(sub.Args) != 1 || !sub.Args[0].Multiple || !slices.Equal(sub.Fields("config set"), []string{"CONFIG SET", "parameter …"}) {
		t.Errorf("config set: %+v %v", sub, ok)
	}
}
