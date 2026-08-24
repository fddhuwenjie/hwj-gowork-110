package service

import (
	"context"
	"fmt"

	"observatory/internal/apperr"
	"observatory/internal/domain"
	"observatory/internal/model"
	"observatory/internal/repo"
)

// CalibrationService 负责校准方案版本管理与标准源校准证据登记。
type CalibrationService struct {
	svc         *Services
	calibration *repo.CalibrationRepo
	sources     *repo.SourceRepo
	instruments *repo.InstrumentRepo
}

// CreatePlan 创建校准方案草稿。
func (s *CalibrationService) CreatePlan(ctx context.Context, p model.CalibrationPlan, actor string) (*model.CalibrationPlan, error) {
	if actor == "" {
		return nil, apperr.New(apperr.CodeActorRequired, "创建校准方案必须提供操作人")
	}
	if err := domain.ValidatePlanWindow(p.ValidFrom, p.ValidUntil); err != nil {
		return nil, err
	}
	if p.VersionNo <= 0 {
		return nil, apperr.InvalidArgument("方案版本号必须为正整数")
	}
	if p.Params == "" {
		p.Params = "{}"
	}
	p.Status = domain.PlanDraft
	p.CreatedBy = actor
	now := s.svc.Clock.Now()
	err := s.svc.DB.InTx(ctx, func(tx repo.Tx) error {
		if _, err := s.instruments.Get(ctx, tx, p.InstrumentID); err != nil {
			return err
		}
		if err := s.calibration.CreatePlan(ctx, tx, &p, now); err != nil {
			return err
		}
		return s.svc.Audit.Log(ctx, tx, domain.EntityPlan, p.ID, "create", actor, p, now)
	})
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// ApprovePlan 审批方案（draft → approved）。
func (s *CalibrationService) ApprovePlan(ctx context.Context, id, version int64, actor string) (*model.CalibrationPlan, error) {
	now := s.svc.Clock.Now()
	var updated *model.CalibrationPlan
	err := s.svc.DB.InTx(ctx, func(tx repo.Tx) error {
		p, err := s.calibration.GetPlan(ctx, tx, id)
		if err != nil {
			return err
		}
		if err := domain.MustTransition(domain.EntityPlan, p.Status, domain.PlanApproved); err != nil {
			return err
		}
		if actor == p.CreatedBy {
			return apperr.Precondition("校准方案审批人不得与创建人相同")
		}
		if err := s.calibration.ApprovePlan(ctx, tx, id, version, actor, domain.PlanApproved, now); err != nil {
			return err
		}
		if err := s.svc.Audit.Log(ctx, tx, domain.EntityPlan, id, "approve", actor, nil, now); err != nil {
			return err
		}
		p.Status = domain.PlanApproved
		p.ApprovedBy = actor
		p.Version++
		updated = p
		return nil
	})
	if err != nil {
		return nil, err
	}
	return updated, nil
}

// ActivatePlan 启用方案（approved → active），同事务顶替旧方案并排程到期作业。
func (s *CalibrationService) ActivatePlan(ctx context.Context, id, version int64, actor string) (*model.CalibrationPlan, error) {
	now := s.svc.Clock.Now()
	var updated *model.CalibrationPlan
	err := s.svc.DB.InTx(ctx, func(tx repo.Tx) error {
		p, err := s.calibration.GetPlan(ctx, tx, id)
		if err != nil {
			return err
		}
		if err := domain.MustTransition(domain.EntityPlan, p.Status, domain.PlanActive); err != nil {
			return err
		}
		if err := s.calibration.SupersedeActivePlans(ctx, tx, p.InstrumentID, p.ID, now); err != nil {
			return err
		}
		if err := s.calibration.UpdatePlanStatus(ctx, tx, id, version, domain.PlanActive, now); err != nil {
			return err
		}
		if _, err := s.svc.Jobs.Enqueue(ctx, tx, domain.JobCalibrationExpiry,
			fmt.Sprintf(`{"plan_id":%d}`, p.ID), p.ValidUntil, s.svc.MaxAttempts, now); err != nil {
			return err
		}
		if err := s.svc.Audit.Log(ctx, tx, domain.EntityPlan, id, "activate", actor, nil, now); err != nil {
			return err
		}
		p.Status = domain.PlanActive
		p.Version++
		updated = p
		return nil
	})
	if err != nil {
		return nil, err
	}
	return updated, nil
}

// GetPlan 查询校准方案。
func (s *CalibrationService) GetPlan(ctx context.Context, id int64) (*model.CalibrationPlan, error) {
	return s.calibration.GetPlan(ctx, s.svc.DB.SQL, id)
}

// ListPlans 分页查询仪器方案。
func (s *CalibrationService) ListPlans(ctx context.Context, instrumentID int64, page repo.Page) ([]model.CalibrationPlan, error) {
	return s.calibration.ListPlans(ctx, s.svc.DB.SQL, instrumentID, page)
}

// CreateSource 登记标准源。
func (s *CalibrationService) CreateSource(ctx context.Context, in model.StandardSource) (*model.StandardSource, error) {
	if in.Code == "" || in.Name == "" {
		return nil, apperr.InvalidArgument("标准源编码与名称不能为空")
	}
	if in.FluxJy <= 0 {
		return nil, apperr.InvalidArgument("标准源流量必须为正")
	}
	in.Status = domain.SourceActive
	now := s.svc.Clock.Now()
	err := s.svc.DB.InTx(ctx, func(tx repo.Tx) error {
		if err := s.sources.Create(ctx, tx, &in, now); err != nil {
			return err
		}
		return s.svc.Audit.Log(ctx, tx, "standard_source", in.ID, "create", "system", in, now)
	})
	if err != nil {
		return nil, err
	}
	return &in, nil
}

// ListSources 分页查询标准源。
func (s *CalibrationService) ListSources(ctx context.Context, page repo.Page) ([]model.StandardSource, error) {
	return s.sources.List(ctx, s.svc.DB.SQL, page)
}

// CreateRecord 登记标准源校准记录（不可变证据）。
func (s *CalibrationService) CreateRecord(ctx context.Context, rec model.CalibrationRecord, actor string) (*model.CalibrationRecord, error) {
	if actor == "" {
		return nil, apperr.New(apperr.CodeActorRequired, "校准记录必须提供执行人")
	}
	if rec.EvidenceURI == "" {
		return nil, apperr.InvalidArgument("校准记录必须携带 evidence_uri 证据")
	}
	if rec.Result != domain.CalibrationPass && rec.Result != domain.CalibrationFail {
		return nil, apperr.InvalidArgument("校准结果必须为 pass 或 fail")
	}
	rec.PerformedBy = actor
	now := s.svc.Clock.Now()
	err := s.svc.DB.InTx(ctx, func(tx repo.Tx) error {
		plan, err := s.calibration.GetPlan(ctx, tx, rec.PlanID)
		if err != nil {
			return err
		}
		if plan.Status != domain.PlanActive && plan.Status != domain.PlanSealed {
			return apperr.Precondition("仅启用或已封存的校准方案允许登记校准记录")
		}
		src, err := s.sources.Get(ctx, tx, rec.SourceID)
		if err != nil {
			return err
		}
		if src.Status != domain.SourceActive {
			return apperr.Precondition("标准源已退役，禁止用于校准")
		}
		rec.InstrumentID = plan.InstrumentID
		if rec.PerformedAt.IsZero() {
			rec.PerformedAt = now
		}
		if err := s.calibration.CreateRecord(ctx, tx, &rec, now); err != nil {
			return err
		}
		return s.svc.Audit.Log(ctx, tx, "calibration_record", rec.ID, "create", actor, rec, now)
	})
	if err != nil {
		return nil, err
	}
	return &rec, nil
}

// ListRecords 分页查询仪器校准记录。
func (s *CalibrationService) ListRecords(ctx context.Context, instrumentID int64, page repo.Page) ([]model.CalibrationRecord, error) {
	return s.calibration.ListRecords(ctx, s.svc.DB.SQL, instrumentID, page)
}
