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

import "observatory/internal/model"

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

func TestBug10Trigger(t *testing.T) {
	svc, db, _ := newBugStack(t)
	ctx := context.Background()
	site, err := svc.Sites.CreateSite(ctx, model.Site{Code: "B10S", Name: "旧名", Latitude: -80, Longitude: 70, AltitudeM: 4000})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.SQL.ExecContext(ctx, `CREATE TRIGGER reject_bug10_audit BEFORE INSERT ON audit_log BEGIN SELECT RAISE(ABORT,'audit blocked'); END`); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Sites.UpdateSite(ctx, site.ID, site.Version, "新名", -80, 70, 4000, "op"); err == nil {
		t.Fatal("update succeeded despite audit failure")
	}
	got, _ := svc.Sites.GetSite(ctx, site.ID)
	if got.Name != "旧名" || got.Version != site.Version {
		t.Fatalf("partial update persisted: %+v", got)
	}
}
func TestBug10Control(t *testing.T) {
	svc, db, _ := newBugStack(t)
	ctx := context.Background()
	site, err := svc.Sites.CreateSite(ctx, model.Site{Code: "B10C", Name: "原站点", Latitude: -30, Longitude: 120, AltitudeM: 1800})
	if err != nil {
		t.Fatal(err)
	}
	updated, err := svc.Sites.UpdateSite(ctx, site.ID, site.Version, "新站点", -31, 121, 1900, "operator")
	if err != nil {
		t.Fatalf("normal update failed: %v", err)
	}
	if updated.Name != "新站点" || updated.Version != site.Version+1 {
		t.Fatalf("normal update not persisted: %+v", updated)
	}
	var count int
	if err := db.SQL.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM audit_log WHERE entity='site' AND entity_id=? AND action='update' AND actor='operator'`,
		site.ID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("update audit count=%d, want 1", count)
	}
}
