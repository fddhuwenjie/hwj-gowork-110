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

func TestBug08Trigger(t *testing.T) {
	svc, db, clk := newBugStack(t)
	ctx := context.Background()
	now := clk.Now().Format("2006-01-02T15:04:05.000000000Z07:00")
	site, _ := svc.Sites.CreateSite(ctx, model.Site{Code: "B08S", Name: "站", Latitude: -80, Longitude: 70, AltitudeM: 4000})
	i1, _ := svc.Instruments.CreateInstrument(ctx, model.Instrument{SiteID: site.ID, Code: "B08I1", Name: "甲", Kind: "imager", TempMinMK: 250, TempMaxMK: 350})
	i2, _ := svc.Instruments.CreateInstrument(ctx, model.Instrument{SiteID: site.ID, Code: "B08I2", Name: "乙", Kind: "imager", TempMinMK: 250, TempMaxMK: 350})
	db.SQL.ExecContext(ctx, "PRAGMA foreign_keys=OFF")
	_, err := db.SQL.ExecContext(ctx, `INSERT INTO observation_batches(id,window_id,target_id,instrument_id,object_uri,status,started_at,idempotency_key,version,created_at,updated_at) VALUES(1,1,1,?,'u','frozen',?,'b',1,?,?)`, i1.ID, now, now, now)
	if err != nil {
		t.Fatal(err)
	}
	bid := int64(1)
	a, err := svc.Anomalies.CreateManual(ctx, &bid, i2.ID, domain.AnomalyManual, "manual", "op")
	if err != nil {
		t.Fatal(err)
	}
	if a.InstrumentID != i1.ID {
		t.Fatalf("batch instrument lost: got %d want %d", a.InstrumentID, i1.ID)
	}
	b, err := svc.Anomalies.CreateManual(ctx, nil, i2.ID, domain.AnomalyManual, "standalone", "op")
	if err != nil {
		t.Fatal(err)
	}
	if b.InstrumentID != i2.ID {
		t.Fatalf("standalone instrument changed: got %d want %d", b.InstrumentID, i2.ID)
	}
}
func TestBug08Control(t *testing.T) {}
