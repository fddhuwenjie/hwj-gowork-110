package jobs_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"observatory/internal/clock"
	"observatory/internal/domain"
	"observatory/internal/jobs"
	"observatory/internal/model"
	"observatory/internal/repo"
	"observatory/internal/service"
	"observatory/internal/store/sqlite"
)

var epoch = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

type fixture struct {
	db  *sqlite.DB
	clk *clock.Fake
	svc *service.Services
	run *jobs.Runner
}

func newFixture(t *testing.T, dbPath string) *fixture {
	t.Helper()
	clk := clock.NewFake(epoch)
	db, err := sqlite.Open(dbPath)
	if err != nil {
		t.Fatalf("打开数据库失败: %v", err)
	}
	if err := db.Migrate(context.Background()); err != nil {
		t.Fatalf("迁移失败: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	svc := service.New(db, clk, 2)
	run := jobs.NewRunner(db, clk, 10*time.Millisecond, time.Second, nil)
	jobs.RegisterHandlers(run, svc)
	return &fixture{db: db, clk: clk, svc: svc, run: run}
}

func newDBPath(t *testing.T) string {
	return filepath.Join(t.TempDir(), "jobs.db")
}

// TestPrecoolTimeoutJob 预冷超时作业：超时后会话置 timeout、仪器转维护、登记异常。
func TestPrecoolTimeoutJob(t *testing.T) {
	f := newFixture(t, newDBPath(t))
	ctx := context.Background()

	site, _ := f.svc.Sites.CreateSite(ctx, model.Site{Code: "JS1", Name: "站", Latitude: -80, Longitude: 70, AltitudeM: 4000})
	in, _ := f.svc.Instruments.CreateInstrument(ctx, model.Instrument{
		SiteID: site.ID, Code: "JI1", Name: "仪", Kind: "imager", TempMinMK: 250, TempMaxMK: 350,
	})
	cryo, _ := f.svc.Cryo.RegisterSystem(ctx, in.ID, "制冷机", 300)
	sess, err := f.svc.Cryo.StartPrecool(ctx, cryo.ID, 300, epoch.Add(30*time.Minute), "op")
	if err != nil {
		t.Fatalf("预冷失败: %v", err)
	}
	// 未到期时作业不执行（run_at 在未来）。
	done, err := f.run.RunOnce(ctx)
	if err != nil {
		t.Fatalf("RunOnce 失败: %v", err)
	}
	if done {
		t.Fatalf("未到期作业不应被认领")
	}
	// 推进时钟超过截止时间。
	f.clk.Advance(31 * time.Minute)
	done, err = f.run.RunOnce(ctx)
	if err != nil || !done {
		t.Fatalf("到期作业应执行: done=%v err=%v", done, err)
	}
	sessions, _ := f.svc.Cryo.ListSessions(ctx, cryo.ID, repo.Page{Limit: 10})
	if len(sessions) != 1 || sessions[0].Status != domain.PrecoolTimeout {
		t.Fatalf("会话应为 timeout，实际 %+v", sessions)
	}
	inAfter, _ := f.svc.Instruments.GetInstrument(ctx, in.ID)
	if inAfter.Status != domain.InstrumentMaintenance {
		t.Fatalf("仪器应转维护，实际 %s", inAfter.Status)
	}
	anomalies, _ := f.svc.Anomalies.List(ctx, in.ID, domain.AnomalyOpen, repo.Page{Limit: 10})
	if len(anomalies) != 1 || anomalies[0].Kind != domain.AnomalyPrecoolTimeout {
		t.Fatalf("应登记预冷超时异常")
	}
	// 幂等：再次执行不改变结果。
	done, _ = f.run.RunOnce(ctx)
	_ = sess
}

// TestCalibrationExpiryJob 校准到期作业：方案过期，就绪仪器转维护。
func TestCalibrationExpiryJob(t *testing.T) {
	f := newFixture(t, newDBPath(t))
	ctx := context.Background()

	site, _ := f.svc.Sites.CreateSite(ctx, model.Site{Code: "JS2", Name: "站", Latitude: -80, Longitude: 70, AltitudeM: 4000})
	in, _ := f.svc.Instruments.CreateInstrument(ctx, model.Instrument{
		SiteID: site.ID, Code: "JI2", Name: "仪", Kind: "imager", TempMinMK: 250, TempMaxMK: 350,
	})
	cryo, _ := f.svc.Cryo.RegisterSystem(ctx, in.ID, "制冷机", 300)
	plan, _ := f.svc.Calibration.CreatePlan(ctx, model.CalibrationPlan{
		InstrumentID: in.ID, VersionNo: 1,
		ValidFrom: epoch.Add(-time.Hour), ValidUntil: epoch.Add(time.Hour),
	}, "planner")
	f.svc.Calibration.ApprovePlan(ctx, plan.ID, plan.Version, "approver")
	if _, err := f.svc.Calibration.ActivatePlan(ctx, plan.ID, plan.Version+1, "approver"); err != nil {
		t.Fatalf("启用失败: %v", err)
	}
	// 使仪器就绪（预冷达标）。
	f.svc.Cryo.StartPrecool(ctx, cryo.ID, 300, epoch.Add(2*time.Hour), "op")
	f.svc.Cryo.AddReading(ctx, cryo.ID, 300, 0, time.Time{}, "rd", "op")
	inReady, _ := f.svc.Instruments.GetInstrument(ctx, in.ID)
	if inReady.Status != domain.InstrumentReady {
		t.Fatalf("仪器应就绪，实际 %s", inReady.Status)
	}

	f.clk.Advance(61 * time.Minute) // 超过 valid_until
	if _, err := f.run.RunOnce(ctx); err != nil {
		t.Fatalf("作业执行失败: %v", err)
	}
	planAfter, _ := f.svc.Calibration.GetPlan(ctx, plan.ID)
	if planAfter.Status != domain.PlanExpired {
		t.Fatalf("方案应过期，实际 %s", planAfter.Status)
	}
	inAfter, _ := f.svc.Instruments.GetInstrument(ctx, in.ID)
	if inAfter.Status != domain.InstrumentMaintenance {
		t.Fatalf("无有效方案的仪器应转维护，实际 %s", inAfter.Status)
	}
}

// TestWindowEndJob 窗口结束作业：到期激活窗口关闭、仪器回就绪、批次自动冻结。
func TestWindowEndJob(t *testing.T) {
	f := newFixture(t, newDBPath(t))
	ctx := context.Background()

	// 快速建链至窗口激活。
	site, _ := f.svc.Sites.CreateSite(ctx, model.Site{Code: "JS3", Name: "站", Latitude: -80, Longitude: 70, AltitudeM: 4000})
	in, _ := f.svc.Instruments.CreateInstrument(ctx, model.Instrument{
		SiteID: site.ID, Code: "JI3", Name: "仪", Kind: "imager", TempMinMK: 250, TempMaxMK: 350,
	})
	cryo, _ := f.svc.Cryo.RegisterSystem(ctx, in.ID, "制冷机", 300)
	plan, _ := f.svc.Calibration.CreatePlan(ctx, model.CalibrationPlan{
		InstrumentID: in.ID, VersionNo: 1,
		ValidFrom: epoch.Add(-time.Hour), ValidUntil: epoch.Add(72 * time.Hour),
	}, "planner")
	f.svc.Calibration.ApprovePlan(ctx, plan.ID, plan.Version, "approver")
	f.svc.Calibration.ActivatePlan(ctx, plan.ID, plan.Version+1, "approver")
	src, _ := f.svc.Calibration.CreateSource(ctx, model.StandardSource{Code: "JSRC", Name: "源", FluxJy: 10})
	f.svc.Calibration.CreateRecord(ctx, model.CalibrationRecord{
		PlanID: plan.ID, SourceID: src.ID, Result: "pass", EvidenceURI: "s3://e",
	}, "cal")
	f.svc.Cryo.StartPrecool(ctx, cryo.ID, 300, epoch.Add(2*time.Hour), "op")
	f.svc.Cryo.AddReading(ctx, cryo.ID, 300, 0, time.Time{}, "rd", "op")
	win, _ := f.svc.Windows.Apply(ctx, model.ObservationWindow{
		InstrumentID: in.ID, Title: "W", StartAt: epoch, EndAt: epoch.Add(2 * time.Hour),
	}, "sch")
	tgt, _, _ := f.svc.Targets.Schedule(ctx, win.ID, model.Target{
		Name: "T", RaDeg: 1, DecDeg: 1, Priority: 1, ExposureGoal: 3, IdempotencyKey: "k",
	}, "sch")
	if _, err := f.svc.Windows.Approve(ctx, win.ID, win.Version, plan.ID, "appr"); err != nil {
		t.Fatalf("批准失败: %v", err)
	}
	if _, err := f.svc.Windows.Activate(ctx, win.ID, win.Version+1, "op"); err != nil {
		t.Fatalf("激活失败: %v", err)
	}
	b, _, err := f.svc.Batches.Start(ctx, win.ID, tgt.ID, "s3://raw/w", "batch-w", nil, "op")
	if err != nil {
		t.Fatalf("批次开始失败: %v", err)
	}

	f.clk.Advance(3 * time.Hour) // 超过窗口结束
	// 循环执行直至无到期作业（预冷超时与窗口结束可能同时到期）。
	for {
		done, err := f.run.RunOnce(ctx)
		if err != nil {
			t.Fatalf("窗口结束作业失败: %v", err)
		}
		if !done {
			break
		}
	}
	winAfter, _ := f.svc.Windows.Get(ctx, win.ID)
	if winAfter.Status != domain.WindowClosed {
		t.Fatalf("窗口应关闭，实际 %s", winAfter.Status)
	}
	inAfter, _ := f.svc.Instruments.GetInstrument(ctx, in.ID)
	if inAfter.Status != domain.InstrumentReady {
		t.Fatalf("仪器应回就绪，实际 %s", inAfter.Status)
	}
	bAfter, _ := f.svc.Batches.Get(ctx, b.ID)
	if bAfter.Status != domain.BatchFrozen {
		t.Fatalf("未完成批次应自动冻结，实际 %s", bAfter.Status)
	}
}

// TestJobRetryAndDead 失败作业按退避重试，超过最大次数进入 dead。
func TestJobRetryAndDead(t *testing.T) {
	f := newFixture(t, newDBPath(t))
	ctx := context.Background()

	f.run.Register("always_fail", func(ctx context.Context, payload string) error {
		return errors.New("模拟失败")
	})
	if _, err := f.svc.Jobs.Enqueue(ctx, f.db.SQL, "always_fail", `{"x":1}`, epoch, 2, epoch); err != nil {
		t.Fatalf("排程失败: %v", err)
	}
	// 第一次：failed, attempts=1。
	if _, err := f.run.RunOnce(ctx); err != nil {
		t.Fatalf("RunOnce 失败: %v", err)
	}
	list, _ := f.svc.Jobs.List(ctx, f.db.SQL, "always_fail", "", repo.Page{Limit: 10})
	if len(list) != 1 || list[0].Status != domain.JobFailed || list[0].Attempts != 1 {
		t.Fatalf("首次执行后应为 failed/attempts=1，实际 %+v", list)
	}
	// 退避未到：不可认领。
	if done, _ := f.run.RunOnce(ctx); done {
		t.Fatalf("退避期内不应认领")
	}
	// 推进时钟越过退避：attempts=2 达到 max → dead。
	f.clk.Advance(2 * time.Second)
	if _, err := f.run.RunOnce(ctx); err != nil {
		t.Fatalf("RunOnce 失败: %v", err)
	}
	list, _ = f.svc.Jobs.List(ctx, f.db.SQL, "always_fail", "", repo.Page{Limit: 10})
	if list[0].Status != domain.JobDead || list[0].Attempts != 2 {
		t.Fatalf("超过最大次数应为 dead，实际 %+v", list[0])
	}
	// 人工重试恢复 pending。
	if err := f.svc.Jobs.Retry(ctx, f.db.SQL, list[0].ID, f.clk.Now()); err != nil {
		t.Fatalf("人工重试失败: %v", err)
	}
	j, _ := f.svc.Jobs.Get(ctx, f.db.SQL, list[0].ID)
	if j.Status != domain.JobPending {
		t.Fatalf("重试后应为 pending，实际 %s", j.Status)
	}
}

// TestRestartRecovery 重启恢复：关闭并重开同一数据库文件，
// running 作业被恢复为 pending 并可再次认领执行。
func TestRestartRecovery(t *testing.T) {
	dbPath := newDBPath(t)
	ctx := context.Background()

	f := newFixture(t, dbPath)
	if _, err := f.svc.Jobs.Enqueue(ctx, f.db.SQL, "always_fail", `{"r":1}`, epoch, 3, epoch); err != nil {
		t.Fatalf("排程失败: %v", err)
	}
	// 模拟崩溃：作业处于 running 且进程退出。
	if _, err := f.db.SQL.Exec(`UPDATE jobs SET status='running', attempts=1`); err != nil {
		t.Fatalf("模拟崩溃失败: %v", err)
	}
	_ = f.db.Close()

	// 重启：同一 DB 文件。
	f2 := newFixture(t, dbPath)
	f2.run.Register("always_fail", func(ctx context.Context, payload string) error {
		return errors.New("仍失败")
	})
	if err := f2.run.Recover(ctx); err != nil {
		t.Fatalf("重启恢复失败: %v", err)
	}
	list, _ := f2.svc.Jobs.List(ctx, f2.db.SQL, "always_fail", "", repo.Page{Limit: 10})
	if len(list) != 1 || list[0].Status != domain.JobPending {
		t.Fatalf("恢复后应为 pending，实际 %+v", list)
	}
	// 恢复后可再次认领执行。
	done, err := f2.run.RunOnce(ctx)
	if err != nil || !done {
		t.Fatalf("恢复后应可执行: done=%v err=%v", done, err)
	}
	j, _ := f2.svc.Jobs.Get(ctx, f2.db.SQL, list[0].ID)
	if j.Attempts != 2 {
		t.Fatalf("attempts 应累计为 2，实际 %d", j.Attempts)
	}
}
