package redis

import (
	"context"
	"errors"
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"

	goredis "github.com/redis/go-redis/v9"

	"github.com/kackerx/sqlmux/internal/db"
	"github.com/kackerx/sqlmux/internal/i18n"
)

// ErrExists is a field, member or entry that is there already, where another
// is to be given its name.
var ErrExists = msgError(i18n.N("已有同名的项"))

// ErrNoNull is a value of NULL: Redis has none.
var ErrNoNull = msgError(i18n.N("Redis 的值不能是 NULL"))

// onPlanned is a test's hook, for tests only: it runs once the changes are checked and planned,
// before the commands go out, where another client may change the key.
var onPlanned func()

// step is one command of a save, and the change it is of, by its place among
// them: deletes, updates, inserts.
type step struct {
	args  []any
	owner int
}

// changeError is the change that is at fault in a save, and why.
type changeError struct {
	idx int
	err error
}

func (e changeError) Error() string { return e.err.Error() }
func (e changeError) Unwrap() error { return e.err }

// Save writes the changes of a table tab of the key in one MULTI / EXEC, after a
// WATCH of the key and a read of what is there, which must be what was loaded:
// a cell that is no more what it was, or a row that is gone, is ErrStale or
// ErrGone, a name that is taken ErrExists (§8.7). A read-only connection has
// none of it. An EXEC that was sent and whose answer was lost says so, as a COMMIT
// does (F13.1).
func (k Key) Save(ctx context.Context, ch db.Changes, _ bool) (failed int, tx byte, err error) {
	if k.ReadOnly {
		return -1, 0, db.ErrReadOnly
	}
	typ, err := k.typeOf(ctx, k.Main)
	if err != nil {
		return -1, 0, err
	}
	failed, sent := -1, false
	err = k.Main.Run(ctx, func(ctx context.Context, c db.Conn) error {
		return Use(ctx, c, func(cl *goredis.Client) error {
			return cl.Watch(ctx, func(tx *goredis.Tx) error {
				steps, err := plan(ctx, tx, typ, k.Name, ch)
				if err != nil {
					return err
				}
				if onPlanned != nil {
					onPlanned()
				}
				sent = true
				cmds, err := tx.TxPipelined(ctx, func(p goredis.Pipeliner) error {
					for _, s := range steps {
						p.Do(ctx, s.args...)
					}
					return nil
				})
				if err != nil && !errors.Is(err, goredis.TxFailedErr) {
					for i, cmd := range cmds { // the command at fault, by its change
						if cmd.Err() != nil {
							failed = steps[i].owner
							return cmd.Err()
						}
					}
				}
				return err
			}, k.Name)
		})
	})
	var ce changeError
	switch {
	case errors.As(err, &ce):
		failed, err = ce.idx, ce.err
	case errors.Is(err, goredis.TxFailedErr):
		err = db.ErrStale
	case sent && err != nil && (Driver{}).Broken(err) == db.LostSent:
		err = db.ErrCommitUnknown
	}
	return failed, 'I', err
}

// reader is what plan reads the key with: a Tx that watches it, or loaded.
type reader interface {
	Get(ctx context.Context, key string) *goredis.StringCmd
	Do(ctx context.Context, args ...any) *goredis.Cmd
	HExists(ctx context.Context, key, field string) *goredis.BoolCmd
	HGet(ctx context.Context, key, field string) *goredis.StringCmd
	LLen(ctx context.Context, key string) *goredis.IntCmd
	LIndex(ctx context.Context, key string, index int64) *goredis.StringCmd
	SIsMember(ctx context.Context, key string, member any) *goredis.BoolCmd
	ZScore(ctx context.Context, key, member string) *goredis.FloatCmd
	XRange(ctx context.Context, key, start, stop string) *goredis.XMessageSliceCmd
}

// deletedMark starts the value a list's deleted places hold until the one LREM; the
// marks of Preview, of what is not told before the save, start with a NUL too, which
// no value of a change has (writable): a placeholder is known by where it came from.
const (
	deletedMark = "\x00sqlmux-deleted\x00"
	keptMark    = "\x00its value" // the value a rename keeps
	scoreMark   = "\x00its score" // the score a rename keeps
)

