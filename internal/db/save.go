package db

import (
	"context"
	"strings"
	"time"

	"sqlmux/internal/i18n"
	"sqlmux/internal/sqlkit"
)

// Row is one row's changes to save (§10.3): its row identity's values as
// loaded, and the changed columns in table order.
type Row struct {
	Key  []string
	Cols []Change
}

// Change is one changed column of a Row.
type Change struct {
	Name    string
	Val     Val  // the new value; unused when Default
	Default bool // the DEFAULT keyword
	Old     Val  // as loaded: the row must still hold it
}

// ErrStale is a row that no longer holds what was loaded, or is gone; ErrGone
// one to delete that is gone; ErrNotInserted one to insert that went in as
// none, a PG BEFORE trigger's NULL.
var (
	ErrStale       = msgError(i18n.N("行数据已变化或行不存在"))
	ErrGone        = msgError(i18n.N("行不存在"))
	ErrNotInserted = msgError(i18n.N("没有插入"))
)

// msgError is an error that says itself in the language in use, when it is
// asked (§14「界面语言」); the same text is the same error.
type msgError string

func (e msgError) Error() string { return i18n.T(string(e)) }

// Changes is what a save writes (§10.3, §10.6): the rows to delete, by
// their row identity's values; the rows to update; the rows to insert,
// each the columns set in it, Val or Default, the rest their DEFAULT.
type Changes struct {
	Deletes [][]string
	Updates []Row
	Inserts [][]Change
}

// endTimeout bounds COMMIT and ROLLBACK, which a cancel must not reach: a
// COMMIT the server has done would come back as an error (§10.3).
const endTimeout = 10 * time.Second

// End runs sql, a COMMIT, a ROLLBACK or the like, on c out of ctx's cancel,
// bounded by endTimeout. It returns the command tag: a COMMIT of a failed
// transaction's is ROLLBACK.
func End(ctx context.Context, c Conn, sql string) (string, error) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), endTimeout)
	defer cancel()
	r, err := c.Query(ctx, sql)
	return r.Tag, err
}

// Guarded runs f on c, in savepoint name, a plain word, when c is in a
// transaction: f failing takes back what f did alone, where an error would
// leave the whole transaction failed, all the user did in it lost (§10.3).
func Guarded(ctx context.Context, c Conn, name string, f func() error) error {
	if TxStatus(c) == 'I' {
		return f()
	}
	if _, err := c.Query(ctx, "savepoint "+name); err != nil {
		return err
	}
	if err := f(); err != nil {
		_, _ = End(ctx, c, "rollback to savepoint "+name)
		_, _ = End(ctx, c, "release savepoint "+name)
		return err
	}
	_, err := End(ctx, c, "release savepoint "+name)
	return err
}

// Save writes ch to schema.table on w, in e's SQL (§10.3, §10.6): first
// the DELETEs, then the UPDATEs, then the INSERTs, each hitting exactly
// one row. A row is found by keyCols with =; an UPDATE's also by each
// changed column still holding what was loaded. In its own transaction it
// commits, or on any failure rolls back. In the manual transaction mode
// (one begun first if need be), or in a transaction the user began, it
// goes in that one, not committed, a failure taking back this save alone.
// failed is the index of the statement at fault in that order, deletes,
// updates, inserts, -1 for none; tx is where the transaction is as it
// ends. Cancelling ctx stops the statements only.
// Reference: lazysql drivers/utils.go queriesInTransaction and its
// ExecutePendingChanges, which run the changes as listed and check
// neither the rows hit nor the loaded values; its INSERT leaves a DEFAULT
// column out, where ours writes DEFAULT.
func Save(ctx context.Context, w *Worker, e Engine, schema, table string, keyCols []string, ch Changes, manual bool) (failed int, tx byte, err error) {
	type stmt struct {
		sql  string
		args []Val
		miss error // when it hits no row
	}
	var stmts []stmt
	for _, k := range ch.Deletes {
		sql, args := remove(e, schema, table, keyCols, k)
		stmts = append(stmts, stmt{sql, args, ErrGone})
	}
	for _, r := range ch.Updates {
		sql, args := update(e, schema, table, keyCols, r)
		stmts = append(stmts, stmt{sql, args, ErrStale})
	}
	for _, r := range ch.Inserts {
		sql, args := insert(e, schema, table, r)
		stmts = append(stmts, stmt{sql, args, ErrNotInserted})
	}
	failed = -1
	err = w.Run(ctx, func(ctx context.Context, c Conn) error {
		defer func() { tx = TxStatus(c) }()
		write := func() error {
			for i, s := range stmts {
				res, err := c.Query(ctx, s.sql, s.args...)
				if err == nil && res.Affected != 1 {
					err = s.miss
				}
				if err != nil {
					failed = i
					return err
				}
			}
			return nil
		}
		nested := manual || TxStatus(c) != 'I'
		if TxStatus(c) == 'I' {
			if _, err := c.Query(ctx, "begin"); err != nil {
				_, _ = End(ctx, c, "rollback") // a cancel may land once the server has begun
				return err
			}
		}
		if nested {
			return Guarded(ctx, c, "sqlmux_save", write)
		}
		if err := write(); err != nil {
			_, _ = End(ctx, c, "rollback")
			return err
		}
		_, err := End(ctx, c, "commit")
		return err
	})
	return failed, tx, err
}

