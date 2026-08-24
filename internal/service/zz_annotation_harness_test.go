package service_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"observatory/internal/clock"
	"observatory/internal/domain"
	"observatory/internal/model"
	"observatory/internal/service"
	"observatory/internal/store/sqlite"
)

func newBug13Stack(t *testing.T) (*service.Services, *sqlite.DB) {
	t.Helper()
	db, err := sqlite.Open(filepath.Join(t.TempDir(), "bug13.db"))
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return service.New(db, clock.NewFake(time.Date(2026, 1, 2, 12, 0, 0, 0, time.UTC)), 3), db
}

func seedBug13Anomaly(t *testing.T, svc *service.Services) int64 {
	t.Helper()
	ctx := context.Background()
	site, err := svc.Sites.CreateSite(ctx, model.Site{Code: "B13S", Name: "site", Latitude: -80, Longitude: 70, AltitudeM: 4000})
	if err != nil {
		t.Fatal(err)
	}
	inst, err := svc.Instruments.CreateInstrument(ctx, model.Instrument{SiteID: site.ID, Code: "B13I", Name: "instrument", Kind: "imager", TempMinMK: 250, TempMaxMK: 350})
	if err != nil {
		t.Fatal(err)
	}
	a, err := svc.Anomalies.CreateManual(ctx, nil, inst.ID, domain.AnomalyManual, "retry boundary", "operator")
	if err != nil {
		t.Fatal(err)
	}
	return a.ID
}

func TestBug13Trigger(t *testing.T) {
	svc, db := newBug13Stack(t)
	id := seedBug13Anomaly(t, svc)
	if _, err := db.SQL.ExecContext(context.Background(), `UPDATE anomalies SET status=? WHERE id=?`, domain.AnomalyResolved, id); err != nil {
		t.Fatal(err)
	}
	got, err := svc.Anomalies.Resolve(context.Background(), id, "operator")
	if err != nil {
		t.Fatalf("same-state retry failed: %v", err)
	}
	if got.Status != domain.AnomalyResolved {
		t.Fatalf("status=%s, want %s", got.Status, domain.AnomalyResolved)
	}
}

func TestBug13Control(t *testing.T) {
	svc, _ := newBug13Stack(t)
	id := seedBug13Anomaly(t, svc)
	if _, err := svc.Anomalies.Resolve(context.Background(), id, "operator"); err == nil {
		t.Fatal("open to resolved jump must remain rejected")
	}
}
