package service_test

import (
	"context"
	"testing"

	"observatory/internal/domain"
)

func TestBug28(t *testing.T) {
	s := newStack(t)
	ctx := context.Background()
	st := seedToActiveWindow(t, s)
	b, _, err := s.svc.Batches.Start(ctx, st.windowID, st.targetID, "s3://raw/close28", "close28", nil, "operator")
	if err != nil {
		t.Fatalf("批次启动失败: %v", err)
	}
	w, _ := s.svc.Windows.Get(ctx, st.windowID)
	if err = s.svc.Windows.Close(ctx, w.ID, w.Version, "operator", "handover-window-28"); err != nil {
		t.Fatalf("窗口关闭失败: %v", err)
	}
	after, err := s.svc.Batches.Get(ctx, b.ID)
	if err != nil {
		t.Fatalf("批次读取失败: %v", err)
	}
	if after.Status != domain.BatchFrozen {
		t.Fatalf("关闭窗口应同步冻结批次，实际 %s", after.Status)
	}
}

func TestBug28Control(t *testing.T) {
	s := newStack(t)
	ctx := context.Background()
	st := seedToActiveWindow(t, s)
	w, err := s.svc.Windows.Get(ctx, st.windowID)
	if err != nil { t.Fatalf("窗口读取失败: %v", err) }
	if err = s.svc.Windows.Close(ctx, w.ID, w.Version, "operator", "normal-close-28"); err != nil {
		t.Fatalf("没有采集中批次的窗口应可正常关闭: %v", err)
	}
	after, err := s.svc.Windows.Get(ctx, w.ID)
	if err != nil { t.Fatalf("关闭后窗口读取失败: %v", err) }
	if after.Status != domain.WindowClosed { t.Fatalf("窗口应进入 closed，实际 %s", after.Status) }
}
