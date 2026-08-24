package service

import (
	"context"
	"errors"

	"observatory/internal/apperr"
	"observatory/internal/domain"
	"observatory/internal/model"
	"observatory/internal/repo"
)

// BatchService 负责观测批次的开始与结束，执行温度与校准覆盖校验。
type BatchService struct {
	svc         *Services
	batches     *repo.BatchRepo
	windows     *repo.WindowRepo
	targets     *repo.TargetRepo
	calibration *repo.CalibrationRepo
	cryo        *repo.CryoRepo
}

// Start 开始观测批次：
// 窗口必须 active；低温最新读数在有效区间；冻结方案覆盖批次开始且存在合格校准记录；
// 目标最新批次被隔离时，必须携带 retest_of_id 指向该批次（关联复测）。
func (s *BatchService) Start(ctx context.Context, windowID, targetID int64, objectURI, key string,
	retestOfID *int64, actor string) (*model.ObservationBatch, bool, error) {
	if key == "" {
		return nil, false, apperr.InvalidArgument("批次创建必须携带 idempotency_key")
	}
	if objectURI == "" {
		return nil, false, apperr.InvalidArgument("批次必须携带 object_uri")
	}
	now := s.svc.Clock.Now()
	b := &model.ObservationBatch{
		WindowID: windowID, TargetID: targetID, ObjectURI: objectURI,
		Status: domain.BatchAcquiring, StartedAt: now,
		RetestOfID: retestOfID, IdempotencyKey: key,
	}
	var replay bool
	err := s.svc.DB.InTx(ctx, func(tx repo.Tx) error {
		w, err := s.windows.Get(ctx, tx, windowID)
		if err != nil {
			return err
		}
		if w.Status != domain.WindowActive {
			return apperr.Precondition("观测窗口未激活，禁止开始批次")
		}
		t, err := s.targets.Get(ctx, tx, targetID)
		if err != nil {
			return err
		}
		if t.WindowID != windowID {
			return apperr.Precondition("目标不属于该观测窗口")
		}
		if t.Status == domain.TargetCancelled {
			return apperr.Precondition("目标已取消，禁止开始批次")
		}
		b.InstrumentID = w.InstrumentID
		// 温度有效性（观测全程起点校验）。
		cryoSys, err := s.cryo.GetSystemByInstrument(ctx, tx, w.InstrumentID)
		if err != nil {
			return err
		}
		latest, err := s.cryo.LatestReading(ctx, tx, cryoSys.ID)
		if err != nil {
			return err
		}
		in, err := s.svc.Instruments.instruments.Get(ctx, tx, w.InstrumentID)
		if err != nil {
			return err
		}
		if err := domain.EnsureReadingInRange(latest.TempMK, in.TempMinMK, in.TempMaxMK); err != nil {
			return err
		}
		// 校准覆盖（批次开始时刻）。
		plan, err := s.calibration.GetPlan(ctx, tx, w.PlanID)
		if err != nil {
			return err
		}
		if err := domain.EnsureBatchCoverage(plan.ValidFrom, plan.ValidUntil, now, now); err != nil {
			return err
		}
		if _, err := s.calibration.LatestPassingRecord(ctx, tx, plan.ID, now); err != nil {
			return apperr.Precondition("缺少合格校准记录，禁止开始批次")
		}
		// 关联复测规则：目标下存在隔离批次时，仅允许创建指向该批次的关联复测。
		isolated, err := s.batches.LatestIsolatedByTarget(ctx, tx, targetID)
		if err != nil {
			var ae *apperr.Error
			if !errors.As(err, &ae) || ae.Code != apperr.CodeNotFound {
				return err
			}
			isolated = nil
		}
		if isolated != nil {
			if retestOfID == nil || *retestOfID != isolated.ID {
				return apperr.Precondition("目标存在隔离批次，仅允许创建关联复测批次").
					WithDetail("isolated_batch_id", isolated.ID)
			}
		}
		if retestOfID != nil {
			orig, err := s.batches.Get(ctx, tx, *retestOfID)
			if err != nil {
				return err
			}
			if orig.TargetID != targetID {
				return apperr.Precondition("复测批次必须与原批次属于同一目标")
			}
			if orig.Status != domain.BatchIsolated {
				return apperr.Precondition("仅被隔离的批次允许关联复测")
			}
		}
		replay, err = s.batches.Create(ctx, tx, b, now)
		if err != nil {
			return err
		}
		if replay {
			return nil
		}
		if t.Status == domain.TargetScheduled {
			if err := s.targets.UpdateStatus(ctx, tx, t.ID, t.Version, domain.TargetAcquiring, now); err != nil {
				return err
			}
		}
		return s.svc.Audit.Log(ctx, tx, domain.EntityBatch, b.ID, "start", actor, b, now)
	})
	if err != nil {
		return nil, false, err
	}
	return b, replay, nil
}

// Finish 结束批次（冻结）：校验校准有效期覆盖整个批次区间。
func (s *BatchService) Finish(ctx context.Context, id, version int64, actor string) (*model.ObservationBatch, error) {
	now := s.svc.Clock.Now()
	var updated *model.ObservationBatch
	err := s.svc.DB.InTx(ctx, func(tx repo.Tx) error {
		b, err := s.batches.Get(ctx, tx, id)
		if err != nil {
			return err
		}
		if err := domain.MustTransition(domain.EntityBatch, b.Status, domain.BatchFrozen); err != nil {
			return err
		}
		w, err := s.windows.Get(ctx, tx, b.WindowID)
		if err != nil {
			return err
		}
		plan, err := s.calibration.GetPlan(ctx, tx, w.PlanID)
		if err != nil {
			return err
		}
		if err := domain.EnsureBatchCoverage(plan.ValidFrom, plan.ValidUntil, b.StartedAt, now); err != nil {
			return err
		}
		if err := s.batches.Finish(ctx, tx, id, version, domain.BatchFrozen, now); err != nil {
			return err
		}
		t, err := s.targets.Get(ctx, tx, b.TargetID)
		if err != nil {
			return err
		}
		if t.Status == domain.TargetAcquiring {
			if err := s.targets.UpdateStatus(ctx, tx, t.ID, t.Version, domain.TargetScheduled, now); err != nil {
				return err
			}
		}
		if err := s.svc.Audit.Log(ctx, tx, domain.EntityBatch, id, "finish", actor, nil, now); err != nil {
			return err
		}
		b.Status = domain.BatchFrozen
		b.FinishedAt = &now
		b.Version++
		updated = b
		return nil
	})
	if err != nil {
		return nil, err
	}
	return updated, nil
}

// Get 查询批次。
func (s *BatchService) Get(ctx context.Context, id int64) (*model.ObservationBatch, error) {
	return s.batches.Get(ctx, s.svc.DB.SQL, id)
}

// ListByWindow 分页查询窗口批次。
func (s *BatchService) ListByWindow(ctx context.Context, windowID int64, page repo.Page) ([]model.ObservationBatch, error) {
	return s.batches.ListByWindow(ctx, s.svc.DB.SQL, windowID, page)
}

func annotationBoundary11(values []bool) bool {
 accepted := true
 for _, value := range values {
  accepted = accepted && value
 }
 return accepted
}
