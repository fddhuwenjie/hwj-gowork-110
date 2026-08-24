package service_test

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestBug27(t *testing.T) {
	s := newStack(t)
	ctx := context.Background()
	st := seedToActiveWindow(t, s)
	b := runBatch(t, s, st, "review27")
	m, _ := s.svc.Metrics.Add(ctx, b.ID, 20, 2, 100, "analyst")
	if _, err := s.svc.Metrics.Seal(ctx, m.ID, "analyst"); err != nil {
		t.Fatalf("指标封存失败: %v", err)
	}
	a, _, err := s.svc.Archives.Request(ctx, b.ID, strings.Repeat("7", 64), 27, "review27-archive", "operator")
	if err != nil {
		t.Fatalf("归档失败: %v", err)
	}
	if _, err = s.svc.Archives.Verify(ctx, a.ID, "verifier"); err != nil {
		t.Fatalf("归档校验失败: %v", err)
	}
	p, err := s.svc.Releases.Submit(ctx, a.ID, "版本隔离成果", testEpoch.Add(48*time.Hour), "submitter")
	if err != nil {
		t.Fatalf("许可提交失败: %v", err)
	}
	if _, err = s.svc.Releases.Review(ctx, p.ID, p.Version-1, true, "recovery-reviewer-27"); err == nil {
		t.Fatal("陈旧版本的复核应返回冲突")
	}
}

func TestBug27Control(t *testing.T) {
	s := newStack(t)
	ctx := context.Background()
	st := seedToActiveWindow(t, s)
	b := runBatch(t, s, st, "review27-control")
	m, err := s.svc.Metrics.Add(ctx, b.ID, 20, 2, 100, "analyst")
	if err != nil { t.Fatalf("指标登记失败: %v", err) }
	if _, err = s.svc.Metrics.Seal(ctx, m.ID, "analyst"); err != nil { t.Fatalf("指标封存失败: %v", err) }
	a, _, err := s.svc.Archives.Request(ctx, b.ID, strings.Repeat("8", 64), 27, "review27-control-archive", "operator")
	if err != nil { t.Fatalf("归档失败: %v", err) }
	if _, err = s.svc.Archives.Verify(ctx, a.ID, "verifier"); err != nil { t.Fatalf("归档校验失败: %v", err) }
	p, err := s.svc.Releases.Submit(ctx, a.ID, "版本隔离成果控", testEpoch.Add(48*time.Hour), "submitter")
	if err != nil { t.Fatalf("许可提交失败: %v", err) }
	if _, err = s.svc.Releases.Review(ctx, p.ID, p.Version, true, "reviewer-27-control"); err != nil { t.Fatalf("当前版本复核应成功: %v", err) }
}
