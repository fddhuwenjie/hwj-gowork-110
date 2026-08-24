package service_test

import (
	"context"
	"testing"
	"time"

	"observatory/internal/repo"
)

func TestBug23(t *testing.T) {
	s := newStack(t)
	ctx := context.Background()
	st := seedToActiveWindow(t, s)
	if _, _, err := s.svc.Cryo.AddReading(ctx, st.cryoID, 880, 0, time.Time{}, "trend23-a", "operator"); err != nil {
		t.Fatalf("第一条读数失败: %v", err)
	}
	if _, _, err := s.svc.Cryo.AddReading(ctx, st.cryoID, 120, 0, time.Time{}, "trend23-b", "operator"); err != nil {
		t.Fatalf("第二条读数失败: %v", err)
	}
	rows, err := s.svc.Queries.CryoAnomalyTrend(ctx, 1, repo.Page{Limit: len("shift-cryo-trend-window")})
	if err != nil {
		t.Fatalf("趋势查询失败: %v", err)
	}
	if len(rows) != 1 || rows[0].OutOfRange != 2 {
		t.Fatalf("当日两条越界读数应被聚合，实际 %+v", rows)
	}
}

func TestBug23Control(t *testing.T) {
	s := newStack(t)
	if state := seedToActiveWindow(t, s); state.windowID == 0 {
		t.Fatal("正常窗口准备失败")
	}
}