// plan reads what the changes need of the key, on tx, which watches it, checks
// that it is as it was loaded, and gives the commands that make the changes.
func plan(ctx context.Context, tx reader, typ, key string, ch db.Changes) (steps []step, err error) {
	nd, nu := len(ch.Deletes), len(ch.Updates)
	at := func(idx int, err error) error { return changeError{idx, err} }
	cmd := func(owner int, args ...any) { steps = append(steps, step{args, owner}) }
	// What a read of the key's entries comes to: a nil is that the entry is not there,
	// absent that a check found none, taken that it found one, and any error besides (a
	// connection that went, the server's) is returned as it is, not made a row's.
	miss := func(idx int, err error) error {
		if errors.Is(err, goredis.Nil) {
			return at(idx, db.ErrGone)
		}
		return err
	}
	absent := func(idx int, ok bool, err error) error {
		switch {
		case err != nil:
			return err
		case !ok:
			return at(idx, db.ErrGone)
		}
		return nil
	}
	taken := func(idx int, ok bool, err error) error {
		switch {
		case err != nil:
			return err
		case ok:
			return at(idx, ErrExists)
		}
		return nil
	}
	switch typ {
	case "string", "ReJSON-RL":
		if len(ch.Inserts) > 0 {
			return nil, at(nd+nu, fmt.Errorf("%s", i18n.T("string 只有一个值")))
		}
		get := func() (string, error) {
			if typ == "string" {
				return tx.Get(ctx, key).Result()
			}
			return tx.Do(ctx, "JSON.GET", key, ".").Text()
		}
		for i := range ch.Deletes {
			if _, err := get(); err != nil {
				return nil, miss(i, err)
			}
			cmd(i, "UNLINK", key)
		}
		for i, r := range ch.Updates {
			cur, err := get()
			if err != nil {
				return nil, miss(nd+i, err)
			}
			for _, c := range r.Cols {
				if err := writable(c); err != nil {
					return nil, at(nd+i, err)
				}
				if cur != c.Old.S {
					return nil, at(nd+i, db.ErrStale)
				}
				if typ == "string" {
					cmd(nd+i, "SET", key, c.Val.S, "KEEPTTL")
				} else {
					cmd(nd+i, "JSON.SET", key, "$", c.Val.S)
				}
			}
		}
	case "hash":
		for i, d := range ch.Deletes {
			ok, err := tx.HExists(ctx, key, d[0]).Result()
			if err := absent(i, ok, err); err != nil {
				return nil, err
			}
			cmd(i, "HDEL", key, d[0])
		}
		for i, r := range ch.Updates {
			field, owner := r.Key[0], nd+i
			cur, err := tx.HGet(ctx, key, field).Result()
			if err != nil {
				return nil, miss(owner, err)
			}
			value, rename := cur, ""
			for _, c := range r.Cols {
				if err := writable(c); err != nil {
					return nil, at(owner, err)
				}
				if c.Old.S != map[string]string{"field": field, "value": cur}[c.Name] {
					return nil, at(owner, db.ErrStale)
				}
				switch c.Name {
				case "value":
					value = c.Val.S
				case "field":
					rename = c.Val.S
				}
			}
			if rename != "" && rename != field {
				ok, err := tx.HExists(ctx, key, rename).Result()
				if err := taken(owner, ok, err); err != nil {
					return nil, err
				}
				cmd(owner, "HDEL", key, field)
				cmd(owner, "HSET", key, rename, value)
			} else {
				cmd(owner, "HSET", key, field, value)
			}
		}
		for i, r := range ch.Inserts {
			f, v, err := pair(r, "field", "value")
			if err != nil {
				return nil, at(nd+nu+i, err)
			}
			ok, err := tx.HExists(ctx, key, f).Result()
			if err := taken(nd+nu+i, ok, err); err != nil {
				return nil, err
			}
			cmd(nd+nu+i, "HSET", key, f, v)
		}
	case "list":
		n, err := tx.LLen(ctx, key).Result()
		if err != nil {
			return nil, err
		}
		sentinel := deletedMark + strconv.FormatInt(n, 10)
		for i, d := range ch.Deletes {
			idx, err := strconv.ParseInt(d[0], 10, 64)
			if err != nil || idx < 0 || idx >= n {
				return nil, at(i, db.ErrGone)
			}
			cmd(i, "LSET", key, strconv.FormatInt(idx, 10), sentinel)
		}
		for i, r := range ch.Updates {
			idx, err := strconv.ParseInt(r.Key[0], 10, 64)
			if err != nil || idx < 0 || idx >= n {
				return nil, at(nd+i, db.ErrGone)
			}
			cur, err := tx.LIndex(ctx, key, idx).Result()
			if err != nil {
				return nil, miss(nd+i, err)
			}
			for _, c := range r.Cols {
				if err := writable(c); err != nil {
					return nil, at(nd+i, err)
				}
				if c.Old.S != cur {
					return nil, at(nd+i, db.ErrStale)
				}
				cmd(nd+i, "LSET", key, strconv.FormatInt(idx, 10), c.Val.S)
			}
		}
		if nd > 0 { // the places of what stays hold until the one LREM, which comes after the LSETs
			cmd(0, "LREM", key, 0, sentinel)
		}
		for i, r := range ch.Inserts {
			v, err := value(r, "value")
			if err != nil {
				return nil, at(nd+nu+i, err)
			}
			cmd(nd+nu+i, "RPUSH", key, v)
		}
	case "set":
		for i, d := range ch.Deletes {
			ok, err := tx.SIsMember(ctx, key, d[0]).Result()
			if err := absent(i, ok, err); err != nil {
				return nil, err
			}
			cmd(i, "SREM", key, d[0])
		}
		for i, r := range ch.Updates {
			old := r.Key[0]
			ok, err := tx.SIsMember(ctx, key, old).Result()
			if err := absent(nd+i, ok, err); err != nil {
				return nil, err
			}
			for _, c := range r.Cols {
				if err := writable(c); err != nil {
					return nil, at(nd+i, err)
				}
				ok, err := tx.SIsMember(ctx, key, c.Val.S).Result()
				if err != nil {
					return nil, err
				}
				if ok && c.Val.S != old {
					return nil, at(nd+i, ErrExists)
				}
				cmd(nd+i, "SREM", key, old)
				cmd(nd+i, "SADD", key, c.Val.S)
			}
		}
		for i, r := range ch.Inserts {
			m, err := value(r, "member")
			if err != nil {
				return nil, at(nd+nu+i, err)
			}
			ok, err := tx.SIsMember(ctx, key, m).Result()
			if err := taken(nd+nu+i, ok, err); err != nil {
				return nil, err
			}
			cmd(nd+nu+i, "SADD", key, m)
		}
	case "zset":
		for i, d := range ch.Deletes {
			if _, err := tx.ZScore(ctx, key, d[0]).Result(); err != nil {
				return nil, miss(i, err)
			}
			cmd(i, "ZREM", key, d[0])
		}
		for i, r := range ch.Updates {
			member, owner := r.Key[0], nd+i
			cur, err := tx.ZScore(ctx, key, member).Result()
			if err != nil {
				return nil, miss(owner, err)
			}
			score, rename := scoreText(cur), ""
			for _, c := range r.Cols {
				if err := writable(c); err != nil {
					return nil, at(owner, err)
				}
				switch c.Name {
				case "score":
					if old, err := strconv.ParseFloat(c.Old.S, 64); err != nil || old != cur {
						return nil, at(owner, db.ErrStale)
					}
					score = c.Val.S
				case "member":
					rename = c.Val.S
				}
			}
			if rename != "" && rename != member {
				if _, err := tx.ZScore(ctx, key, rename).Result(); !errors.Is(err, goredis.Nil) {
					return nil, taken(owner, err == nil, err)
				}
				cmd(owner, "ZREM", key, member)
				cmd(owner, "ZADD", key, score, rename)
			} else {
				cmd(owner, "ZADD", key, score, member)
			}
		}
		for i, r := range ch.Inserts {
			m, s, err := pair(r, "member", "score")
			if err != nil {
				return nil, at(nd+nu+i, err)
			}
			if _, err := tx.ZScore(ctx, key, m).Result(); !errors.Is(err, goredis.Nil) {
				return nil, taken(nd+nu+i, err == nil, err)
			}
			cmd(nd+nu+i, "ZADD", key, s, m)
		}
	case "stream":
		if len(ch.Updates) > 0 || len(ch.Inserts) > 0 {
			return nil, at(nd, fmt.Errorf("%s", i18n.T("stream 的条目不能改，只能删除")))
		}
		for i, d := range ch.Deletes {
			es, err := tx.XRange(ctx, key, d[0], d[0]).Result()
			if err := absent(i, len(es) == 1, err); err != nil {
				return nil, err
			}
			cmd(i, "XDEL", key, d[0])
		}
	default:
		return nil, fmt.Errorf("%s", i18n.T("不认识的类型 %s", typ))
	}
	return steps, nil
}

