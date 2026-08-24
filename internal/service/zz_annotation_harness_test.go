package service_test

import (
	"context"
	"observatory/internal/clock"
	"observatory/internal/service"
	"observatory/internal/store/sqlite"
	"path/filepath"
	"testing"
	"time"
)

import "observatory/internal/repo"

func newBugStack(t *testing.T) (*service.Services, *sqlite.DB, *clock.Fake) {
	t.Helper()
	clk := clock.NewFake(time.Date(2026, 1, 2, 12, 0, 0, 0, time.UTC))
	db, err := sqlite.Open(filepath.Join(t.TempDir(), "bug.db"))
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return service.New(db, clk, 3), db, clk
}

func TestBug06Trigger(t *testing.T) {
	svc, db, clk := newBugStack(t)
	ctx := context.Background()
	now := clk.Now().Format("2006-01-02T15:04:05.000000000Z07:00")
	db.SQL.ExecContext(ctx, "PRAGMA foreign_keys=OFF")
	ins := `INSERT INTO observation_batches(id,window_id,target_id,instrument_id,retest_of_id,object_uri,status,started_at,finished_at,idempotency_key,version,created_at,updated_at) VALUES(?,?,?,?,?,'u',?,?,NULL,?,1,?,?)`
	db.SQL.ExecContext(ctx, ins, 1, 1, 1, 1, nil, "isolated", now, "b1", now, now)
	db.SQL.ExecContext(ctx, ins, 2, 1, 1, 1, 1, "isolated", now, "b2", now, now)
	db.SQL.ExecContext(ctx, ins, 3, 1, 1, 1, nil, "isolated", now, "b3", now, now)
	db.SQL.ExecContext(ctx, ins, 4, 1, 1, 1, nil, "isolated", now, "b4", now, now)
	rows, err := svc.Queries.PendingRetests(ctx, repo.Page{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 4 || rows[0].BatchID != 1 {
		t.Fatalf("isolated retest incorrectly hid original: %+v", rows)
	}
	rows, err = svc.Queries.PendingRetests(ctx, repo.Page{Limit: 1, Cursor: 3})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].BatchID != 4 {
		t.Fatalf("cursor was not preserved: %+v", rows)
	}
}
func TestBug06Control(t *testing.T) {}
