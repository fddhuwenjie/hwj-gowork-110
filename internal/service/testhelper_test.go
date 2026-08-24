package service_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"observatory/internal/clock"
	"observatory/internal/model"
	"observatory/internal/service"
	"observatory/internal/store/sqlite"
)

// testEpoch 为测试基准时刻。
var testEpoch = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

// stack 为一套测试服务栈。
type stack struct {
	svc *service.Services
	clk *clock.Fake
	db  *sqlite.DB
}

// newStack 使用真实临时 SQLite 文件创建服务栈（禁止 :memory:）。
func newStack(t *testing.T) *stack {
	t.Helper()
	clk := clock.NewFake(testEpoch)
	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := sqlite.Open(dbPath)
	if err != nil {
		t.Fatalf("打开临时数据库失败: %v", err)
	}
	if err := db.Migrate(context.Background()); err != nil {
		t.Fatalf("迁移失败: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return &stack{svc: service.New(db, clk, 3), clk: clk, db: db}
}

// chainState 记录业务链各阶段产物。
type chainState struct {
	siteID       int64
	instrumentID int64
	channelID    int64
	cryoID       int64
	planID       int64
	sourceID     int64
	recordID     int64
	windowID     int64
	targetID     int64
}

// seedToActiveWindow 驱动完整业务链至窗口激活：
// 站点→仪器→通道→低温→方案(审批/启用)→标准源→校准记录→预冷→达标读数→窗口→目标→曝光→批准→激活。
func seedToActiveWindow(t *testing.T, s *stack) chainState {
	t.Helper()
	ctx := context.Background()
	var st chainState

	site, err := s.svc.Sites.CreateSite(ctx, model.Site{
		Code: "DOME-A", Name: "昆仑站", Latitude: -80.25, Longitude: 77.06, AltitudeM: 4093,
	})
	if err != nil {
		t.Fatalf("建站失败: %v", err)
	}
	st.siteID = site.ID

	in, err := s.svc.Instruments.CreateInstrument(ctx, model.Instrument{
		SiteID: site.ID, Code: "CryoCam-1", Name: "低温相机一号", Kind: "imager",
		TempMinMK: 250, TempMaxMK: 350,
	})
	if err != nil {
		t.Fatalf("仪器建档失败: %v", err)
	}
	st.instrumentID = in.ID

	ch, err := s.svc.Instruments.AddChannel(ctx, in.ID, model.DetectorChannel{
		ChannelNo: 1, Name: "主通道", WavelengthNM: 850, Gain: 1.2, Offset: 0.1,
	})
	if err != nil {
		t.Fatalf("通道创建失败: %v", err)
	}
	st.channelID = ch.ID

	cryo, err := s.svc.Cryo.RegisterSystem(ctx, in.ID, "稀释制冷机", 300)
	if err != nil {
		t.Fatalf("低温系统登记失败: %v", err)
	}
	st.cryoID = cryo.ID

	plan, err := s.svc.Calibration.CreatePlan(ctx, model.CalibrationPlan{
		InstrumentID: in.ID, VersionNo: 1, Params: `{"bias":"auto"}`,
		ValidFrom:  testEpoch.Add(-time.Hour),
		ValidUntil: testEpoch.Add(72 * time.Hour),
	}, "planner")
	if err != nil {
		t.Fatalf("方案创建失败: %v", err)
	}
	st.planID = plan.ID
	if _, err := s.svc.Calibration.ApprovePlan(ctx, plan.ID, plan.Version, "approver"); err != nil {
		t.Fatalf("方案审批失败: %v", err)
	}
	if _, err := s.svc.Calibration.ActivatePlan(ctx, plan.ID, plan.Version+1, "approver"); err != nil {
		t.Fatalf("方案启用失败: %v", err)
	}

	src, err := s.svc.Calibration.CreateSource(ctx, model.StandardSource{
		Code: "HD-101", Name: "标准星HD101", FluxJy: 12.5, Spectrum: "A0V",
	})
	if err != nil {
		t.Fatalf("标准源登记失败: %v", err)
	}
	st.sourceID = src.ID

	rec, err := s.svc.Calibration.CreateRecord(ctx, model.CalibrationRecord{
		PlanID: plan.ID, SourceID: src.ID, Result: "pass", DeviationPct: 0.8,
		EvidenceURI: "s3://evidence/cal-1",
	}, "calibrator")
	if err != nil {
		t.Fatalf("校准记录失败: %v", err)
	}
	st.recordID = rec.ID

	if _, err := s.svc.Cryo.StartPrecool(ctx, cryo.ID, 300, testEpoch.Add(30*time.Minute), "operator"); err != nil {
		t.Fatalf("开始预冷失败: %v", err)
	}
	if _, _, err := s.svc.Cryo.AddReading(ctx, cryo.ID, 300, 1e-3, time.Time{}, "rd-1", "operator"); err != nil {
		t.Fatalf("预冷达标读数失败: %v", err)
	}

	win, err := s.svc.Windows.Apply(ctx, model.ObservationWindow{
		InstrumentID: in.ID, Title: "极夜窗口A",
		StartAt: testEpoch.Add(time.Hour), EndAt: testEpoch.Add(24 * time.Hour),
	}, "scheduler")
	if err != nil {
		t.Fatalf("窗口申请失败: %v", err)
	}
	st.windowID = win.ID

	tgt, _, err := s.svc.Targets.Schedule(ctx, win.ID, model.Target{
		Name: "M31", RaDeg: 10.68, DecDeg: 41.27, Priority: 1, ExposureGoal: 2,
		IdempotencyKey: "tgt-m31",
	}, "scheduler")
	if err != nil {
		t.Fatalf("目标排程失败: %v", err)
	}
	st.targetID = tgt.ID

	for _, seq := range []int64{1, 2} {
		if _, err := s.svc.Targets.AddExposure(ctx, tgt.ID, seq, 120, "K", "operator"); err != nil {
			t.Fatalf("曝光 %d 追加失败: %v", seq, err)
		}
	}

	if _, err := s.svc.Windows.Approve(ctx, win.ID, win.Version, plan.ID, "approver"); err != nil {
		t.Fatalf("窗口批准失败: %v", err)
	}
	if _, err := s.svc.Windows.Activate(ctx, win.ID, win.Version+1, "operator"); err != nil {
		t.Fatalf("窗口激活失败: %v", err)
	}
	return st
}

// runBatch 执行一个批次从开始到冻结。
func runBatch(t *testing.T, s *stack, st chainState, key string) *model.ObservationBatch {
	t.Helper()
	ctx := context.Background()
	b, _, err := s.svc.Batches.Start(ctx, st.windowID, st.targetID,
		"s3://raw/"+key, key, nil, "operator")
	if err != nil {
		t.Fatalf("批次开始失败: %v", err)
	}
	b, err = s.svc.Batches.Finish(ctx, b.ID, b.Version, "operator")
	if err != nil {
		t.Fatalf("批次冻结失败: %v", err)
	}
	return b
}
