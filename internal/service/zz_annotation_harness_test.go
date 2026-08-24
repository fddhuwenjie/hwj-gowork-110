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

func TestBug01Trigger(t *testing.T) {
	svc, db, _ := newBugStack(t)
	ctx := context.Background()
	site, _ := svc.Sites.CreateSite(ctx, model.Site{Code: "B01S", Name: "站", Latitude: -80, Longitude: 70, AltitudeM: 4000})
	inst, _ := svc.Instruments.CreateInstrument(ctx, model.Instrument{SiteID: site.ID, Code: "B01I", Name: "仪", Kind: "imager", TempMinMK: 250, TempMaxMK: 350})
	a, err := svc.Anomalies.CreateManual(ctx, nil, inst.ID, domain.AnomalyManual, "复测异常", "alice")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.SQL.ExecContext(ctx, "UPDATE anomalies SET status=? WHERE id=?", domain.AnomalyRetestCreated, a.ID); err != nil {
		t.Fatal(err)
	}
	got, err := svc.Anomalies.Resolve(ctx, a.ID, "")
	if err != nil {
		t.Fatalf("retest_created anomaly should resolve after retest handling: %v", err)
	}
	if got.Status != domain.AnomalyResolved {
		t.Fatalf("status=%s want resolved", got.Status)
	}
}
func TestBug01Control(t *testing.T) {
	if !domain.CanTransition(domain.EntityAnomaly, domain.AnomalyOpen, domain.AnomalyClosed) {
		t.Fatal("ordinary close must remain legal")
	}
}
