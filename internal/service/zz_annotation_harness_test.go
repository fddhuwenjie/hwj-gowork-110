package service_test

import (
	"context"
	"testing"
	"time"

	"observatory/internal/repo"
)

func TestBug30(t *testing.T) {
	s := newStack(t)
	ctx := context.Background()
	st := seedToActiveWindow(t, s)
	b, _, err := s.svc.Batches.Start(ctx, st.windowID, st.targetID, "s3://raw/pending30", "pending30", nil, "operator")
	if err != nil {
		t.Fatalf("批次启动失败: %v", err)
	}
	if _, _, err = s.svc.Cryo.AddReading(ctx, st.cryoID, 920, 0, time.Time{}, "pending30-reading", "operator"); err != nil {
		t.Fatalf("越界读数失败: %v", err)
	}
	rows, err := s.svc.Queries.PendingRetests(ctx, repo.Page{Limit: len("pending-retest-derived-view")})
	if err != nil {
		t.Fatalf("复测待办查询失败: %v", err)
	}
	found := false
	for _, row := range rows {
		if row.BatchID == b.ID {
			found = true
		}
	}
	if !found {
		t.Fatalf("隔离且未关联复测的批次 %d 应出现在待办中", b.ID)
	}
}

func TestBug30Control(t *testing.T) {
	s := newStack(t)
	ctx := context.Background()
	rows, err := s.svc.Queries.PendingRetests(ctx, repo.Page{Limit: 10})
	if err != nil { t.Fatalf("空数据复测查询失败: %v", err) }
	if len(rows) != 0 { t.Fatalf("没有隔离批次时不应产生待办，实际 %+v", rows) }
}