// scoreText is a score as ZADD takes it; the NaN that loaded says for one it does not
// know, which no server gives, is scoreMark.
func scoreText(f float64) string {
	if math.IsNaN(f) {
		return scoreMark
	}
	return strconv.FormatFloat(f, 'g', -1, 64)
}

// writable is whether a change's values can be written: a NULL cannot, and the
// text of bytes is no bytes (§8.1).
func writable(c db.Change) error {
	switch {
	case c.Val.Null || c.Default:
		return ErrNoNull
	case db.IsBinaryShown(c.Val.S) || db.IsBinaryShown(c.Old.S):
		return ErrBinary
	}
	return nil
}

// value is the value a row inserted gives its column name.
func value(r []db.Change, name string) (string, error) {
	for _, c := range r {
		if c.Name == name && !c.Default {
			if err := writable(c); err != nil {
				return "", err
			}
			return c.Val.S, nil
		}
	}
	return "", fmt.Errorf("%s", i18n.T("缺少 %s", name))
}

// pair is the values a row inserted gives its columns a and b, both needed, the
// first not empty.
func pair(r []db.Change, a, b string) (string, string, error) {
	x, err := value(r, a)
	if err != nil {
		return "", "", err
	}
	y, err := value(r, b)
	if err != nil {
		return "", "", err
	}
	if strings.TrimSpace(x) == "" {
		return "", "", fmt.Errorf("%s", i18n.T("缺少 %s", a))
	}
	return x, y, nil
}

