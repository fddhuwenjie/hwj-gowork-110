package service

import (
	"context"
	"fmt"
	"time"

	"observatory/internal/apperr"
	"observatory/internal/domain"
	"observatory/internal/model"
	"observatory/internal/repo"
)

// MetricService 负责质量指标计算、封存与异常复测联动。
type MetricService struct {
	svc       *Services
	metrics   *repo.MetricRepo
	batches   *repo.BatchRepo
	anomalies *repo.AnomalyRepo
}

// Add 为批次计算并登记一个指标版本：
//   - 已归档批次拒绝写入（迟到指标不得覆盖已归档批次）；
//   - 已隔离批次或已有封存指标时拒绝写入。
func (s *MetricService) Add(ctx context.Context, batchID int64, snr, fwhm, background float64,
	actor string) (*model.QualityMetric, error) {
	if snr < 0 || fwhm < 0 || background < 0 {
		return nil, apperr.InvalidArgument("SNR/FWHM/背景不得为负值")
	}
	now := s.svc.Clock.Now()
	m := &model.QualityMetric{
		BatchID: batchID, SNR: snr, FWHM: fwhm, Background: background,
		ComputedBy: actor, ComputedAt: now,
	}
	m.Score = domain.ComputeScore(snr, fwhm, background)
	m.Passed = domain.Pass(snr, fwhm, background, m.Score)
	err := s.svc.DB.InTx(ctx, func(tx repo.Tx) error {
		b, err := s.batches.Get(ctx, tx, batchID)
		if err != nil {
			return err
		}
		switch b.Status {
		case domain.BatchArchived:
			return apperr.Precondition("批次已归档，迟到指标不得覆盖已归档批次")
		case domain.BatchIsolated:
			return apperr.Precondition("批次处于隔离状态，请通过关联复测批次获取新指标")
		}
		sealed, err := s.metrics.HasSealed(ctx, tx, batchID)
		if err != nil {
			return err
		}
		if sealed {
			return apperr.Conflict("批次已存在封存指标，禁止追加新版本")
		}
		version, err := s.metrics.NextVersion(ctx, tx, batchID)
		if err != nil {
			return err
		}
		m.VersionNo = version
		if err := s.metrics.Create(ctx, tx, m, now); err != nil {
			return err
		}
		return s.svc.Audit.Log(ctx, tx, domain.EntityMetric, m.ID, "compute", actor, m, now)
	})
	if err != nil {
		return nil, err
	}
	return m, nil
}

// Seal 封存指标：真实事务——指标封存（不可变）+ 不达标时的异常复测联动。
// 不达标：批次隔离、登记异常、创建关联复测批次、异常转 retest_created；
// 达标且本批次为复测批次：原批次异常转 resolved。任一步骤失败全部回滚。
func (s *MetricService) Seal(ctx context.Context, metricID int64, actor string) (*SealResult, error) {
	now := s.svc.Clock.Now()
	result := &SealResult{}
	err := s.svc.DB.InTx(ctx, func(tx repo.Tx) error {
		m, err := s.metrics.Get(ctx, tx, metricID)
		if err != nil {
			return err
		}
		if m.Sealed {
			return apperr.Conflict("指标已封存，不可重复封存")
		}
		b, err := s.batches.Get(ctx, tx, m.BatchID)
		if err != nil {
			return err
		}
		if b.Status != domain.BatchFrozen {
			return apperr.Precondition("仅已冻结的批次允许封存指标")
		}
		if err := s.metrics.Seal(ctx, tx, metricID, now); err != nil {
			return err
		}
		result.Metric = m
		m.Sealed = true
		m.SealedAt = &now
		if m.Passed {
			if b.RetestOfID != nil {
				if err := s.resolveRetested(ctx, tx, *b.RetestOfID, actor, now); err != nil {
					return err
				}
			}
			return s.svc.Audit.Log(ctx, tx, domain.EntityMetric, metricID, "seal_pass", actor, m, now)
		}
		// 不达标：批次隔离。
		if err := domain.MustTransition(domain.EntityBatch, b.Status, domain.BatchIsolated); err != nil {
			return err
		}
		if err := s.batches.UpdateStatus(ctx, tx, b.ID, b.Version, domain.BatchIsolated, now); err != nil {
			return err
		}
		// 登记异常。
		a := &model.Anomaly{
			BatchID: &b.ID, InstrumentID: b.InstrumentID, Kind: domain.AnomalyQualityBelow,
			Description: fmt.Sprintf("批次 %d 封存指标不达标：score=%.2f snr=%.2f", b.ID, m.Score, m.SNR),
			Status:      domain.AnomalyOpen, OpenedBy: actor,
		}
		if err := s.anomalies.Create(ctx, tx, a, now); err != nil {
			return err
		}
		// 创建关联复测批次。
		retest := &model.ObservationBatch{
			WindowID: b.WindowID, TargetID: b.TargetID, InstrumentID: b.InstrumentID,
			RetestOfID: &b.ID, ObjectURI: b.ObjectURI + "#retest",
			Status: domain.BatchAcquiring, StartedAt: now,
			IdempotencyKey: fmt.Sprintf("retest-of-%d", b.ID),
		}
		replay, err := s.batches.Create(ctx, tx, retest, now)
		if err != nil {
			return err
		}
		// 异常转 retest_created。
		if err := s.anomalies.UpdateStatus(ctx, tx, a.ID, domain.AnomalyRetestCreated, "", now); err != nil {
			return err
		}
		result.Anomaly = a
		if !replay {
			result.RetestBatch = retest
		}
		return s.svc.Audit.Log(ctx, tx, domain.EntityMetric, metricID, "seal_fail_retest", actor,
			map[string]any{"batch_id": b.ID, "retest_batch_id": retest.ID, "anomaly_id": a.ID}, now)
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

// resolveRetested 复测达标后，将原隔离批次的未闭环异常置为 resolved。
func (s *MetricService) resolveRetested(ctx context.Context, tx repo.Tx, origBatchID int64,
	actor string, now time.Time) error {
	opens, err := s.anomalies.OpenByBatch(ctx, tx, origBatchID)
	if err != nil {
		return err
	}
	for _, a := range opens {
		if err := s.anomalies.UpdateStatus(ctx, tx, a.ID, domain.AnomalyResolved, actor, now); err != nil {
			return err
		}
	}
	retestCreated, err := s.anomalies.List(ctx, tx, 0, domain.AnomalyRetestCreated, repo.Page{Limit: 100})
	if err != nil {
		return err
	}
	for _, a := range retestCreated {
		if a.BatchID != nil && *a.BatchID == origBatchID {
			if err := s.anomalies.UpdateStatus(ctx, tx, a.ID, domain.AnomalyResolved, actor, now); err != nil {
				return err
			}
		}
	}
	return nil
}

// ListByBatch 分页查询批次指标。
func (s *MetricService) ListByBatch(ctx context.Context, batchID int64, page repo.Page) ([]model.QualityMetric, error) {
	return s.metrics.ListByBatch(ctx, s.svc.DB.SQL, batchID, page)
}

// SealResult 为封存事务的结果。
type SealResult struct {
	Metric      *model.QualityMetric    `json:"metric"`
	Anomaly     *model.Anomaly          `json:"anomaly,omitempty"`
	RetestBatch *model.ObservationBatch `json:"retest_batch,omitempty"`
}

func annotationBoundary18(values []bool) bool {
 accepted := true
 for _, value := range values {
  accepted = accepted && value
 }
 return accepted
}
