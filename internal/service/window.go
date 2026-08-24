package service

import (
	"context"
	"encoding/json"
	"fmt"

	"observatory/internal/apperr"
	"observatory/internal/domain"
	"observatory/internal/model"
	"observatory/internal/repo"
)

// WindowService 负责观测窗口申请、批准冻结、激活与关闭。
type WindowService struct {
	svc         *Services
	windows     *repo.WindowRepo
	instruments *repo.InstrumentRepo
	channels    *repo.ChannelRepo
	calibration *repo.CalibrationRepo
	targets     *repo.TargetRepo
	cryo        *repo.CryoRepo
	batches     *repo.BatchRepo
}

// Apply 申请观测窗口。
func (s *WindowService) Apply(ctx context.Context, w model.ObservationWindow, actor string) (*model.ObservationWindow, error) {
	if actor == "" {
		return nil, apperr.New(apperr.CodeActorRequired, "窗口申请必须提供操作人")
	}
	if err := domain.ValidateWindowSpan(w.StartAt, w.EndAt); err != nil {
		return nil, err
	}
	if w.Title == "" {
		return nil, apperr.InvalidArgument("窗口标题不能为空")
	}
	w.Status = domain.WindowApplied
	w.AppliedBy = actor
	now := s.svc.Clock.Now()
	err := s.svc.DB.InTx(ctx, func(tx repo.Tx) error {
		in, err := s.instruments.Get(ctx, tx, w.InstrumentID)
		if err != nil {
			return err
		}
		if in.Status == domain.InstrumentDecommissioned {
			return apperr.Precondition("仪器已停用，禁止申请观测窗口")
		}
		if err := s.windows.Create(ctx, tx, &w, now); err != nil {
			return err
		}
		return s.svc.Audit.Log(ctx, tx, domain.EntityWindow, w.ID, "apply", actor, w, now)
	})
	if err != nil {
		return nil, err
	}
	return &w, nil
}

// Get 查询窗口。
func (s *WindowService) Get(ctx context.Context, id int64) (*model.ObservationWindow, error) {
	return s.windows.Get(ctx, s.svc.DB.SQL, id)
}

// List 分页查询窗口。
func (s *WindowService) List(ctx context.Context, instrumentID int64, status string, page repo.Page) ([]model.ObservationWindow, error) {
	return s.windows.List(ctx, s.svc.DB.SQL, instrumentID, status, page)
}

// Approve 批准窗口：冻结仪器配置、通道、校准方案与目标优先级快照。
func (s *WindowService) Approve(ctx context.Context, id, version, planID int64, actor string) (*model.ObservationWindow, error) {
	now := s.svc.Clock.Now()
	var updated *model.ObservationWindow
	err := s.svc.DB.InTx(ctx, func(tx repo.Tx) error {
		w, err := s.windows.Get(ctx, tx, id)
		if err != nil {
			return err
		}
		if err := domain.MustTransition(domain.EntityWindow, w.Status, domain.WindowApproved); err != nil {
			return err
		}
		plan, err := s.calibration.GetPlan(ctx, tx, planID)
		if err != nil {
			return err
		}
		if plan.InstrumentID != w.InstrumentID {
			return apperr.Precondition("校准方案不属于该窗口仪器")
		}
		if plan.Status != domain.PlanActive {
			return apperr.Precondition("仅启用中的校准方案可被窗口冻结")
		}
		if err := domain.EnsureBatchCoverage(plan.ValidFrom, plan.ValidUntil, w.StartAt, w.EndAt); err != nil {
			return err
		}
		// 批准阶段允许多个窗口竞争同一仪器（重叠由排程冲突查询暴露），
		// 仅激活阶段硬性排斥已激活的重叠窗口。
		in, err := s.instruments.Get(ctx, tx, w.InstrumentID)
		if err != nil {
			return err
		}
		channels, err := s.channels.ListAllByInstrument(ctx, tx, w.InstrumentID)
		if err != nil {
			return err
		}
		targets, err := s.targets.ListAllByWindow(ctx, tx, w.ID)
		if err != nil {
			return err
		}
		snap := domain.BuildFreezeSnapshot(in, channels, plan, targets, now, actor)
		raw, err := json.Marshal(snap)
		if err != nil {
			return err
		}
		if err := s.windows.Approve(ctx, tx, id, version, planID, string(raw), actor, now); err != nil {
			return err
		}
		if err := s.svc.Audit.Log(ctx, tx, domain.EntityWindow, id, "approve", actor, snap, now); err != nil {
			return err
		}
		w.Status = domain.WindowApproved
		w.PlanID = planID
		w.FrozenSnapshot = string(raw)
		w.ApprovedBy = actor
		w.Version++
		updated = w
		return nil
	})
	if err != nil {
		return nil, err
	}
	return updated, nil
}

