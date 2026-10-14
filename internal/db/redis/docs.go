package redis

import (
	"context"
	"strings"

	goredis "github.com/redis/go-redis/v9"

	"github.com/kackerx/sqlmux/internal/db"
)

// Arg is an argument of a command as COMMAND DOCS describes it (§9.8): a name and
// a type (key, string, integer, pattern, pure-token, oneof, block …), the token
// that goes before its value when it has one, and what it may be: left out, or
// given again. A oneof or a block has the arguments it is made of.
type Arg struct {
	Name, Type, Token string
	Optional          bool
	Multiple          bool
	Args              []Arg
}

// Doc is a command's documentation: a line on what it does, its arguments, and its
// subcommands by their own names (set, not config|set).
type Doc struct {
	Summary string
	Args    []Arg
	Subs    map[string]Doc
}

// Docs are the documentation of the server's commands by name in lower case. Nil is
// a server that has none (before 7, or one that would not say).
type Docs map[string]Doc

// LoadDocs reads COMMAND DOCS, Redis 7's, which a server before it answers with an
// error (§9.8).
func LoadDocs(ctx context.Context, c db.Conn) (Docs, error) {
	var reply any
	err := Use(ctx, c, func(cl *goredis.Client) (err error) {
		reply, err = cl.Do(ctx, "COMMAND", "DOCS").Result()
		return err
	})
	if err != nil {
		return nil, err
	}
	return parseDocs(reply), nil
}

// parseDocs is the map COMMAND DOCS gives (RESP3), command name to its fields.
func parseDocs(reply any) Docs {
	docs := Docs{}
	all, _ := reply.(map[any]any)
	for name, v := range all {
		n, _ := name.(string)
		docs[strings.ToLower(n)] = parseDoc(v)
	}
	return docs
}

func parseDoc(v any) (d Doc) {
	m, _ := v.(map[any]any)
	d.Summary, _ = m["summary"].(string)
	d.Args = parseArgs(m["arguments"])
	if subs, _ := m["subcommands"].(map[any]any); len(subs) > 0 {
		d.Subs = map[string]Doc{}
		for name, sv := range subs {
			n, _ := name.(string)
			_, sub, _ := strings.Cut(n, "|")
			d.Subs[strings.ToLower(sub)] = parseDoc(sv)
		}
	}
	return d
}

func parseArgs(v any) (args []Arg) {
	list, _ := v.([]any)
	for _, e := range list {
		m, _ := e.(map[any]any)
		a := Arg{Args: parseArgs(m["arguments"])}
		a.Name, _ = m["name"].(string)
		a.Type, _ = m["type"].(string)
		a.Token, _ = m["token"].(string)
		for _, f := range words(m["flags"]) {
			a.Optional = a.Optional || f == "optional"
			a.Multiple = a.Multiple || f == "multiple" || f == "multiple_token"
		}
		args = append(args, a)
	}
	return args
}

// Text is the argument as a line of the command shows it: its name, or its token and
// name; the alternatives of a oneof between |; [] around what may be left out and …
// after what may be given again.
func (a Arg) Text() string {
	var s string
	switch {
	case a.Type == "pure-token":
		s = a.Token
	case a.Type == "oneof":
		parts := make([]string, len(a.Args))
		for i, x := range a.Args {
			parts[i] = x.Text()
		}
		s = strings.Join(parts, " | ")
	case a.Type == "block":
		parts := make([]string, len(a.Args))
		for i, x := range a.Args {
			parts[i] = x.Text()
		}
		s = strings.Join(parts, " ")
		if a.Token != "" {
			s = a.Token + " " + s
		}
	case a.Token != "":
		s = a.Token + " " + a.Name
	default:
		s = a.Name
	}
	if a.Multiple {
		s += " …"
	}
	if a.Optional {
		s = "[" + s + "]"
	}
	return s
}

// Fields are the command's line as the signature shows it: the name, then each of
// its arguments as Text says it (§9.8).
func (d Doc) Fields(name string) []string {
	fields := []string{strings.ToUpper(name)}
	for _, a := range d.Args {
		fields = append(fields, a.Text())
	}
	return fields
}

// Place is where the next word of a command line goes: the field of Fields it is in
// (0 is the command's name), the words that may stand there, the tokens of the
// arguments that may come next, and the argument whose value it may be (nil when
// only a token goes there).
type Place struct {
	Field  int
	Tokens []string
	Leaf   *Arg
}

// At is where the word after words goes, words being what the line has after the
// command's name (and subcommand's). It reads the arguments in order: a token that
// is there is taken with its value, a oneof by the alternative it starts with, a
// block by what it is made of, an argument that may be given again as long as words
// are not another argument's token. The position is linear: an optional token after
// arguments that are given again may be put on the wrong one (§9.8).
func (d Doc) At(words []string) Place {
	p, _ := walk(d.Args, words, 0)
	return p
}

