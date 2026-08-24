package service_test

import (
	"context"
	"testing"
	"time"

	"observatory/internal/domain"
	"observatory/internal/model"
)

// TestTemperatureBoundary 温度边界：恰好等于上下限的读数合法，越界读数触发异常链路。
func TestTemperatureBoundary(t *testing.T) {
	s := newStack(t)
	ctx := context.Background()

	site, _ := s.svc.Sites.CreateSite(ctx, model.Site{Code: "S1", Name: "站1", Latitude: -80, Longitude: 70, AltitudeM: 4000})
	in, err := s.svc.Instruments.CreateInstrument(ctx, model.Instrument{
		SiteID: site.ID, Code: "I1", Name: "仪1", Kind: "imager", TempMinMK: 250, TempMaxMK: 350,
	})
	if err != nil {
		t.Fatalf("建档失败: %v", err)
	}
	cryo, err := s.svc.Cryo.RegisterSystem(ctx, in.ID, "制冷机", 300)
	if err != nil {
		t.Fatalf("低温登记失败: %v", err)
	}
	if _, err := s.svc.Cryo.StartPrecool(ctx, cryo.ID, 300, testEpoch.Add(time.Hour), "op"); err != nil {
		t.Fatalf("预冷失败: %v", err)
	}

	// 边界值：恰好下限。
	if _, _, err := s.svc.Cryo.AddReading(ctx, cryo.ID, 250, 0, time.Time{}, "b-min", "op"); err != nil {
		t.Fatalf("下限边界读数应合法: %v", err)
	}
	cryoSys, _ := s.svc.Cryo.GetSystem(ctx, cryo.ID)
	if cryoSys.Status != domain.CryoStable {
		t.Fatalf("入区间读数后低温应转稳，实际 %s", cryoSys.Status)
	}

	// 非法温度区间建档。
	if _, err := s.svc.Instruments.CreateInstrument(ctx, model.Instrument{
		SiteID: site.ID, Code: "I2", Name: "仪2", Kind: "imager", TempMinMK: 400, TempMaxMK: 300,
	}); err == nil {
		t.Fatalf("下限大于上限应被拒绝")
	}
}

// TestOutOfRangeDuringObservation 观测期间越界读数：低温异常、批次隔离、异常登记。
func TestOutOfRangeDuringObservation(t *testing.T) {
	s := newStack(t)
	ctx := context.Background()
	st := seedToActiveWindow(t, s)

	b, _, err := s.svc.Batches.Start(ctx, st.windowID, st.targetID, "s3://raw/x", "batch-oor", nil, "op")
	if err != nil {
		t.Fatalf("批次开始失败: %v", err)
	}
	// 越界读数（上限 350，写入 999）。
	if _, _, err := s.svc.Cryo.AddReading(ctx, st.cryoID, 999, 0, time.Time{}, "oor-1", "op"); err != nil {
		t.Fatalf("越界读数写入失败: %v", err)
	}
	cryoSys, _ := s.svc.Cryo.GetSystem(ctx, st.cryoID)
	if cryoSys.Status != domain.CryoAbnormal {
		t.Fatalf("低温应转异常，实际 %s", cryoSys.Status)
	}
	batch, _ := s.svc.Batches.Get(ctx, b.ID)
	if batch.Status != domain.BatchIsolated {
		t.Fatalf("批次应被隔离，实际 %s", batch.Status)
	}
	anomalies, err := s.svc.Anomalies.List(ctx, st.instrumentID, domain.AnomalyOpen, page100())
	if err != nil || len(anomalies) == 0 {
		t.Fatalf("应登记越界异常: %v", err)
	}
	if anomalies[0].Kind != domain.AnomalyTempOutOfRange {
		t.Fatalf("异常类型应为 temp_out_of_range，实际 %s", anomalies[0].Kind)
	}
}

// TestActivateRequiresStableCryo 低温未转稳时窗口禁止激活。
func TestActivateRequiresStableCryo(t *testing.T) {
	s := newStack(t)
	ctx := context.Background()

	site, _ := s.svc.Sites.CreateSite(ctx, model.Site{Code: "S2", Name: "站2", Latitude: -80, Longitude: 70, AltitudeM: 4000})
	in, _ := s.svc.Instruments.CreateInstrument(ctx, model.Instrument{
		SiteID: site.ID, Code: "I3", Name: "仪3", Kind: "imager", TempMinMK: 250, TempMaxMK: 350,
	})
	cryo, _ := s.svc.Cryo.RegisterSystem(ctx, in.ID, "制冷机", 300)
	plan, _ := s.svc.Calibration.CreatePlan(ctx, model.CalibrationPlan{
		InstrumentID: in.ID, VersionNo: 1,
		ValidFrom: testEpoch.Add(-time.Hour), ValidUntil: testEpoch.Add(72 * time.Hour),
	}, "planner")
	s.svc.Calibration.ApprovePlan(ctx, plan.ID, plan.Version, "approver")
	s.svc.Calibration.ActivatePlan(ctx, plan.ID, plan.Version+1, "approver")
	src, _ := s.svc.Calibration.CreateSource(ctx, model.StandardSource{Code: "SRC", Name: "源", FluxJy: 10})
	s.svc.Calibration.CreateRecord(ctx, model.CalibrationRecord{
		PlanID: plan.ID, SourceID: src.ID, Result: "pass", EvidenceURI: "s3://e",
	}, "cal")
	// 只开始预冷，不写入达标读数。
	s.svc.Cryo.StartPrecool(ctx, cryo.ID, 300, testEpoch.Add(time.Hour), "op")

	win, _ := s.svc.Windows.Apply(ctx, model.ObservationWindow{
		InstrumentID: in.ID, Title: "W", StartAt: testEpoch.Add(time.Hour), EndAt: testEpoch.Add(2 * time.Hour),
	}, "sch")
	if _, err := s.svc.Windows.Approve(ctx, win.ID, win.Version, plan.ID, "appr"); err != nil {
		t.Fatalf("批准失败: %v", err)
	}
	if _, err := s.svc.Windows.Activate(ctx, win.ID, win.Version+1, "op"); err == nil {
		t.Fatalf("低温未稳定时应拒绝激活")
	}
}
