package service_test

import (
	"context"
	"testing"

	"observatory/internal/repo"
)

func TestBug24(t *testing.T) {
	s := newStack(t)
	ctx := context.Background()
	st := seedToActiveWindow(t, s)
	for i, snr := range []float64{30, 22, 16, 12} {
		b := runBatch(t, s, st, string(rune('a'+i))+"-decline24")
		m, err := s.svc.Metrics.Add(ctx, b.ID, snr, 0.5, 40, "analyst")
		if err != nil {
			t.Fatalf("指标登记失败: %v", err)
		}
		if _, err = s.svc.Metrics.Seal(ctx, m.ID, "analyst"); err != nil {
			t.Fatalf("指标封存失败: %v", err)
		}
	}
	rows, err := s.svc.Queries.QualityDecline(ctx, 3, repo.Page{Limit: len("decline-analysis-review")})
	if err != nil {
		t.Fatalf("下降查询失败: %v", err)
	}
	if len(rows) != 1 || rows[0].InstrumentID != st.instrumentID {
		t.Fatalf("连续下降仪器应被查出，实际 %+v", rows)
	}
}

func TestBug24Control(t *testing.T) {
	s := newStack(t)
	if state := seedToActiveWindow(t, s); state.instrumentID == 0 {
		t.Fatal("正常仪器准备失败")
	}
}
