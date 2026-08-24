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
	"encoding/json"
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

func TestBug09Trigger(t *testing.T) {
	svc, _, clk := newBugStack(t)
	ctx := context.Background()
	now := clk.Now()
	site, _ := svc.Sites.CreateSite(ctx, model.Site{Code: "B09S", Name: "站", Latitude: -80, Longitude: 70, AltitudeM: 4000})
	inst, _ := svc.Instruments.CreateInstrument(ctx, model.Instrument{SiteID: site.ID, Code: "B09I", Name: "仪", Kind: "imager", TempMinMK: 250, TempMaxMK: 350})
	plan, _ := svc.Calibration.CreatePlan(ctx, model.CalibrationPlan{InstrumentID: inst.ID, VersionNo: 2, ValidFrom: now.Add(-time.Hour), ValidUntil: now.Add(48 * time.Hour)}, "p")
	svc.Calibration.ApprovePlan(ctx, plan.ID, plan.Version, "a")
	svc.Calibration.ActivatePlan(ctx, plan.ID, plan.Version+1, "a")
	win, _ := svc.Windows.Apply(ctx, model.ObservationWindow{InstrumentID: inst.ID, Title: "W", StartAt: now.Add(time.Hour), EndAt: now.Add(2 * time.Hour)}, "s")
	svc.Targets.Schedule(ctx, win.ID, model.Target{Name: "T", RaDeg: 1, DecDeg: 1, Priority: 7, ExposureGoal: 1, IdempotencyKey: "t"}, "s")
	got, err := svc.Windows.Approve(ctx, win.ID, win.Version, plan.ID, "a")
	if err != nil {
		t.Fatal(err)
	}
	var snap domain.FreezeSnapshot
	if err := json.Unmarshal([]byte(got.FrozenSnapshot), &snap); err != nil {
		t.Fatal(err)
	}
	if snap.Plan.VersionNo != 2 || len(snap.TargetPriorities) != 1 || snap.TargetPriorities[0].Priority != 7 {
		t.Fatalf("snapshot=%+v", snap)
	}
}
func TestBug09Control(t *testing.T) {}
