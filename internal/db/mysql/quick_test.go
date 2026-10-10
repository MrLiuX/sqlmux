//go:build integration

package mysql

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kackerx/sqlmux/internal/db"
)

// Quick SQL reads in a read-only transaction, in the tree's database, a
// read limited to a row past maxRows by §9.4's LIMIT, one with its own
// worked out whole (§12).
func TestQuick(t *testing.T) {
	w := db.NewWorker(connect(t, true))
	ctx := context.Background()
	r, err := my.Quick(ctx, w, "select * from t_order", "sqlmux", 100)
	if err != nil || len(r.Rows) != 100 || !r.Truncated {
		t.Fatalf("select * from t_order: %d rows, truncated %v, %v", len(r.Rows), r.Truncated, err)
	}
	if r, err = my.Quick(ctx, w, "  -- a comment\nselect * from t_user", "sqlmux", 100); err != nil || len(r.Rows) != 50 || r.Truncated {
		t.Errorf("t_user: %d rows, truncated %v, %v", len(r.Rows), r.Truncated, err)
	}
	if r, err = my.Quick(ctx, w, "(select * from t_order limit 200)", "sqlmux", 100); err != nil || len(r.Rows) != 100 || !r.Truncated {
		t.Errorf("its own LIMIT: %d rows, truncated %v, %v", len(r.Rows), r.Truncated, err)
	}
	// the rest run as they are
	if r, err = my.Quick(ctx, w, "explain select * from t_order", "sqlmux", 100); err != nil || len(r.Rows) == 0 {
		t.Errorf("explain: %v %v", r.Rows, err)
	}
	// the tree's database is the current one
	if r, err = my.Quick(ctx, w, "select count(*) from agent", "agentable", 100); err != nil || r.Rows[0][0].S != "3" {
		t.Errorf("agentable's agent: %v %v", r.Rows, err)
	}
	if r, err = my.Quick(ctx, w, "show tables", "agentable", 100); err != nil || len(r.Rows) == 0 {
		t.Errorf("show tables: %v %v", r.Rows, err)
	}
	// one statement at a time: no multiStatements
	if _, err := my.Quick(ctx, w, "select 1; delete from t_order", "sqlmux", 100); errno(err) != 1064 {
		t.Errorf("two statements: %v", err)
	}
	// cancelled, the connection goes on, out of the transaction
	time.AfterFunc(200*time.Millisecond, w.Cancel)
	if _, err := my.Quick(ctx, w, "select sleep(10)", "sqlmux", 100); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled: %v", err)
	}
	if r, err = my.Quick(ctx, w, "select 1", "sqlmux", 100); err != nil || r.Rows[0][0].S != "1" {
		t.Errorf("after the cancel: %v %v", r.Rows, err)
	}
}

// A write is refused by the read-only transaction and changes nothing
// (the app does not send what IsRead calls a write, F-05). It runs on a
// database of its own (AGENTS.md「集成测试环境」).
func TestQuickRefusesWrites(t *testing.T) {
	ctx := context.Background()
	dsn := OwnDB(t)
	own, _, err := Connect(ctx, dsn, "", false)
	if err != nil {
		t.Fatal(err)
	}
	defer own.Close()
	if _, err := own.Query(ctx, "create table t (x int)"); err != nil {
		t.Fatal(err)
	}
	if _, err := own.Query(ctx, "insert into t values (1), (2), (3)"); err != nil {
		t.Fatal(err)
	}
	meta, _, err := Connect(ctx, dsn, "", true)
	if err != nil {
		t.Fatal(err)
	}
	w := db.NewWorker(meta)
	defer w.Close()
	cfg, _ := Config(dsn, "")
	if _, err := my.Quick(ctx, w, "delete from t", cfg.DBName, 100); my.ErrorOf(err).Code != "25006" {
		t.Errorf("delete: %v", err)
	}
	if r, _ := own.Query(ctx, "select count(*) from t"); r.Rows[0][0].S != "3" {
		t.Errorf("rows left: %v", r.Rows)
	}
}