// walk reads words as args, the field numbers counting from base; it says where the
// word after them goes (done false: the words ran out inside, or before, the args)
// or that all the args are read and the words are left over (done true).
func walk(args []Arg, words []string, base int) (Place, bool) {
	i := 0
	for idx := range args {
		a := args[idx]
		if i >= len(words) {
			return expect(args, idx, base), false
		}
		n, in := consume(a, words[i:])
		if in != nil {
			in.Field = base + idx + 1
			return *in, false
		}
		i += n
		for a.Multiple && n > 0 && i < len(words) {
			if startsAnother(args[idx+1:], words[i]) {
				break
			}
			if n, in = consume(a, words[i:]); in != nil {
				in.Field = base + idx + 1
				return *in, false
			}
			i += n
		}
		if a.Multiple && n > 0 && i >= len(words) { // another of it may come, or what follows it
			p := expect(args, idx, base)
			for _, next := range args[idx+1:] {
				p.Tokens = append(p.Tokens, tokens(next)...)
				if !next.Optional {
					break
				}
			}
			return p, false
		}
	}
	return Place{Field: -1}, true
}

// startsAnother is whether word is a token that one of args begins with.
func startsAnother(args []Arg, word string) bool {
	for _, a := range args {
		for _, t := range tokens(a) {
			if strings.EqualFold(t, word) {
				return true
			}
		}
	}
	return false
}

// consume reads arg a from the front of words: how many it takes, 0 when it is not
// there (a left out one, a token that is not the word); in is where the word after
// them goes when they end inside a.
func consume(a Arg, words []string) (n int, in *Place) {
	if len(words) == 0 {
		return 0, nil
	}
	switch {
	case a.Type == "pure-token":
		if strings.EqualFold(words[0], a.Token) {
			return 1, nil
		}
		return 0, nil
	case a.Type == "oneof":
		for _, alt := range a.Args {
			if startsWith(alt, words[0]) {
				return consume(alt, words)
			}
		}
		return 0, nil
	case a.Type == "block":
		rest, taken := words, 0
		switch {
		case a.Token != "": // LIMIT offset count: its token, then what it is made of
			if !strings.EqualFold(words[0], a.Token) {
				return 0, nil
			}
			rest, taken = words[1:], 1
		case a.Optional && len(a.Args) > 0 && !startsWith(a.Args[0], words[0]):
			return 0, nil
		}
		p, done := walk(a.Args, rest, 0)
		if !done {
			return len(words), &Place{Tokens: p.Tokens, Leaf: p.Leaf}
		}
		return taken + blockLen(a.Args, rest), nil
	case a.Token != "":
		if !strings.EqualFold(words[0], a.Token) {
			return 0, nil
		}
		if len(words) == 1 {
			v := a
			v.Token = ""
			return 1, &Place{Leaf: &v}
		}
		return 2, nil
	}
	return 1, nil // a value
}

// blockLen is how many of words the args of a block take, all of them read.
func blockLen(args []Arg, words []string) int {
	i := 0
	for _, a := range args {
		if i >= len(words) {
			break
		}
		n, _ := consume(a, words[i:])
		i += n
	}
	return i
}

// startsWith is whether word begins arg a: its token, or one of a oneof's, or the
// first of a block's; a value is begun by any word, so by none here.
func startsWith(a Arg, word string) bool {
	for _, t := range tokens(a) {
		if strings.EqualFold(t, word) {
			return true
		}
	}
	return false
}

// tokens are the words that may begin arg a: its own token, those of a oneof's
// alternatives, the first argument's of a block; none for a value.
func tokens(a Arg) []string {
	switch {
	case a.Type == "pure-token" || a.Token != "":
		return []string{a.Token}
	case a.Type == "oneof":
		var out []string
		for _, x := range a.Args {
			out = append(out, tokens(x)...)
		}
		return out
	case a.Type == "block" && len(a.Args) > 0:
		return tokens(a.Args[0])
	}
	return nil
}

// expect is the place where arg idx of args begins, no word typed yet: the tokens
// of it and of the optional ones after it, up to the first that has to be there,
// whose value (when it is a value) is what goes there.
func expect(args []Arg, idx, base int) Place {
	p := Place{Field: base + idx + 1}
	for j := idx; j < len(args); j++ {
		a := args[j]
		p.Tokens = append(p.Tokens, tokens(a)...)
		if a.Optional {
			continue
		}
		if len(tokens(a)) == 0 {
			p.Leaf = leafOf(a)
		}
		break
	}
	return p
}

// leafOf is the argument a word is the value of at the start of a: itself, or the
// first of a block.
func leafOf(a Arg) *Arg {
	if a.Type == "block" && len(a.Args) > 0 {
		return leafOf(a.Args[0])
	}
	if a.Type == "oneof" || a.Type == "pure-token" {
		return nil
	}
	return &a
}
