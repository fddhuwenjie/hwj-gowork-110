package service_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"observatory/internal/domain"
	"observatory/internal/model"
)

// TestQueryPendingCalibration 临近窗口仍未完成校准的仪器。
func TestQueryPendingCalibration(t *testing.T) {
	s := newStack(t)
	ctx := context.Background()

	// 完整链路仪器：已有合格校准记录，不应出现在结果中。
	st := seedToActiveWindow(t, s)

	// 第二台仪器：方案启用但无校准记录，窗口已批准 → 应被查出。
	site, _ := s.svc.Sites.CreateSite(ctx, model.Site{Code: "QS1", Name: "站Q", Latitude: -80, Longitude: 70, AltitudeM: 4000})
	in, _ := s.svc.Instruments.CreateInstrument(ctx, model.Instrument{
		SiteID: site.ID, Code: "QI1", Name: "仪Q", Kind: "imager", TempMinMK: 250, TempMaxMK: 350,
	})
	plan, _ := s.svc.Calibration.CreatePlan(ctx, model.CalibrationPlan{
		InstrumentID: in.ID, VersionNo: 1,
		ValidFrom: testEpoch.Add(-time.Hour), ValidUntil: testEpoch.Add(96 * time.Hour),
	}, "planner")
	s.svc.Calibration.ApprovePlan(ctx, plan.ID, plan.Version, "approver")
	s.svc.Calibration.ActivatePlan(ctx, plan.ID, plan.Version+1, "approver")
	win, _ := s.svc.Windows.Apply(ctx, model.ObservationWindow{
		InstrumentID: in.ID, Title: "缺校准窗口",
		StartAt: testEpoch.Add(2 * time.Hour), EndAt: testEpoch.Add(6 * time.Hour),
	}, "sch")
	if _, err := s.svc.Windows.Approve(ctx, win.ID, win.Version, plan.ID, "appr"); err != nil {
		t.Fatalf("批准失败: %v", err)
	}

	rows, err := s.svc.Queries.InstrumentsPendingCalibration(ctx, 72, page100())
	if err != nil {
		t.Fatalf("查询失败: %v", err)
	}
	if len(rows) != 1 || rows[0].WindowID != win.ID {
		t.Fatalf("应仅查出缺校准的窗口，实际 %+v", rows)
	}
	for _, r := range rows {
		if r.WindowID == st.windowID {
			t.Fatalf("已完成校准的窗口不应出现")
		}
	}
	// 时间窗收窄至 1 小时（窗口 2 小时后开始）→ 查不出。
	rows, _ = s.svc.Queries.InstrumentsPendingCalibration(ctx, 1, page100())
	if len(rows) != 0 {
		t.Fatalf("超出临近范围的窗口不应出现")
	}
}

