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

func TestBug04Trigger(t *testing.T) {
	svc, db, clk := newBugStack(t)
	ctx := context.Background()
	now := clk.Now()
	site, _ := svc.Sites.CreateSite(ctx, model.Site{Code: "B04S", Name: "站", Latitude: -80, Longitude: 70, AltitudeM: 4000})
	inst, _ := svc.Instruments.CreateInstrument(ctx, model.Instrument{SiteID: site.ID, Code: "B04I", Name: "仪", Kind: "imager", TempMinMK: 250, TempMaxMK: 350})
	plan, _ := svc.Calibration.CreatePlan(ctx, model.CalibrationPlan{InstrumentID: inst.ID, VersionNo: 1, ValidFrom: now.Add(-time.Hour), ValidUntil: now.Add(48 * time.Hour)}, "p")
	svc.Calibration.ApprovePlan(ctx, plan.ID, plan.Version, "a")
	svc.Calibration.ActivatePlan(ctx, plan.ID, plan.Version+1, "a")
	win, _ := svc.Windows.Apply(ctx, model.ObservationWindow{InstrumentID: inst.ID, Title: "W", StartAt: now.Add(time.Hour), EndAt: now.Add(2 * time.Hour)}, "s")
	if _, err := db.SQL.ExecContext(ctx, `CREATE TRIGGER reject_bug04_audit BEFORE INSERT ON audit_log BEGIN SELECT RAISE(ABORT,'audit blocked'); END`); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Windows.Approve(ctx, win.ID, win.Version, plan.ID, "a"); err == nil {
		t.Fatal("approval succeeded despite audit failure")
	}
	got, _ := svc.Windows.Get(ctx, win.ID)
	if got.Status != "applied" || got.FrozenSnapshot != "" {
		t.Fatalf("partial approval persisted: %+v", got)
	}
}
func TestBug04Control(t *testing.T) {}
