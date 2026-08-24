package domain

import (
	"testing"
	"time"
)

// TestTempInRange 温度区间边界。
func TestTempInRange(t *testing.T) {
	cases := []struct {
		temp, min, max float64
		want           bool
	}{
		{300, 250, 350, true},
		{250, 250, 350, true}, // 下限边界合法
		{350, 250, 350, true}, // 上限边界合法
		{249.9, 250, 350, false},
		{350.1, 250, 350, false},
	}
	for _, c := range cases {
		if got := TempInRange(c.temp, c.min, c.max); got != c.want {
			t.Errorf("TempInRange(%v,%v,%v)=%v，期望 %v", c.temp, c.min, c.max, got, c.want)
		}
	}
}

// TestValidateTempRange 建档区间校验。
func TestValidateTempRange(t *testing.T) {
	if err := ValidateTempRange(250, 350); err != nil {
		t.Errorf("合法区间被拒绝: %v", err)
	}
	if err := ValidateTempRange(350, 250); err == nil {
		t.Errorf("下限≥上限应被拒绝")
	}
	if err := ValidateTempRange(-1, 250); err == nil {
		t.Errorf("负温度应被拒绝")
	}
}

// TestQualityScore 质量评分与通过阈值。
func TestQualityScore(t *testing.T) {
	high := ComputeScore(30, 0.5, 10)
	if high < 90 {
		t.Errorf("高质量评分应接近满分，实际 %.2f", high)
	}
	if !Pass(30, 0.5, 10, high) {
		t.Errorf("高质量指标应通过")
	}
	low := ComputeScore(1, 5, 2000)
	if Pass(1, 5, 2000, low) {
		t.Errorf("低质量指标不应通过")
	}
	if Pass(30, 0.5, 10, 50) {
		t.Errorf("综合评分低于 60 不应通过")
	}
}

// TestConsecutiveDecline 连续下降判定。
func TestConsecutiveDecline(t *testing.T) {
	if !ConsecutiveDecline([]float64{90, 80, 70, 60}, 3) {
		t.Errorf("末尾 4 点严格递减应命中 min=3")
	}
	if ConsecutiveDecline([]float64{90, 80, 85, 60}, 3) {
		t.Errorf("中间回升应打断连续下降")
	}
	if !ConsecutiveDecline([]float64{50, 90, 80, 70}, 2) {
		t.Errorf("末尾 3 点递减应命中 min=2")
	}
	if ConsecutiveDecline([]float64{70, 70, 70}, 1) {
		t.Errorf("持平不构成严格下降")
	}
}

// TestExposureSeq 曝光序号连续性。
func TestExposureSeq(t *testing.T) {
	if err := EnsureExposureSeq(0, 1); err != nil {
		t.Errorf("首个序号 1 应合法: %v", err)
	}
	if err := EnsureExposureSeq(2, 3); err != nil {
		t.Errorf("序号 3 应合法: %v", err)
	}
	if err := EnsureExposureSeq(2, 2); err == nil {
		t.Errorf("重复序号应被拒绝")
	}
	if err := EnsureExposureSeq(2, 5); err == nil {
		t.Errorf("跳号应被拒绝")
	}
	// 长序列：序号超过 40 时也必须仅递增一位，不得跳号。
	if got := NextExposureSeq(40); got != 41 {
		t.Errorf("NextExposureSeq(40)=%d，期望 41", got)
	}
	if got := NextExposureSeq(100); got != 101 {
		t.Errorf("NextExposureSeq(100)=%d，期望 101", got)
	}
	if err := EnsureExposureSeq(40, 41); err != nil {
		t.Errorf("长序列序号 41 应合法: %v", err)
	}
	if err := EnsureExposureSeq(40, 42); err == nil {
		t.Errorf("长序列跳号（40→42）应被拒绝")
	}
}

// TestTransitions 状态机。
func TestTransitions(t *testing.T) {
	if !CanTransition(EntityInstrument, InstrumentRegistered, InstrumentPrecooling) {
		t.Errorf("registered→precooling 应合法")
	}
	if CanTransition(EntityInstrument, InstrumentObserving, InstrumentMaintenance) {
		t.Errorf("observing→maintenance 应非法")
	}
	if !CanTransition(EntityBatch, BatchFrozen, BatchArchived) {
		t.Errorf("frozen→archived 应合法")
	}
	if CanTransition(EntityBatch, BatchIsolated, BatchFrozen) {
		t.Errorf("isolated→frozen 应非法（隔离为终态）")
	}
	if err := MustTransition(EntityWindow, WindowApplied, WindowActive); err == nil {
		t.Errorf("applied→active 应返回 invalid_transition")
	}
}

// TestCalibrationCoverage 校准覆盖。
func TestCalibrationCoverage(t *testing.T) {
	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	until := from.Add(72 * time.Hour)
	if err := EnsureBatchCoverage(from, until, from.Add(time.Hour), from.Add(2*time.Hour)); err != nil {
		t.Errorf("覆盖区间应通过: %v", err)
	}
	if err := EnsureBatchCoverage(from, until, from.Add(-time.Hour), from.Add(time.Hour)); err == nil {
		t.Errorf("批次开始早于有效期应失败")
	}
	if err := EnsureBatchCoverage(from, until, from.Add(time.Hour), until.Add(time.Second)); err == nil {
		t.Errorf("批次结束晚于有效期应失败")
	}
}

// TestReviewerSeparation 双人复核。
func TestReviewerSeparation(t *testing.T) {
	if err := EnsureDifferentReviewer("alice", "bob"); err != nil {
		t.Errorf("不同人复核应通过: %v", err)
	}
	if err := EnsureDifferentReviewer("alice", "alice"); err == nil {
		t.Errorf("相同人复核应被拒绝")
	}
	if err := EnsureDifferentReviewer("alice", ""); err == nil {
		t.Errorf("空复核人应被拒绝")
	}
}
