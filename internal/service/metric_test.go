package service_test

import (
	"context"
	"testing"

	"observatory/internal/domain"
)

// TestSealFailIsolationAndRetest 指标封存不达标：批次隔离、异常登记、关联复测批次创建，
// 且原目标只允许创建关联复测。
func TestSealFailIsolationAndRetest(t *testing.T) {
	s := newStack(t)
	ctx := context.Background()
	st := seedToActiveWindow(t, s)
	b := runBatch(t, s, st, "batch-q")

	// 低 SNR → 不达标。
	m, err := s.svc.Metrics.Add(ctx, b.ID, 2, 5.0, 900, "analyst")
	if err != nil {
		t.Fatalf("指标登记失败: %v", err)
	}
	if m.Passed {
		t.Fatalf("指标应不达标")
	}
	seal, err := s.svc.Metrics.Seal(ctx, m.ID, "analyst")
	if err != nil {
		t.Fatalf("封存失败: %v", err)
	}
	if seal.Anomaly == nil || seal.RetestBatch == nil {
		t.Fatalf("不达标封存应产生异常与复测批次")
	}
	if seal.RetestBatch.RetestOfID == nil || *seal.RetestBatch.RetestOfID != b.ID {
		t.Fatalf("复测批次必须关联原批次")
	}

	batch, _ := s.svc.Batches.Get(ctx, b.ID)
	if batch.Status != domain.BatchIsolated {
		t.Fatalf("批次应保持隔离，实际 %s", batch.Status)
	}
	anomaly, _ := s.svc.Anomalies.List(ctx, st.instrumentID, domain.AnomalyRetestCreated, page100())
	if len(anomaly) == 0 {
		t.Fatalf("异常应转为 retest_created")
	}

	// 隔离批次禁止再登记指标。
	if _, err := s.svc.Metrics.Add(ctx, b.ID, 30, 1, 10, "analyst"); err == nil {
		t.Fatalf("隔离批次应拒绝新指标")
	}
	// 普通批次被拒绝：必须携带 retest_of_id。
	_, _, err = s.svc.Batches.Start(ctx, st.windowID, st.targetID, "s3://raw/n", "batch-normal", nil, "op")
	if err == nil {
		t.Fatalf("目标存在隔离批次时，普通批次应被拒绝")
	}
	// 注意：封存事务已自动创建复测批次，直接手工关联应因最新批次为 acquiring 复测而拒绝。
	retestID := seal.RetestBatch.ID
	_, _, err = s.svc.Batches.Start(ctx, st.windowID, st.targetID, "s3://raw/n2", "batch-normal-2", &retestID, "op")
	if err == nil {
		t.Fatalf("最新批次非隔离时按旧规则校验，复测进行中禁止重复批次")
	}

	// 复测批次执行并达标 → 原异常闭环为 resolved。
	s.clk.Advance(0) // 时钟不变
	if _, err := s.svc.Batches.Finish(ctx, retestID, 1, "op"); err != nil {
		t.Fatalf("复测批次冻结失败: %v", err)
	}
	m2, err := s.svc.Metrics.Add(ctx, retestID, 25, 1.5, 80, "analyst")
	if err != nil {
		t.Fatalf("复测指标登记失败: %v", err)
	}
	if _, err := s.svc.Metrics.Seal(ctx, m2.ID, "analyst"); err != nil {
		t.Fatalf("复测封存失败: %v", err)
	}
	resolved, _ := s.svc.Anomalies.List(ctx, st.instrumentID, domain.AnomalyResolved, page100())
	if len(resolved) == 0 {
		t.Fatalf("复测达标后原异常应转 resolved")
	}
}

// TestSealRequiresFrozenBatch 未冻结批次禁止封存指标。
func TestSealRequiresFrozenBatch(t *testing.T) {
	s := newStack(t)
	ctx := context.Background()
	st := seedToActiveWindow(t, s)

	b, _, err := s.svc.Batches.Start(ctx, st.windowID, st.targetID, "s3://raw/u", "batch-uf", nil, "op")
	if err != nil {
		t.Fatalf("批次开始失败: %v", err)
	}
	m, err := s.svc.Metrics.Add(ctx, b.ID, 20, 2, 100, "analyst")
	if err != nil {
		t.Fatalf("指标登记失败: %v", err)
	}
	if _, err := s.svc.Metrics.Seal(ctx, m.ID, "analyst"); err == nil {
		t.Fatalf("acquiring 批次应拒绝封存")
	}
}

// TestSealImmutable 封存后的指标不可重复封存。
func TestSealImmutable(t *testing.T) {
	s := newStack(t)
	ctx := context.Background()
	st := seedToActiveWindow(t, s)
	b := runBatch(t, s, st, "batch-im")

	m, _ := s.svc.Metrics.Add(ctx, b.ID, 20, 2, 100, "analyst")
	if _, err := s.svc.Metrics.Seal(ctx, m.ID, "analyst"); err != nil {
		t.Fatalf("首次封存失败: %v", err)
	}
	if _, err := s.svc.Metrics.Seal(ctx, m.ID, "analyst"); err == nil {
		t.Fatalf("重复封存应被拒绝")
	}
}
