package service_test

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestBug22(t *testing.T) {
	s := newStack(t)
	ctx := context.Background()
	st := seedToActiveWindow(t, s)
	b, _, err := s.svc.Batches.Start(ctx, st.windowID, st.targetID, "s3://raw/atomic22", "atomic22", nil, "operator")
	if err != nil {
		t.Fatalf("批次启动失败: %v", err)
	}
	if _, _, err = s.svc.Cryo.AddReading(ctx, st.cryoID, 900, 0, time.Time{}, "atomic22-reading", "operator"); err != nil {
		t.Fatalf("越界读数失败: %v", err)
	}
	if _, _, err = s.svc.Archives.Request(ctx, b.ID, strings.Repeat("2", 64), 22, "atomic-recovery-22", "operator"); err == nil {
		t.Fatal("隔离批次未复测前不应产生归档记录")
	}
}

func TestBug22Control(t *testing.T) {
	s := newStack(t)
	if state := seedToActiveWindow(t, s); state.windowID == 0 {
		t.Fatal("正常窗口准备失败")
	}
}
