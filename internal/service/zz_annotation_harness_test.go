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

func TestBug05Trigger(t *testing.T) {
	svc, _, clk := newBugStack(t)
	ctx := context.Background()
	site, _ := svc.Sites.CreateSite(ctx, model.Site{Code: "B05S", Name: "站", Latitude: -80, Longitude: 70, AltitudeM: 4000})
	inst, _ := svc.Instruments.CreateInstrument(ctx, model.Instrument{SiteID: site.ID, Code: "B05I", Name: "仪", Kind: "imager", TempMinMK: 250, TempMaxMK: 350})
	cryo, _ := svc.Cryo.RegisterSystem(ctx, inst.ID, "冷机", 300)
	observed := clk.Now().Add(-36 * time.Hour)
	if _, _, err := svc.Cryo.AddReading(ctx, cryo.ID, 999, 0, observed, "late", "op"); err != nil {
		t.Fatal(err)
	}
	rows, err := svc.Queries.CryoAnomalyTrend(ctx, 7, repo.Page{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	want := observed.Format("2006-01-02")
	if len(rows) != 1 || rows[0].Day != want {
		t.Fatalf("rows=%+v want day=%s", rows, want)
	}
}
func TestBug05Control(t *testing.T) { _ = time.UTC }
