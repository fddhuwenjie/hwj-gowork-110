package jobs

import (
	"context"
	"observatory/internal/clock"
	"observatory/internal/service"
	"observatory/internal/store/sqlite"
	"path/filepath"
	"testing"
	"time"
)

import (
	"io"
	"log/slog"
	"observatory/internal/repo"
)

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

func TestBug07Trigger(t *testing.T) {
	_, db, clk := newBugStack(t)
	ctx := context.Background()
	jr := repo.NewJobRepo()
	now := clk.Now()
	id, err := jr.Enqueue(ctx, db.SQL, "once", "{}", now, 3, now)
	if err != nil {
		t.Fatal(err)
	}
	if err := jr.MarkDone(ctx, db.SQL, id, now); err != nil {
		t.Fatal(err)
	}
	r := NewRunner(db, clk, time.Millisecond, time.Millisecond, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err := r.Recover(ctx); err != nil {
		t.Fatal(err)
	}
	got, _ := jr.Get(ctx, db.SQL, id)
	if got.Status != "done" {
		t.Fatalf("recover changed terminal job to %s", got.Status)
	}
	if err := jr.Retry(ctx, db.SQL, id, now); err == nil {
		t.Fatal("done job accepted manual retry")
	}
	id2, err := jr.Enqueue(ctx, db.SQL, "advance", "{}", now, 3, now)
	if err != nil {
		t.Fatal(err)
	}
	r.Register("advance", func(context.Context, string) error { clk.Advance(time.Minute); return nil })
	ran, err := r.RunOnce(ctx)
	if err != nil || !ran {
		t.Fatalf("run=%v err=%v", ran, err)
	}
	done, _ := jr.Get(ctx, db.SQL, id2)
	if !done.UpdatedAt.Equal(now.Add(time.Minute)) {
		t.Fatalf("completion time=%s", done.UpdatedAt)
	}
}
func TestBug07Control(t *testing.T) {}
