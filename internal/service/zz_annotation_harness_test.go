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

import (
	"observatory/internal/domain"
	"observatory/internal/model"
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

func TestBug03Trigger(t *testing.T) {
	svc, db, clk := newBugStack(t)
	ctx := context.Background()
	now := clk.Now()
	if _, err := db.SQL.ExecContext(ctx, "PRAGMA foreign_keys=OFF"); err != nil {
		t.Fatal(err)
	}
	layout := "2006-01-02T15:04:05.000000000Z07:00"
	_, err := db.SQL.ExecContext(ctx, `INSERT INTO release_permits(id,archive_id,title,status,submitted_by,reviewed_by,submitted_at,reviewed_at,published_at,expires_at,version,created_at,updated_at) VALUES(1,1,'x','pending_review','alice','',?,NULL,NULL,?,2,?,?)`, now.Format(layout), now.Add(time.Hour).Format(layout), now.Format(layout), now.Format(layout))
	if err != nil {
		t.Fatal(err)
	}
	rr := repo.NewReleaseRepo()
	if err := rr.Review(ctx, db.SQL, 1, 1, domain.ReleaseApproved, "bob", now); err == nil {
		t.Fatal("stale review version accepted")
	}
	if _, err := db.SQL.ExecContext(ctx, "UPDATE release_permits SET status='published',version=4 WHERE id=1"); err != nil {
		t.Fatal(err)
	}
	if err := svc.Releases.Revoke(ctx, 1, 3, "carol"); err == nil {
		t.Fatal("stale revoke version accepted")
	}
}
func TestBug03Control(t *testing.T) {
	if err := domain.EnsureDifferentReviewer("alice", "bob"); err != nil {
		t.Fatal(err)
	}
	_ = model.ReleasePermit{}
}
