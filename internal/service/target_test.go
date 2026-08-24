package service_test

import (
	"context"
	"testing"

	"observatory/internal/model"
)

// TestExposureSequence 曝光序列不得跳号、不得重复。
func TestExposureSequence(t *testing.T) {
	s := newStack(t)
	ctx := context.Background()

	site, _ := s.svc.Sites.CreateSite(ctx, model.Site{Code: "S5", Name: "站5", Latitude: -80, Longitude: 70, AltitudeM: 4000})
	in, _ := s.svc.Instruments.CreateInstrument(ctx, model.Instrument{
		SiteID: site.ID, Code: "I6", Name: "仪6", Kind: "imager", TempMinMK: 250, TempMaxMK: 350,
	})
	win, _ := s.svc.Windows.Apply(ctx, model.ObservationWindow{
		InstrumentID: in.ID, Title: "W5",
		StartAt: testEpoch, EndAt: testEpoch.AddDate(0, 0, 1),
	}, "sch")
	tgt, _, err := s.svc.Targets.Schedule(ctx, win.ID, model.Target{
		Name: "NGC", RaDeg: 10, DecDeg: 20, Priority: 1, ExposureGoal: 5, IdempotencyKey: "k-ngc",
	}, "sch")
	if err != nil {
		t.Fatalf("排程失败: %v", err)
	}

	// 首个曝光必须从 1 开始。
	if _, err := s.svc.Targets.AddExposure(ctx, tgt.ID, 3, 60, "K", "op"); err == nil {
		t.Fatalf("跳号曝光（首个序号为 3）应被拒绝")
	}
	if _, err := s.svc.Targets.AddExposure(ctx, tgt.ID, 1, 60, "K", "op"); err != nil {
		t.Fatalf("序号 1 应被接受: %v", err)
	}
	// 重复序号。
	if _, err := s.svc.Targets.AddExposure(ctx, tgt.ID, 1, 60, "K", "op"); err == nil {
		t.Fatalf("重复序号应被拒绝")
	}
	// 跳号。
	if _, err := s.svc.Targets.AddExposure(ctx, tgt.ID, 5, 60, "K", "op"); err == nil {
		t.Fatalf("跳号（1→5）应被拒绝")
	}
	if _, err := s.svc.Targets.AddExposure(ctx, tgt.ID, 2, 60, "K", "op"); err != nil {
		t.Fatalf("序号 2 应被接受: %v", err)
	}
	exps, _ := s.svc.Targets.ListExposures(ctx, tgt.ID, page100())
	if len(exps) != 2 || exps[0].SeqNo != 1 || exps[1].SeqNo != 2 {
		t.Fatalf("曝光序列异常: %+v", exps)
	}
}

// TestScheduleFrozenAfterApprove 窗口批准后目标优先级冻结，禁止新增排程。
func TestScheduleFrozenAfterApprove(t *testing.T) {
	s := newStack(t)
	ctx := context.Background()
	st := seedToActiveWindow(t, s)

	if _, _, err := s.svc.Targets.Schedule(ctx, st.windowID, model.Target{
		Name: "LATE", RaDeg: 1, DecDeg: 1, Priority: 9, ExposureGoal: 1, IdempotencyKey: "k-late",
	}, "sch"); err == nil {
		t.Fatalf("窗口批准后应拒绝新增排程")
	}
}

// TestInstrumentFrozenByWindow 窗口批准/激活期间仪器与通道配置冻结。
func TestInstrumentFrozenByWindow(t *testing.T) {
	s := newStack(t)
	ctx := context.Background()
	st := seedToActiveWindow(t, s)

	if _, err := s.svc.Instruments.UpdateInstrument(ctx, st.instrumentID, 2, "新名", "", 0, 0, "op"); err == nil {
		t.Fatalf("窗口激活期间仪器配置应被冻结")
	}
	if _, err := s.svc.Instruments.AddChannel(ctx, st.instrumentID, model.DetectorChannel{
		ChannelNo: 2, Name: "辅助", WavelengthNM: 500,
	}); err == nil {
		t.Fatalf("窗口激活期间禁止新增通道")
	}
	if _, err := s.svc.Instruments.UpdateChannel(ctx, st.channelID, 1, "", 2.0, 0, "", "op"); err == nil {
		t.Fatalf("窗口激活期间通道配置应被冻结")
	}
}
