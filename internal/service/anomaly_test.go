package service_test

import (
	"context"
	"errors"
	"testing"

	"observatory/internal/apperr"
	"observatory/internal/domain"
	"observatory/internal/model"
)

// TestAnomalyResolveIdempotentRetry 异常处置已 resolved 后，客户端因网络超时
// 重放同一 Resolve 请求应幂等成功返回当前记录，而非 invalid_transition。
func TestAnomalyResolveIdempotentRetry(t *testing.T) {
	s := newStack(t)
	ctx := context.Background()
	st := seedToActiveWindow(t, s)
	b := runBatch(t, s, st, "batch-anom-idem")

	// 封存不达标 → 异常 retest_created（不完成复测，避免自动转 resolved）。
	m, err := s.svc.Metrics.Add(ctx, b.ID, 2, 5.0, 900, "analyst")
	if err != nil {
		t.Fatalf("指标登记失败: %v", err)
	}
	seal, err := s.svc.Metrics.Seal(ctx, m.ID, "analyst")
	if err != nil {
		t.Fatalf("封存失败: %v", err)
	}
	if seal.Anomaly == nil {
		t.Fatalf("不达标封存应产生异常")
	}
	anomalyID := seal.Anomaly.ID

	// 客户端首次 Resolve：retest_created → resolved。
	first, err := s.svc.Anomalies.Resolve(ctx, anomalyID, "op")
	if err != nil {
		t.Fatalf("首次 Resolve 应成功: %v", err)
	}
	if first.Status != domain.AnomalyResolved {
		t.Fatalf("应处于 resolved，实际 %s", first.Status)
	}
	if first.ResolvedBy != "op" {
		t.Fatalf("ResolvedBy 应为 op，实际 %q", first.ResolvedBy)
	}

	// 客户端网络超时重放同一 Resolve：必须幂等返回 resolved 记录，不覆盖 ResolvedBy。
	second, err := s.svc.Anomalies.Resolve(ctx, anomalyID, "op")
	if err != nil {
		t.Fatalf("重放 Resolve 应幂等成功，实际 %v", err)
	}
	if second.ID != first.ID || second.Status != domain.AnomalyResolved {
		t.Fatalf("重放应返回同一 resolved 异常，实际 id=%d status=%s", second.ID, second.Status)
	}
	if second.ResolvedBy != "op" {
		t.Fatalf("ResolvedBy 应保留为 op，实际 %q", second.ResolvedBy)
	}
}

// TestAnomalyCloseIdempotentRetry 已 closed 异常重放 Close 幂等成功。
func TestAnomalyCloseIdempotentRetry(t *testing.T) {
	s := newStack(t)
	ctx := context.Background()

	site, _ := s.svc.Sites.CreateSite(ctx, newSite("S-anom-c"))
	in, _ := s.svc.Instruments.CreateInstrument(ctx, newInstrument(site.ID, "I-anom-c"))

	// 无需复测的异常：open → closed。
	a, err := s.svc.Anomalies.CreateManual(ctx, nil, in.ID, domain.AnomalyManual, "手工登记", "op")
	if err != nil {
		t.Fatalf("登记异常失败: %v", err)
	}
	first, err := s.svc.Anomalies.Close(ctx, a.ID, "op")
	if err != nil {
		t.Fatalf("首次 Close 失败: %v", err)
	}
	if first.Status != domain.AnomalyClosed {
		t.Fatalf("应处于 closed，实际 %s", first.Status)
	}
	second, err := s.svc.Anomalies.Close(ctx, a.ID, "op")
	if err != nil {
		t.Fatalf("重放 Close 应幂等成功: %v", err)
	}
	if second.ID != first.ID || second.Status != domain.AnomalyClosed {
		t.Fatalf("重放应返回同一 closed 异常")
	}
}

// TestAnomalyIllegalSkipTransitionRejected 越级转换（open → resolved 跳过
// retest_created）仍被拒绝，幂等路径不放宽跨状态校验。
func TestAnomalyIllegalSkipTransitionRejected(t *testing.T) {
	s := newStack(t)
	ctx := context.Background()

	site, _ := s.svc.Sites.CreateSite(ctx, newSite("S-anom-skip"))
	in, _ := s.svc.Instruments.CreateInstrument(ctx, newInstrument(site.ID, "I-anom-skip"))

	a, err := s.svc.Anomalies.CreateManual(ctx, nil, in.ID, domain.AnomalyManual, "越级测试", "op")
	if err != nil {
		t.Fatalf("登记异常失败: %v", err)
	}
	if _, err := s.svc.Anomalies.Resolve(ctx, a.ID, "op"); err == nil {
		t.Fatalf("open → resolved 越级转换应被拒绝")
	} else {
		var ae *apperr.Error
		if !errors.As(err, &ae) || ae.Code != apperr.CodeInvalidTransition {
			t.Fatalf("应返回 invalid_transition，实际 %v", err)
		}
	}
}

func newSite(code string) (site model.Site) {
	return model.Site{Code: code, Name: code, Latitude: -80, Longitude: 70, AltitudeM: 4000}
}

func newInstrument(siteID int64, code string) model.Instrument {
	return model.Instrument{
		SiteID: siteID, Code: code, Name: code, Kind: "imager", TempMinMK: 250, TempMaxMK: 350,
	}
}
