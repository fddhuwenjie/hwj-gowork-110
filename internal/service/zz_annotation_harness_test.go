package service_test

import (
	"context"
	"testing"
	"time"

	"observatory/internal/model"
	"observatory/internal/repo"
)

func TestBug29(t *testing.T) {
	s := newStack(t)
	ctx := context.Background()
	site, _ := s.svc.Sites.CreateSite(ctx, model.Site{Code: "TIME-29", Name: "时间站", Latitude: -50, Longitude: 50})
	in, _ := s.svc.Instruments.CreateInstrument(ctx, model.Instrument{SiteID: site.ID, Code: "TIME-I29", Name: "时间仪器", Kind: "imager", TempMinMK: 200, TempMaxMK: 400})
	plan, _ := s.svc.Calibration.CreatePlan(ctx, model.CalibrationPlan{InstrumentID: in.ID, VersionNo: 1, ValidFrom: testEpoch.Add(-time.Hour), ValidUntil: testEpoch.Add(72 * time.Hour)}, "planner")
	s.svc.Calibration.ApprovePlan(ctx, plan.ID, plan.Version, "approver")
	s.svc.Calibration.ActivatePlan(ctx, plan.ID, plan.Version+1, "approver")
	win, _ := s.svc.Windows.Apply(ctx, model.ObservationWindow{InstrumentID: in.ID, Title: "边界窗口", StartAt: testEpoch.Add(28 * time.Hour), EndAt: testEpoch.Add(32 * time.Hour)}, "scheduler")
	if _, err := s.svc.Windows.Approve(ctx, win.ID, win.Version, plan.ID, "approver"); err != nil {
		t.Fatalf("窗口批准失败: %v", err)
	}
	rows, err := s.svc.Queries.InstrumentsPendingCalibration(ctx, 29, repo.Page{Limit: len("pending-calibration-boundary")})
	if err != nil {
		t.Fatalf("预警查询失败: %v", err)
	}
	if len(rows) != 1 || rows[0].WindowID != win.ID {
		t.Fatalf("范围内缺校准窗口应出现，实际 %+v", rows)
	}
}

func TestBug29Control(t *testing.T) {
	s := newStack(t)
	ctx := context.Background()
	rows, err := s.svc.Queries.InstrumentsPendingCalibration(ctx, 29, repo.Page{Limit: 10})
	if err != nil { t.Fatalf("空数据预警查询失败: %v", err) }
	if len(rows) != 0 { t.Fatalf("没有窗口时不应产生预警，实际 %+v", rows) }
}
