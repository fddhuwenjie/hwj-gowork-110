package service_test

import (
	"context"
	"testing"
	"time"

	"observatory/internal/model"
)

// TestCalibrationCoverageAtFinish 校准有效期必须覆盖整个批次：
// 批次开始时有效，但时钟推进超过 valid_until 后结束批次应失败。
func TestCalibrationCoverageAtFinish(t *testing.T) {
	s := newStack(t)
	ctx := context.Background()
	st := seedToActiveWindow(t, s)

	b, _, err := s.svc.Batches.Start(ctx, st.windowID, st.targetID, "s3://raw/c", "batch-cov", nil, "op")
	if err != nil {
		t.Fatalf("批次开始失败: %v", err)
	}
	// 推进时钟超出校准有效期（72h 之外），但仍在窗口外无妨——Finish 只校验覆盖。
	s.clk.Advance(80 * time.Hour)
	if _, err := s.svc.Batches.Finish(ctx, b.ID, b.Version, "op"); err == nil {
		t.Fatalf("校准有效期未覆盖批次结束时刻，应拒绝冻结")
	}
}

// TestApproveWindowPlanCoverage 窗口批准要求方案有效期覆盖窗口区间。
func TestApproveWindowPlanCoverage(t *testing.T) {
	s := newStack(t)
	ctx := context.Background()

	site, _ := s.svc.Sites.CreateSite(ctx, model.Site{Code: "S3", Name: "站3", Latitude: -80, Longitude: 70, AltitudeM: 4000})
	in, _ := s.svc.Instruments.CreateInstrument(ctx, model.Instrument{
		SiteID: site.ID, Code: "I4", Name: "仪4", Kind: "imager", TempMinMK: 250, TempMaxMK: 350,
	})
	// 方案有效期仅 1 小时，窗口 48 小时。
	plan, _ := s.svc.Calibration.CreatePlan(ctx, model.CalibrationPlan{
		InstrumentID: in.ID, VersionNo: 1,
		ValidFrom: testEpoch.Add(-time.Minute), ValidUntil: testEpoch.Add(time.Hour),
	}, "planner")
	s.svc.Calibration.ApprovePlan(ctx, plan.ID, plan.Version, "approver")
	s.svc.Calibration.ActivatePlan(ctx, plan.ID, plan.Version+1, "approver")

	win, _ := s.svc.Windows.Apply(ctx, model.ObservationWindow{
		InstrumentID: in.ID, Title: "长窗口",
		StartAt: testEpoch, EndAt: testEpoch.Add(48 * time.Hour),
	}, "sch")
	if _, err := s.svc.Windows.Approve(ctx, win.ID, win.Version, plan.ID, "appr"); err == nil {
		t.Fatalf("方案有效期未覆盖窗口时应拒绝批准")
	}
}

// TestCalibrationRecordImmutable 校准记录为不可变证据：仓储层无更新入口，
// 重复登记仅追加新版本且历史可溯。
func TestCalibrationRecordImmutable(t *testing.T) {
	s := newStack(t)
	ctx := context.Background()
	st := seedToActiveWindow(t, s)

	rec, err := s.svc.Calibration.CreateRecord(ctx, model.CalibrationRecord{
		PlanID: st.planID, SourceID: st.sourceID, Result: "fail",
		DeviationPct: 5.5, EvidenceURI: "s3://evidence/cal-2",
	}, "calibrator2")
	if err != nil {
		t.Fatalf("追加校准记录失败: %v", err)
	}
	records, err := s.svc.Calibration.ListRecords(ctx, st.instrumentID, page100())
	if err != nil {
		t.Fatalf("读取校准记录失败: %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("应有 2 条校准记录，实际 %d", len(records))
	}
	if records[0].ID != st.recordID || records[1].ID != rec.ID {
		t.Fatalf("校准记录顺序异常")
	}
	_ = rec
}

// TestApproveRequiresDifferentActor 校准方案审批人不得与创建人相同。
func TestApproveRequiresDifferentActor(t *testing.T) {
	s := newStack(t)
	ctx := context.Background()
	site, _ := s.svc.Sites.CreateSite(ctx, model.Site{Code: "S4", Name: "站4", Latitude: -80, Longitude: 70, AltitudeM: 4000})
	in, _ := s.svc.Instruments.CreateInstrument(ctx, model.Instrument{
		SiteID: site.ID, Code: "I5", Name: "仪5", Kind: "imager", TempMinMK: 250, TempMaxMK: 350,
	})
	plan, _ := s.svc.Calibration.CreatePlan(ctx, model.CalibrationPlan{
		InstrumentID: in.ID, VersionNo: 1,
		ValidFrom: testEpoch.Add(-time.Hour), ValidUntil: testEpoch.Add(time.Hour),
	}, "planner")
	if _, err := s.svc.Calibration.ApprovePlan(ctx, plan.ID, plan.Version, "planner"); err == nil {
		t.Fatalf("创建人与审批人相同应被拒绝")
	}
}
