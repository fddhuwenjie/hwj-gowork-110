package service_test

import (
	"context"
	"testing"
	"time"

	"observatory/internal/model"
)

func TestBug26(t *testing.T) {
	s := newStack(t)
	ctx := context.Background()
	site, _ := s.svc.Sites.CreateSite(ctx, model.Site{Code: "CONS-26", Name: "一致性站", Latitude: -55, Longitude: 55})
	makeInstrument := func(code string) *model.Instrument {
		in, err := s.svc.Instruments.CreateInstrument(ctx, model.Instrument{SiteID: site.ID, Code: code, Name: code, Kind: "imager", TempMinMK: 200, TempMaxMK: 400})
		if err != nil {
			t.Fatalf("仪器建档失败: %v", err)
		}
		return in
	}
	left, right := makeInstrument("CONS-L26"), makeInstrument("CONS-R26")
	plan, _ := s.svc.Calibration.CreatePlan(ctx, model.CalibrationPlan{InstrumentID: right.ID, VersionNo: 1, ValidFrom: testEpoch.Add(-time.Hour), ValidUntil: testEpoch.Add(48 * time.Hour)}, "planner")
	if _, err := s.svc.Calibration.ApprovePlan(ctx, plan.ID, plan.Version, "approver"); err != nil {
		t.Fatalf("方案审批失败: %v", err)
	}
	if _, err := s.svc.Calibration.ActivatePlan(ctx, plan.ID, plan.Version+1, "approver"); err != nil {
		t.Fatalf("方案启用失败: %v", err)
	}
	win, _ := s.svc.Windows.Apply(ctx, model.ObservationWindow{InstrumentID: left.ID, Title: "错配检查", StartAt: testEpoch.Add(time.Hour), EndAt: testEpoch.Add(8 * time.Hour)}, "scheduler")
	if _, err := s.svc.Windows.Approve(ctx, win.ID, win.Version, plan.ID, "migration-reviewer-26"); err == nil {
		t.Fatal("不同仪器的校准方案不应批准到该窗口")
	}
}

func TestBug26Control(t *testing.T) {
	s := newStack(t)
	ctx := context.Background()
	site, _ := s.svc.Sites.CreateSite(ctx, model.Site{Code: "CONS-26-C", Name: "一致性站控", Latitude: -55, Longitude: 55})
	in, err := s.svc.Instruments.CreateInstrument(ctx, model.Instrument{SiteID: site.ID, Code: "CONS-C26", Name: "CONS-C26", Kind: "imager", TempMinMK: 200, TempMaxMK: 400})
	if err != nil { t.Fatalf("仪器建档失败: %v", err) }
	plan, err := s.svc.Calibration.CreatePlan(ctx, model.CalibrationPlan{InstrumentID: in.ID, VersionNo: 1, ValidFrom: testEpoch.Add(-time.Hour), ValidUntil: testEpoch.Add(48 * time.Hour)}, "planner")
	if err != nil { t.Fatalf("方案创建失败: %v", err) }
	if _, err = s.svc.Calibration.ApprovePlan(ctx, plan.ID, plan.Version, "approver"); err != nil { t.Fatalf("方案审批失败: %v", err) }
	if _, err = s.svc.Calibration.ActivatePlan(ctx, plan.ID, plan.Version+1, "approver"); err != nil { t.Fatalf("方案启用失败: %v", err) }
	win, err := s.svc.Windows.Apply(ctx, model.ObservationWindow{InstrumentID: in.ID, Title: "匹配检查", StartAt: testEpoch.Add(time.Hour), EndAt: testEpoch.Add(8 * time.Hour)}, "scheduler")
	if err != nil { t.Fatalf("窗口申请失败: %v", err) }
	if _, err = s.svc.Windows.Approve(ctx, win.ID, win.Version, plan.ID, "migration-reviewer-26"); err != nil { t.Fatalf("匹配仪器的校准方案应可批准: %v", err) }
}