// TestQueryCryoTrend 低温异常趋势。
func TestQueryCryoTrend(t *testing.T) {
	s := newStack(t)
	ctx := context.Background()
	st := seedToActiveWindow(t, s)

	// 观测中写入两条越界读数。
	s.svc.Batches.Start(ctx, st.windowID, st.targetID, "s3://raw/t", "batch-trend", nil, "op")
	if _, _, err := s.svc.Cryo.AddReading(ctx, st.cryoID, 999, 0, time.Time{}, "oor-t1", "op"); err != nil {
		t.Fatalf("越界读数失败: %v", err)
	}
	if _, _, err := s.svc.Cryo.AddReading(ctx, st.cryoID, 100, 0, time.Time{}, "oor-t2", "op"); err != nil {
		t.Fatalf("越界读数失败: %v", err)
	}

	rows, err := s.svc.Queries.CryoAnomalyTrend(ctx, 7, page100())
	if err != nil {
		t.Fatalf("趋势查询失败: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("应聚合为 1 行（同日同系统），实际 %d", len(rows))
	}
	if rows[0].OutOfRange != 2 || rows[0].MaxTempMK != 999 || rows[0].MinTempMK != 100 {
		t.Fatalf("聚合结果异常: %+v", rows[0])
	}
}

// TestQueryCryoTrendLateArrival 跨 UTC 边界后补传的越界读数必须按实际 recorded_at
// 归入观测发生日，而不可因迟到上传被归到新的一天。
func TestQueryCryoTrendLateArrival(t *testing.T) {
	s := newStack(t)
	ctx := context.Background()
	st := seedToActiveWindow(t, s)
	s.svc.Batches.Start(ctx, st.windowID, st.targetID, "s3://raw/t2", "batch-trend-late", nil, "op")

	// 两条越界读数：实际观测发生在 D1 临近 UTC 午夜，第二条迟到补传至 D2。
	d1 := testEpoch
	d2 := testEpoch.AddDate(0, 0, 1)
	lateRecorded := d1.Add(-2 * time.Hour) // 22:00 D-1（前一日）
	if _, _, err := s.svc.Cryo.AddReading(ctx, st.cryoID, 999, 0, lateRecorded, "oor-late", "op"); err != nil {
		t.Fatalf("迟到读数失败: %v", err)
	}
	// 推进时钟跨越 UTC 日期边界后再查询，模拟"按到达时刻归到新的一天"。
	s.clk.Advance(48 * time.Hour)

	rows, err := s.svc.Queries.CryoAnomalyTrend(ctx, 7, page100())
	if err != nil {
		t.Fatalf("趋势查询失败: %v", err)
	}
	// 应归入观测发生日（lateRecorded 当日），而非时钟推进后的新一天。
	wantDay := lateRecorded.UTC().Format("2006-01-02")
	for _, r := range rows {
		if r.Day == wantDay && r.OutOfRange != 1 {
			t.Fatalf("观测发生日应聚合计 1 条越界，实际 %+v", r)
		}
		if r.Day == d2.Format("2006-01-02") && r.OutOfRange > 0 {
			t.Fatalf("迟到读数不应归到到达日 %s，实际 %+v", r.Day, r)
		}
	}
}

// TestQueryTargetConflicts 目标排程冲突：同仪器已批准窗口时间重叠。
func TestQueryTargetConflicts(t *testing.T) {
	s := newStack(t)
	ctx := context.Background()
	st := seedToActiveWindow(t, s)

	// 第二个窗口与已激活窗口重叠（批准阶段允许竞争）。
	win2, _ := s.svc.Windows.Apply(ctx, model.ObservationWindow{
		InstrumentID: st.instrumentID, Title: "重叠窗口",
		StartAt: testEpoch.Add(2 * time.Hour), EndAt: testEpoch.Add(30 * time.Hour),
	}, "sch")
	// 冻结需要 active 方案；原方案已被封存，新建 v2 方案。
	plan2, _ := s.svc.Calibration.CreatePlan(ctx, model.CalibrationPlan{
		InstrumentID: st.instrumentID, VersionNo: 2,
		ValidFrom: testEpoch.Add(-time.Hour), ValidUntil: testEpoch.Add(96 * time.Hour),
	}, "planner")
	s.svc.Calibration.ApprovePlan(ctx, plan2.ID, plan2.Version, "approver")
	s.svc.Calibration.ActivatePlan(ctx, plan2.ID, plan2.Version+1, "approver")
	if _, err := s.svc.Windows.Approve(ctx, win2.ID, win2.Version, plan2.ID, "appr"); err != nil {
		t.Fatalf("批准失败: %v", err)
	}

	rows, err := s.svc.Queries.TargetConflicts(ctx, page100())
	if err != nil {
		t.Fatalf("冲突查询失败: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("应查出 1 对冲突窗口，实际 %d", len(rows))
	}
	pair := map[int64]bool{rows[0].WindowAID: true, rows[0].WindowBID: true}
	if !pair[st.windowID] || !pair[win2.ID] {
		t.Fatalf("冲突对应为两个重叠窗口，实际 %+v", rows[0])
	}
}

// TestQueryQualityDecline 质量指标连续下降批次。
func TestQueryQualityDecline(t *testing.T) {
	s := newStack(t)
	ctx := context.Background()
	st := seedToActiveWindow(t, s)

	// 四个批次，评分严格递减但全部达标。
	snrs := []float64{30, 20, 15, 12}
	for i, snr := range snrs {
		b := runBatch(t, s, st, "batch-decline-"+strings.Repeat("x", i+1))
		m, err := s.svc.Metrics.Add(ctx, b.ID, snr, 0.5, 50, "analyst")
		if err != nil {
			t.Fatalf("指标登记失败: %v", err)
		}
		if !m.Passed {
			t.Fatalf("批次 %d 指标应达标，score=%.2f", i, m.Score)
		}
		if _, err := s.svc.Metrics.Seal(ctx, m.ID, "analyst"); err != nil {
			t.Fatalf("封存失败: %v", err)
		}
	}

	rows, err := s.svc.Queries.QualityDecline(ctx, 3, page100())
	if err != nil {
		t.Fatalf("下降查询失败: %v", err)
	}
	if len(rows) != 1 || rows[0].InstrumentID != st.instrumentID {
		t.Fatalf("应查出该仪器连续下降，实际 %+v", rows)
	}
	if len(rows[0].Scores) != 4 {
		t.Fatalf("应包含 4 个评分点，实际 %v", rows[0].Scores)
	}
	// min_consecutive=4 需要 4 次相邻下降（5 个点），不应命中。
	rows, _ = s.svc.Queries.QualityDecline(ctx, 4, page100())
	if len(rows) != 0 {
		t.Fatalf("4 次连续下降不应命中")
	}
}

// TestQueryPendingRetests 待复测：越界隔离（无复测批次）应被查出；封存失败（已自动复测）不应出现。
func TestQueryPendingRetests(t *testing.T) {
	s := newStack(t)
	ctx := context.Background()
	st := seedToActiveWindow(t, s)

	// 隔离批次（温度越界路径，不自动创建复测）。
	b, _, _ := s.svc.Batches.Start(ctx, st.windowID, st.targetID, "s3://raw/pr", "batch-pr", nil, "op")
	s.svc.Cryo.AddReading(ctx, st.cryoID, 999, 0, time.Time{}, "oor-pr", "op")
	bAfter, _ := s.svc.Batches.Get(ctx, b.ID)
	if bAfter.Status != domain.BatchIsolated {
		t.Fatalf("批次应隔离，实际 %s", bAfter.Status)
	}

	rows, err := s.svc.Queries.PendingRetests(ctx, page100())
	if err != nil {
		t.Fatalf("待复测查询失败: %v", err)
	}
	found := false
	for _, r := range rows {
		if r.BatchID == b.ID {
			found = true
		}
	}
	if !found {
		t.Fatalf("隔离批次应出现在待复测列表")
	}
}

// TestQueryExpiredReleases 已过期发布许可。
func TestQueryExpiredReleases(t *testing.T) {
	s := newStack(t)
	ctx := context.Background()
	st := seedToActiveWindow(t, s)
	b := runBatch(t, s, st, "batch-exp")

	m, _ := s.svc.Metrics.Add(ctx, b.ID, 20, 2, 100, "analyst")
	s.svc.Metrics.Seal(ctx, m.ID, "analyst")
	arch, _, _ := s.svc.Archives.Request(ctx, b.ID, repeat64("f"), 10, "arch-exp", "op")
	s.svc.Archives.Verify(ctx, arch.ID, "verifier")
	permit, err := s.svc.Releases.Submit(ctx, arch.ID, "成果", testEpoch.Add(2*time.Hour), "alice")
	if err != nil {
		t.Fatalf("许可提交失败: %v", err)
	}

	// 未过期：查不出。
	rows, _ := s.svc.Queries.ExpiredReleases(ctx, page100())
	if len(rows) != 0 {
		t.Fatalf("未过期许可不应出现")
	}
	// 推进时钟过期：查出。
	s.clk.Advance(3 * time.Hour)
	rows, err = s.svc.Queries.ExpiredReleases(ctx, page100())
	if err != nil {
		t.Fatalf("过期查询失败: %v", err)
	}
	if len(rows) != 1 || rows[0].PermitID != permit.ID {
		t.Fatalf("应查出过期许可，实际 %+v", rows)
	}
	// 过期许可禁止发布。
	if _, _, err := s.svc.Archives.VerifyAndPublish(ctx, arch.ID, permit.ID, "bob"); err == nil {
		t.Fatalf("过期许可应拒绝发布")
	}
}
