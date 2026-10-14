package redis

import (
	"context"
	"slices"
	"strings"

	goredis "github.com/redis/go-redis/v9"

	"github.com/kackerx/sqlmux/internal/db"
)

// Command is what the server says of one command, or one subcommand of it
// (COMMAND): its flags, and the ACL categories it is in.
type Command struct {
	Flags, Categories []string
}

// Commands are the commands of a server by name, in lower case, a subcommand as
// container|sub (§9.8). Nil is the server that would not say.
type Commands map[string]Command

// LoadCommands reads the server's commands with COMMAND, every one with its
// subcommands (§9.8).
func LoadCommands(ctx context.Context, c db.Conn) (Commands, error) {
	var reply any
	err := Use(ctx, c, func(cl *goredis.Client) (err error) {
		reply, err = cl.Do(ctx, "COMMAND").Result()
		return err
	})
	if err != nil {
		return nil, err
	}
	cmds := Commands{}
	cmds.add(reply)
	return cmds, nil
}

// add takes in the entries of reply, an array of [name, arity, flags, first key,
// last key, step, ACL categories, tips, key specs, subcommands].
func (cs Commands) add(reply any) {
	entries, _ := reply.([]any)
	for _, e := range entries {
		f, _ := e.([]any)
		if len(f) < 3 {
			continue
		}
		name, _ := f[0].(string)
		cmd := Command{Flags: words(f[2])}
		if len(f) > 6 {
			cmd.Categories = words(f[6])
		}
		cs[strings.ToLower(name)] = cmd
		if len(f) > 9 {
			cs.add(f[9])
		}
	}
}

// words are the strings of a reply array, in lower case.
func words(v any) (out []string) {
	a, _ := v.([]any)
	for _, e := range a {
		if s, ok := e.(string); ok {
			out = append(out, strings.ToLower(s))
		}
	}
	return out
}

// of is the entry for the command a line starts with: container|sub when the
// server has that, else the command. ok is false for one it has not.
func (cs Commands) of(args []string) (Command, bool) {
	if len(args) == 0 {
		return Command{}, false
	}
	name := strings.ToLower(args[0])
	if len(args) > 1 {
		if c, ok := cs[name+"|"+strings.ToLower(args[1])]; ok {
			return c, true
		}
	}
	c, ok := cs[name]
	return c, ok
}

// Writes is whether a read-only connection keeps the line back: its command is
// flagged write or may_replicate, is a script that is not flagged readonly (EVAL
// and FCALL write as they like), is administration (CONFIG SET, ACL SETUSER,
// SHUTDOWN: the server's own state), or the server does not know it, or would not
// say (§9.8, §13). A line that is not sent is none.
func (cs Commands) Writes(line string) bool {
	args, err := SplitLine(line)
	if err != nil || len(args) == 0 {
		return false
	}
	c, ok := cs.of(args)
	has := func(list []string, w string) bool { return slices.Contains(list, w) }
	return !ok || has(c.Flags, "write") || has(c.Flags, "may_replicate") || has(c.Categories, "@admin") ||
		has(c.Categories, "@scripting") && !has(c.Flags, "readonly")
}

// Dangerous is whether the line is of a command in the @dangerous category (§13);
// a server that did not say has none.
func (cs Commands) Dangerous(line string) bool {
	args, err := SplitLine(line)
	if err != nil {
		return false
	}
	c, _ := cs.of(args)
	return slices.Contains(c.Categories, "@dangerous")
}

// Sensitive is whether the line carries a password, which the history is not to
// keep: AUTH, HELLO and MIGRATE (which take one), ACL SETUSER (a > in it), and
// CONFIG SET of a requirepass, masterauth or masteruser.
func Sensitive(line string) bool {
	args, err := SplitLine(line)
	if err != nil || len(args) == 0 {
		return false
	}
	switch strings.ToLower(args[0]) {
	case "auth", "hello", "migrate":
		return true
	case "acl":
		return len(args) > 1 && strings.EqualFold(args[1], "setuser")
	case "config":
		return len(args) > 2 && strings.EqualFold(args[1], "set") && slices.ContainsFunc(args[2:], func(a string) bool {
			a = strings.ToLower(a)
			return a == "requirepass" || a == "masterauth" || a == "masteruser"
		})
	}
	return false
}