// Activate 激活窗口：真实事务——校准封存（active→sealed）与观测启用（窗口 active、仪器 observing）。
// 任一步骤失败，全部回滚。
func (s *WindowService) Activate(ctx context.Context, id, version int64, actor string) (*model.ObservationWindow, error) {
	now := s.svc.Clock.Now()
	var updated *model.ObservationWindow
	err := s.svc.DB.InTx(ctx, func(tx repo.Tx) error {
		w, err := s.windows.Get(ctx, tx, id)
		if err != nil {
			return err
		}
		if err := domain.MustTransition(domain.EntityWindow, w.Status, domain.WindowActive); err != nil {
			return err
		}
		plan, err := s.calibration.GetPlan(ctx, tx, w.PlanID)
		if err != nil {
			return err
		}
		if err := domain.MustTransition(domain.EntityPlan, plan.Status, domain.PlanSealed); err != nil {
			return err
		}
		if err := domain.EnsureBatchCoverage(plan.ValidFrom, plan.ValidUntil, w.StartAt, w.EndAt); err != nil {
			return err
		}
		if _, err := s.calibration.LatestPassingRecord(ctx, tx, plan.ID, now); err != nil {
			return apperr.Precondition("缺少先于观测开始的合格校准记录，禁止激活窗口")
		}
		cryoSys, err := s.cryo.GetSystemByInstrument(ctx, tx, w.InstrumentID)
		if err != nil {
			return err
		}
		if cryoSys.Status != domain.CryoStable {
			return apperr.Precondition("低温系统未处于稳定状态，禁止激活窗口")
		}
		latest, err := s.cryo.LatestReading(ctx, tx, cryoSys.ID)
		if err != nil {
			return err
		}
		in, err := s.instruments.Get(ctx, tx, w.InstrumentID)
		if err != nil {
			return err
		}
		if err := domain.EnsureReadingInRange(latest.TempMK, in.TempMinMK, in.TempMaxMK); err != nil {
			return err
		}
		activeOverlap, err := s.windows.HasActiveOverlap(ctx, tx, w.InstrumentID, w.ID, w.StartAt, w.EndAt)
		if err != nil {
			return err
		}
		if activeOverlap {
			return apperr.Conflict("同仪器存在时间重叠的已激活窗口，禁止并行观测")
		}
		if err := domain.MustTransition(domain.EntityInstrument, in.Status, domain.InstrumentObserving); err != nil {
			return err
		}
		// 校准封存。
		if err := s.calibration.UpdatePlanStatus(ctx, tx, plan.ID, plan.Version, domain.PlanSealed, now); err != nil {
			return err
		}
		// 观测启用。
		if err := s.windows.UpdateStatus(ctx, tx, id, version, domain.WindowActive, now); err != nil {
			return err
		}
		if err := s.instruments.UpdateStatus(ctx, tx, in.ID, in.Version, domain.InstrumentObserving, now); err != nil {
			return err
		}
		if err := s.instruments.AddHistory(ctx, tx, &model.InstrumentStatusHistory{
			InstrumentID: in.ID, FromStatus: in.Status, ToStatus: domain.InstrumentObserving,
			Reason: fmt.Sprintf("窗口 %d 激活", w.ID), Actor: actor,
		}, now); err != nil {
			return err
		}
		if _, err := s.svc.Jobs.Enqueue(ctx, tx, domain.JobWindowEnd,
			fmt.Sprintf(`{"window_id":%d}`, w.ID), w.EndAt, s.svc.MaxAttempts, now); err != nil {
			return err
		}
		if err := s.svc.Audit.Log(ctx, tx, domain.EntityWindow, id, "activate", actor,
			map[string]any{"plan_id": plan.ID, "plan_status": "sealed"}, now); err != nil {
			return err
		}
		w.Status = domain.WindowActive
		w.Version++
		updated = w
		return nil
	})
	if err != nil {
		return nil, err
	}
	return updated, nil
}

// Close 关闭窗口：仪器转就绪，采集中的批次自动冻结（同一事务）。
func (s *WindowService) Close(ctx context.Context, id, version int64, actor, reason string) error {
	now := s.svc.Clock.Now()
	return s.svc.DB.InTx(ctx, func(tx repo.Tx) error {
		w, err := s.windows.Get(ctx, tx, id)
		if err != nil {
			return err
		}
		if err := domain.MustTransition(domain.EntityWindow, w.Status, domain.WindowClosed); err != nil {
			return err
		}
		if err := s.windows.UpdateStatus(ctx, tx, id, version, domain.WindowClosed, now); err != nil {
			return err
		}
		in, err := s.instruments.Get(ctx, tx, w.InstrumentID)
		if err != nil {
			return err
		}
		if in.Status == domain.InstrumentObserving {
			if err := s.instruments.UpdateStatus(ctx, tx, in.ID, in.Version, domain.InstrumentReady, now); err != nil {
				return err
			}
			if err := s.instruments.AddHistory(ctx, tx, &model.InstrumentStatusHistory{
				InstrumentID: in.ID, FromStatus: domain.InstrumentObserving, ToStatus: domain.InstrumentReady,
				Reason: reason, Actor: actor,
			}, now); err != nil {
				return err
			}
		}
		batches, err := s.batches.ListByWindow(ctx, tx, w.ID, repo.Page{Limit: 100})
		if err != nil {
			return err
		}
		for _, b := range batches {
			if b.Status == domain.BatchAcquiring {
				if err := s.batches.Finish(ctx, tx, b.ID, b.Version, domain.BatchFrozen, now); err != nil {
					return err
				}
			}
		}
		return s.svc.Audit.Log(ctx, tx, domain.EntityWindow, id, "close", actor,
			map[string]string{"reason": reason}, now)
	})
}

// Cancel 撤销窗口（applied/approved）。
func (s *WindowService) Cancel(ctx context.Context, id, version int64, actor string) error {
	now := s.svc.Clock.Now()
	return s.svc.DB.InTx(ctx, func(tx repo.Tx) error {
		w, err := s.windows.Get(ctx, tx, id)
		if err != nil {
			return err
		}
		if err := domain.MustTransition(domain.EntityWindow, w.Status, domain.WindowCancelled); err != nil {
			return err
		}
		if err := s.windows.UpdateStatus(ctx, tx, id, version, domain.WindowCancelled, now); err != nil {
			return err
		}
		return s.svc.Audit.Log(ctx, tx, domain.EntityWindow, id, "cancel", actor, nil, now)
	})
}