// Preview is the commands Save sends for ch, one text each, in redis-cli's words:
// plan run on the key as the tab loaded it (loaded), so what is shown is what is
// made. None when the tab knows no type, or plan would refuse ch, which Save then
// says. What the changes do not give, the value or score a rename keeps and a
// list's deleted places, is <its value>, <its score> and <deleted>.
func (k Key) Preview(ch db.Changes) (out []string) {
	steps, err := plan(context.Background(), loaded(ch), k.Type, k.Name, ch)
	if err != nil {
		return nil
	}
	for _, s := range steps {
		words := make([]string, len(s.args))
		for i, a := range s.args {
			words[i] = word(fmt.Sprint(a))
		}
		out = append(out, strings.Join(words, " "))
	}
	return out
}

func word(s string) string {
	switch {
	case strings.HasPrefix(s, deletedMark):
		return "<deleted>"
	case s == keptMark:
		return "<its value>"
	case s == scoreMark:
		return "<its score>"
	}
	return Quote(s)
}

// loaded is the key as the tab loaded it, as far as the changes tell: the entries
// they name are there, holding what their old values say; a read with nothing to
// answer from gets a placeholder.
type loaded db.Changes

// has is whether the entry name (field, member) is one the changes delete or edit.
func (l loaded) has(name string) bool {
	return slices.ContainsFunc(l.Deletes, func(d []string) bool { return d[0] == name }) ||
		slices.ContainsFunc(l.Updates, func(r db.Row) bool { return r.Key[0] == name })
}

// old is what column col of the edited row at key held; key "" is any row, col ""
// any column.
func (l loaded) old(key, col string) (string, bool) {
	for _, r := range l.Updates {
		if key != "" && r.Key[0] != key {
			continue
		}
		for _, c := range r.Cols {
			if col == "" || c.Name == col {
				return c.Old.S, true
			}
		}
	}
	return "", false
}

func (l loaded) Get(context.Context, string) *goredis.StringCmd {
	s, _ := l.old("", "")
	return goredis.NewStringResult(s, nil)
}

func (l loaded) Do(context.Context, ...any) *goredis.Cmd {
	s, _ := l.old("", "")
	return goredis.NewCmdResult(s, nil)
}

func (l loaded) HExists(_ context.Context, _, field string) *goredis.BoolCmd {
	return goredis.NewBoolResult(l.has(field), nil)
}

func (l loaded) HGet(_ context.Context, _, field string) *goredis.StringCmd {
	v, ok := l.old(field, "value")
	if !ok {
		v = keptMark
	}
	return goredis.NewStringResult(v, nil)
}

// LLen is just enough for every index the changes name to be in.
func (l loaded) LLen(context.Context, string) *goredis.IntCmd {
	var n int64
	for _, d := range l.Deletes {
		i, _ := strconv.ParseInt(d[0], 10, 64)
		n = max(n, i+1)
	}
	for _, r := range l.Updates {
		i, _ := strconv.ParseInt(r.Key[0], 10, 64)
		n = max(n, i+1)
	}
	return goredis.NewIntResult(n, nil)
}

func (l loaded) LIndex(_ context.Context, _ string, i int64) *goredis.StringCmd {
	v, _ := l.old(strconv.FormatInt(i, 10), "")
	return goredis.NewStringResult(v, nil)
}

func (l loaded) SIsMember(_ context.Context, _ string, member any) *goredis.BoolCmd {
	return goredis.NewBoolResult(l.has(fmt.Sprint(member)), nil)
}

func (l loaded) ZScore(_ context.Context, _, member string) *goredis.FloatCmd {
	if !l.has(member) {
		return goredis.NewFloatResult(0, goredis.Nil)
	}
	score := math.NaN()
	if s, ok := l.old(member, "score"); ok {
		score, _ = strconv.ParseFloat(s, 64)
	}
	return goredis.NewFloatResult(score, nil)
}

func (l loaded) XRange(_ context.Context, _, id, _ string) *goredis.XMessageSliceCmd {
	return goredis.NewXMessageSliceCmdResult([]goredis.XMessage{{ID: id}}, nil)
}
