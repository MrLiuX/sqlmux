package redis

import (
	"context"
	"errors"
	"slices"
	"strconv"
	"strings"
	"time"

	goredis "github.com/redis/go-redis/v9"

	"github.com/kackerx/sqlmux/internal/db"
	"github.com/kackerx/sqlmux/internal/i18n"
)

// KeyInfo is a key of a db and its type, as TYPE says it: string, hash, list,
// set, zset, stream or ReJSON-RL, the type of a RedisJSON value.
type KeyInfo struct{ Name, Type string }

// Scanned is what a scan of a db read: the keys, the db's size (DBSIZE), and
// whether the scan reached the end or stopped at its budget.
type Scanned struct {
	Keys     []KeyInfo
	Total    int64
	Complete bool
}

// scanCount is the COUNT of each SCAN: a hint of how many keys the server looks
// at, one round trip each (§8.7).
const scanCount = 1000

// Scan reads the keys of the db c is on that match the glob match, SCAN by SCAN
// and never KEYS, until the cursor comes round or budget keys are read; the
// type of each batch comes in one pipeline (§8.7「树」). A key SCAN gives twice
// is once. The keys are in the order they are read.
func Scan(ctx context.Context, c db.Conn, match string, budget int) (s Scanned, err error) {
	seen := map[string]bool{}
	err = Use(ctx, c, func(cl *goredis.Client) error {
		if s.Total, err = cl.DBSize(ctx).Result(); err != nil {
			return err
		}
		var cursor uint64
		for {
			batch, next, err := cl.Scan(ctx, cursor, match, scanCount).Result()
			if err != nil {
				return err
			}
			batch = slices.DeleteFunc(batch, func(k string) bool { dup := seen[k]; seen[k] = true; return dup })
			if room := budget - len(s.Keys); len(batch) > room {
				batch = batch[:room]
				next = 1 // the budget ends it
			}
			types, err := typesOf(ctx, cl, batch)
			if err != nil {
				return err
			}
			for i, k := range batch {
				s.Keys = append(s.Keys, KeyInfo{k, types[i]})
			}
			if next == 0 || len(s.Keys) >= budget {
				s.Complete = next == 0
				return nil
			}
			cursor = next
		}
	})
	return s, err
}

// typesOf is the type of each of keys, one pipeline.
func typesOf(ctx context.Context, cl *goredis.Client, keys []string) ([]string, error) {
	if len(keys) == 0 {
		return nil, nil
	}
	cmds, err := cl.Pipelined(ctx, func(p goredis.Pipeliner) error {
		for _, k := range keys {
			p.Type(ctx, k)
		}
		return nil
	})
	if err != nil && !errors.Is(err, goredis.Nil) {
		return nil, err
	}
	types := make([]string, len(cmds))
	for i, cmd := range cmds {
		types[i] = cmd.(*goredis.StatusCmd).Val()
	}
	return types, nil
}

// Each gives each batch of the keys of the db c is on that match the glob match,
// every one, with no budget: what a delete of a namespace says it deletes
// (§8.7). each stopping it is that error.
func Each(ctx context.Context, c db.Conn, match string, each func(keys []string) error) error {
	return Use(ctx, c, func(cl *goredis.Client) error {
		seen := map[string]bool{}
		var cursor uint64
		for {
			batch, next, err := cl.Scan(ctx, cursor, match, scanCount).Result()
			if err != nil {
				return err
			}
			batch = slices.DeleteFunc(batch, func(k string) bool { dup := seen[k]; seen[k] = true; return dup })
			if len(batch) > 0 {
				if err := each(batch); err != nil {
					return err
				}
			}
			if next == 0 {
				return nil
			}
			cursor = next
		}
	})
}

// Glob is name as a glob that matches it alone: MATCH treats * ? [ ] and \ as
// its own.
func Glob(name string) string {
	return strings.NewReplacer(`\`, `\\`, "*", `\*`, "?", `\?`, "[", `\[`, "]", `\]`).Replace(name)
}

// Databases are the dbs of the server that hold keys, by number as INFO
// keyspace lists them, which is in order.
func Databases(ctx context.Context, c db.Conn) (dbs []string, err error) {
	var text string
	err = Use(ctx, c, func(cl *goredis.Client) (err error) { text, err = cl.Info(ctx, "keyspace").Result(); return err })
	for l := range strings.SplitSeq(text, "\n") {
		if name, _, ok := strings.Cut(strings.TrimSpace(l), ":"); ok && strings.HasPrefix(name, "db") {
			if n, err := strconv.Atoi(name[2:]); err == nil {
				dbs = append(dbs, strconv.Itoa(n))
			}
		}
	}
	return dbs, err
}

// Unlink deletes keys without blocking the server, and says how many were
// there.
func Unlink(ctx context.Context, c db.Conn, keys ...string) (n int64, err error) {
	err = Use(ctx, c, func(cl *goredis.Client) (err error) { n, err = cl.Unlink(ctx, keys...).Result(); return err })
	return n, err
}

// Rename makes key to; to a key that is there is left alone and exists says it
// is, unless overwrite (§8.7).
func Rename(ctx context.Context, c db.Conn, key, to string, overwrite bool) (exists bool, err error) {
	err = Use(ctx, c, func(cl *goredis.Client) error {
		if overwrite {
			return cl.Rename(ctx, key, to).Err()
		}
		ok, err := cl.RenameNX(ctx, key, to).Result()
		exists = !ok && err == nil
		return err
	})
	return exists, err
}

// SetTTL gives key a time to live, or none when ttl is 0 (PERSIST).
func SetTTL(ctx context.Context, c db.Conn, key string, ttl time.Duration) error {
	return Use(ctx, c, func(cl *goredis.Client) error {
		if ttl <= 0 {
			return cl.Persist(ctx, key).Err()
		}
		ok, err := cl.PExpire(ctx, key, ttl).Result()
		if err == nil && !ok {
			err = errors.New(i18n.T("键不存在"))
		}
		return err
	})
}

// ParseTTL is what the TTL of the form says: seconds (3600), a duration
// (90m, 1h30m), or nothing, 0 or -1 for none; ok is false for any other.
func ParseTTL(s string) (ttl time.Duration, ok bool) {
	s = strings.TrimSpace(s)
	if s == "" || s == "0" || s == "-1" {
		return 0, true
	}
	if n, err := strconv.Atoi(s); err == nil {
		return time.Duration(n) * time.Second, n > 0
	}
	d, err := time.ParseDuration(s)
	return d, err == nil && d > 0
}
