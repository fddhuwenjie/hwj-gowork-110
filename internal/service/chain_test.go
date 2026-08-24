package service_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"observatory/internal/domain"
	"observatory/internal/repo"
)

// TestFullObservationChain 覆盖完整业务链：
// 建档→校准→预冷→窗口→排程→观测→质量→归档→发布，并校验不可变历史。
func TestFullObservationChain(t *testing.T) {
	s := newStack(t)
	ctx := context.Background()
	st := seedToActiveWindow(t, s)

	// 窗口冻结快照校验。
	win, err := s.svc.Windows.Get(ctx, st.windowID)
	if err != nil {
		t.Fatalf("读取窗口失败: %v", err)
	}
	if win.Status != domain.WindowActive {
		t.Fatalf("窗口应为 active，实际 %s", win.Status)
	}
	if !strings.Contains(win.FrozenSnapshot, `"M31"`) || !strings.Contains(win.FrozenSnapshot, `"version_no":1`) {
		t.Fatalf("冻结快照应包含目标与校准方案，实际 %s", win.FrozenSnapshot)
	}

	// 观测执行。
	b := runBatch(t, s, st, "batch-1")
	if b.Status != domain.BatchFrozen {
		t.Fatalf("批次应为 frozen，实际 %s", b.Status)
	}

	// 质量指标计算与封存（达标）。
	m, err := s.svc.Metrics.Add(ctx, b.ID, 20, 2.0, 100, "analyst")
	if err != nil {
		t.Fatalf("指标计算失败: %v", err)
	}
	if !m.Passed {
		t.Fatalf("指标应达标，score=%.2f", m.Score)
	}
	seal, err := s.svc.Metrics.Seal(ctx, m.ID, "analyst")
	if err != nil {
		t.Fatalf("指标封存失败: %v", err)
	}
	if seal.RetestBatch != nil || seal.Anomaly != nil {
		t.Fatalf("达标封存不应产生复测与异常")
	}

	// 归档请求与校验。
	arch, replay, err := s.svc.Archives.Request(ctx, b.ID,
		strings.Repeat("a", 64), 1024, "arch-1", "operator")
	if err != nil {
		t.Fatalf("归档请求失败: %v", err)
	}
	if replay {
		t.Fatalf("首次归档不应为重放")
	}
	if _, err := s.svc.Archives.Verify(ctx, arch.ID, "verifier"); err != nil {
		t.Fatalf("归档校验失败: %v", err)
	}

	// 发布许可：提交→复核（不同人）→校验发布事务。
	permit, err := s.svc.Releases.Submit(ctx, arch.ID, "M31 测光成果",
		testEpoch.Add(48*time.Hour), "submitter")
	if err != nil {
		t.Fatalf("许可提交失败: %v", err)
	}
	if _, err := s.svc.Releases.Review(ctx, permit.ID, permit.Version, true, "reviewer"); err != nil {
		t.Fatalf("许可复核失败: %v", err)
	}
	gotArch, gotPermit, err := s.svc.Archives.VerifyAndPublish(ctx, arch.ID, permit.ID, "publisher")
	if err != nil {
		t.Fatalf("校验发布事务失败: %v", err)
	}
	if gotArch.Status != domain.ArchivePublished || gotPermit.Status != domain.ReleasePublished {
		t.Fatalf("发布后状态错误: archive=%s permit=%s", gotArch.Status, gotPermit.Status)
	}

	// 批次应已进入 archived 终态。
	batch, err := s.svc.Batches.Get(ctx, b.ID)
	if err != nil {
		t.Fatalf("读取批次失败: %v", err)
	}
	if batch.Status != domain.BatchArchived {
		t.Fatalf("批次应为 archived，实际 %s", batch.Status)
	}

	// 不可变历史：仪器状态历史应包含完整转换链。
	history, err := s.svc.Instruments.ListHistory(ctx, st.instrumentID, page100())
	if err != nil {
		t.Fatalf("读取状态历史失败: %v", err)
	}
	var transitions []string
	for _, h := range history {
		transitions = append(transitions, h.FromStatus+"->"+h.ToStatus)
	}
	joined := strings.Join(transitions, ",")
	for _, want := range []string{"->registered", "registered->precooling", "precooling->ready", "ready->observing"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("状态历史缺少 %s，实际 %s", want, joined)
		}
	}
}

// TestLateMetricRejected 归档后迟到指标不得覆盖。
func TestLateMetricRejected(t *testing.T) {
	s := newStack(t)
	ctx := context.Background()
	st := seedToActiveWindow(t, s)
	b := runBatch(t, s, st, "batch-late")

	m, err := s.svc.Metrics.Add(ctx, b.ID, 20, 2.0, 100, "analyst")
	if err != nil {
		t.Fatalf("指标计算失败: %v", err)
	}
	if _, err := s.svc.Metrics.Seal(ctx, m.ID, "analyst"); err != nil {
		t.Fatalf("封存失败: %v", err)
	}
	arch, _, err := s.svc.Archives.Request(ctx, b.ID, strings.Repeat("b", 64), 10, "arch-late", "op")
	if err != nil {
		t.Fatalf("归档失败: %v", err)
	}
	if _, err := s.svc.Archives.Verify(ctx, arch.ID, "verifier"); err != nil {
		t.Fatalf("归档校验失败: %v", err)
	}
	permit, err := s.svc.Releases.Submit(ctx, arch.ID, "成果", testEpoch.Add(time.Hour), "alice")
	if err != nil {
		t.Fatalf("许可提交失败: %v", err)
	}
	if _, err := s.svc.Releases.Review(ctx, permit.ID, permit.Version, true, "bob"); err != nil {
		t.Fatalf("复核失败: %v", err)
	}
	if _, _, err := s.svc.Archives.VerifyAndPublish(ctx, arch.ID, permit.ID, "carol"); err != nil {
		t.Fatalf("发布失败: %v", err)
	}
	if _, err := s.svc.Metrics.Add(ctx, b.ID, 30, 1.0, 50, "late-analyst"); err == nil {
		t.Fatalf("已归档批次应拒绝迟到指标")
	}
}

// page100 返回 100 条分页参数。
func page100() repo.Page {
	return repo.Page{Limit: 100}
}
