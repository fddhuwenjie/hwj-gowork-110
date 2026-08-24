package repo_test

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

func TestBug02Trigger(t *testing.T) {
	svc, db, clk := newBugStack(t)
	ctx := context.Background()
	now := clk.Now()
	site, _ := svc.Sites.CreateSite(ctx, model.Site{Code: "B02S", Name: "站", Latitude: -80, Longitude: 70, AltitudeM: 4000})
	inst, _ := svc.Instruments.CreateInstrument(ctx, model.Instrument{SiteID: site.ID, Code: "B02I", Name: "仪", Kind: "imager", TempMinMK: 250, TempMaxMK: 350})
	p1, _ := svc.Calibration.CreatePlan(ctx, model.CalibrationPlan{InstrumentID: inst.ID, VersionNo: 1, ValidFrom: now.Add(-time.Hour), ValidUntil: now.Add(24 * time.Hour)}, "p")
	svc.Calibration.ApprovePlan(ctx, p1.ID, p1.Version, "a")
	svc.Calibration.ActivatePlan(ctx, p1.ID, p1.Version+1, "a")
	src, _ := svc.Calibration.CreateSource(ctx, model.StandardSource{Code: "B02SRC", Name: "源", FluxJy: 1})
	if _, err := svc.Calibration.CreateRecord(ctx, model.CalibrationRecord{PlanID: p1.ID, SourceID: src.ID, Result: domain.CalibrationPass, EvidenceURI: "s3://old"}, "c"); err != nil {
		t.Fatal(err)
	}
	p2, _ := svc.Calibration.CreatePlan(ctx, model.CalibrationPlan{InstrumentID: inst.ID, VersionNo: 2, ValidFrom: now.Add(-time.Hour), ValidUntil: now.Add(24 * time.Hour)}, "p")
	svc.Calibration.ApprovePlan(ctx, p2.ID, p2.Version, "a")
	svc.Calibration.ActivatePlan(ctx, p2.ID, p2.Version+1, "a")
	if _, err := repo.NewCalibrationRepo().LatestPassingRecord(ctx, db.SQL, p2.ID, now); err == nil {
		t.Fatal("old-plan record leaked into new plan")
	}
	if err := domain.EnsureRecordBefore(now.Add(time.Minute), now); err == nil {
		t.Fatal("late record accepted")
	}
}
func TestBug02Control(t *testing.T) {
	now := time.Now()
	if err := domain.EnsureRecordBefore(now.Add(-time.Minute), now); err != nil {
		t.Fatal(err)
	}
}
