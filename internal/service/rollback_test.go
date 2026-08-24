package service_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"observatory/internal/apperr"
	"observatory/internal/domain"
	"observatory/internal/model"
)

func asAppErr(err error, target **apperr.Error) bool {
	return errors.As(err, target)
}

// TestActivateRollback 窗口激活事务：低温读数越界导致激活失败时，
// 校准方案封存、窗口状态、仪器状态必须完整回滚。
func TestActivateRollback(t *testing.T) {
	s := newStack(t)
	ctx := context.Background()
	_ = seedToActiveWindow(t, s)

	// 构造另一台仪器：低温已稳定但最新读数越界，激活应在事务中段失败并完整回滚。
	site, _ := s.svc.Sites.CreateSite(ctx, model.Site{Code: "S10", Name: "站10", Latitude: -80, Longitude: 70, AltitudeM: 4000})
	in, _ := s.svc.Instruments.CreateInstrument(ctx, model.Instrument{
		SiteID: site.ID, Code: "I10", Name: "仪10", Kind: "imager", TempMinMK: 250, TempMaxMK: 350,
	})
	cryo, _ := s.svc.Cryo.RegisterSystem(ctx, in.ID, "制冷机", 300)
	plan, _ := s.svc.Calibration.CreatePlan(ctx, model.CalibrationPlan{
		InstrumentID: in.ID, VersionNo: 1,
		ValidFrom: testEpoch.Add(-time.Hour), ValidUntil: testEpoch.Add(72 * time.Hour),
	}, "planner")
	s.svc.Calibration.ApprovePlan(ctx, plan.ID, plan.Version, "approver")
	s.svc.Calibration.ActivatePlan(ctx, plan.ID, plan.Version+1, "approver")
	src, _ := s.svc.Calibration.CreateSource(ctx, model.StandardSource{Code: "SRC10", Name: "源10", FluxJy: 10})
	s.svc.Calibration.CreateRecord(ctx, model.CalibrationRecord{
		PlanID: plan.ID, SourceID: src.ID, Result: "pass", EvidenceURI: "s3://e10",
	}, "cal")
	// 预冷达标后再注入越界读数（仪器处于 ready，不触发观测隔离）。
	s.svc.Cryo.StartPrecool(ctx, cryo.ID, 300, testEpoch.Add(time.Hour), "op")
	s.svc.Cryo.AddReading(ctx, cryo.ID, 300, 0, time.Time{}, "rd-ok", "op")
	s.svc.Cryo.AddReading(ctx, cryo.ID, 999, 0, time.Time{}, "rd-bad", "op") // 最新读数越界

	win, _ := s.svc.Windows.Apply(ctx, model.ObservationWindow{
		InstrumentID: in.ID, Title: "W10",
		StartAt: testEpoch.Add(time.Hour), EndAt: testEpoch.Add(24 * time.Hour),
	}, "sch")
	if _, err := s.svc.Windows.Approve(ctx, win.ID, win.Version, plan.ID, "appr"); err != nil {
		t.Fatalf("批准失败: %v", err)
	}
	// 激活必失败（最新读数越界）。
	if _, err := s.svc.Windows.Activate(ctx, win.ID, win.Version+1, "op"); err == nil {
		t.Fatalf("读数越界应拒绝激活")
	}
	// 回滚校验：方案仍为 active（未封存）。
	planAfter, _ := s.svc.Calibration.GetPlan(ctx, plan.ID)
	if planAfter.Status != domain.PlanActive {
		t.Fatalf("回滚后方案应保持 active，实际 %s", planAfter.Status)
	}
	// 窗口仍为 approved。
	winAfter, _ := s.svc.Windows.Get(ctx, win.ID)
	if winAfter.Status != domain.WindowApproved {
		t.Fatalf("回滚后窗口应保持 approved，实际 %s", winAfter.Status)
	}
	// 仪器仍为 ready。
	inAfter, _ := s.svc.Instruments.GetInstrument(ctx, in.ID)
	if inAfter.Status != domain.InstrumentReady {
		t.Fatalf("回滚后仪器应保持 ready，实际 %s", inAfter.Status)
	}
}