// keyWhere is what finds a row by its row identity's values, arg taking
// each value in.
func keyWhere(e Engine, keyCols, key []string, arg func(Val) string) []string {
	var where []string
	for i, k := range keyCols {
		where = append(where, e.Quote(k)+" = "+arg(Val{S: key[i]}))
	}
	return where
}

// argsOf is a statement's arguments, and what puts each one in as e
// writes them.
func argsOf(e Engine) (*[]Val, func(Val) string) {
	var args []Val
	return &args, func(v Val) string {
		args = append(args, v)
		return e.Arg(len(args))
	}
}

// remove is the DELETE of the row with key, and its arguments.
func remove(e Engine, schema, table string, keyCols, key []string) (string, []Val) {
	args, arg := argsOf(e)
	return "delete from " + e.Quote(schema, table) + " where " + strings.Join(keyWhere(e, keyCols, key, arg), " and "), *args
}

// insert is the INSERT of a row with cols set, and its arguments; with
// none set, every column's DEFAULT, as PG's DEFAULT VALUES and MySQL's
// () VALUES () write it (§10.6).
func insert(e Engine, schema, table string, cols []Change) (string, []Val) {
	switch {
	case len(cols) > 0:
	case e.Dialect() == sqlkit.MySQL:
		return "insert into " + e.Quote(schema, table) + " () values ()", nil
	default:
		return "insert into " + e.Quote(schema, table) + " default values", nil
	}
	args, arg := argsOf(e)
	var names, vals []string
	for _, c := range cols {
		v := "DEFAULT"
		if !c.Default {
			v = arg(c.Val)
		}
		names, vals = append(names, e.Quote(c.Name)), append(vals, v)
	}
	return "insert into " + e.Quote(schema, table) + " (" + strings.Join(names, ", ") + ") values (" + strings.Join(vals, ", ") + ")", *args
}

// update is r's UPDATE and its arguments, all text: the changed columns
// must still hold what was loaded, as e's Same tells (§10.3).
func update(e Engine, schema, table string, keyCols []string, r Row) (string, []Val) {
	var sets []string
	args, arg := argsOf(e)
	for _, c := range r.Cols {
		v := "DEFAULT"
		if !c.Default {
			v = arg(c.Val)
		}
		sets = append(sets, e.Quote(c.Name)+" = "+v)
	}
	where := keyWhere(e, keyCols, r.Key, arg)
	for _, c := range r.Cols {
		if c.Old.Null {
			where = append(where, e.Quote(c.Name)+" is null")
		} else {
			where = append(where, e.Same(e.Quote(c.Name), arg(c.Old)))
		}
	}
	return "update " + e.Quote(schema, table) + " set " + strings.Join(sets, ", ") +
		" where " + strings.Join(where, " and "), *args
}