// TestSealRollback 指标封存事务：封存失败（批次未冻结）时不产生异常与复测批次。
func TestSealRollback(t *testing.T) {
	s := newStack(t)
	ctx := context.Background()
	st := seedToActiveWindow(t, s)

	b, _, _ := s.svc.Batches.Start(ctx, st.windowID, st.targetID, "s3://raw/rb", "batch-rb", nil, "op")
	m, _ := s.svc.Metrics.Add(ctx, b.ID, 1, 9, 999, "analyst")
	if _, err := s.svc.Metrics.Seal(ctx, m.ID, "analyst"); err == nil {
		t.Fatalf("未冻结批次封存应失败")
	}
	metric, _ := s.svc.Metrics.ListByBatch(ctx, b.ID, page100())
	if len(metric) != 1 || metric[0].Sealed {
		t.Fatalf("回滚后指标应保持未封存")
	}
	anomalies, _ := s.svc.Anomalies.List(ctx, st.instrumentID, "", page100())
	if len(anomalies) != 0 {
		t.Fatalf("回滚后不应产生异常，实际 %d 条", len(anomalies))
	}
}

// TestPublishRollback 归档校验与成果发布事务：
// 对归档A执行校验后，因许可属于归档B而失败——校验步骤的更新必须随事务回滚。
func TestPublishRollback(t *testing.T) {
	s := newStack(t)
	ctx := context.Background()
	st := seedToActiveWindow(t, s)

	// 归档A：保持 pending。
	b1 := runBatch(t, s, st, "batch-prb-a")
	m1, _ := s.svc.Metrics.Add(ctx, b1.ID, 20, 2, 100, "analyst")
	s.svc.Metrics.Seal(ctx, m1.ID, "analyst")
	archA, _, err := s.svc.Archives.Request(ctx, b1.ID, repeat64("d"), 10, "arch-prb-a", "op")
	if err != nil {
		t.Fatalf("归档A请求失败: %v", err)
	}

	// 归档B：完成校验并取得已复核许可。
	b2 := runBatch(t, s, st, "batch-prb-b")
	m2, _ := s.svc.Metrics.Add(ctx, b2.ID, 20, 2, 100, "analyst")
	s.svc.Metrics.Seal(ctx, m2.ID, "analyst")
	archB, _, err := s.svc.Archives.Request(ctx, b2.ID, repeat64("e"), 10, "arch-prb-b", "op")
	if err != nil {
		t.Fatalf("归档B请求失败: %v", err)
	}
	if _, err := s.svc.Archives.Verify(ctx, archB.ID, "verifier"); err != nil {
		t.Fatalf("归档B校验失败: %v", err)
	}
	permitB, err := s.svc.Releases.Submit(ctx, archB.ID, "成果B", testEpoch.Add(time.Hour), "alice")
	if err != nil {
		t.Fatalf("许可B提交失败: %v", err)
	}
	if _, err := s.svc.Releases.Review(ctx, permitB.ID, permitB.Version, true, "bob"); err != nil {
		t.Fatalf("许可B复核失败: %v", err)
	}

	// 归档A（pending）+ 许可B（已复核但属于归档B）→ 事务中段失败。
	if _, _, err := s.svc.Archives.VerifyAndPublish(ctx, archA.ID, permitB.ID, "carol"); err == nil {
		t.Fatalf("许可不属于归档A，应拒绝发布")
	}
	// 回滚校验：归档A的 verify 更新必须回滚为 pending。
	archAfter, _ := s.svc.Archives.Get(ctx, archA.ID)
	if archAfter.Status != domain.ArchivePending {
		t.Fatalf("回滚后归档A应保持 pending，实际 %s", archAfter.Status)
	}
	permitAfter, _ := s.svc.Releases.Get(ctx, permitB.ID)
	if permitAfter.Status != domain.ReleaseApproved {
		t.Fatalf("回滚后许可B应保持 approved，实际 %s", permitAfter.Status)
	}
}

func repeat64(ch string) string {
	out := ""
	for len(out) < 64 {
		out += ch
	}
	return out
}
