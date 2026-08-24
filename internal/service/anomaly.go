package service

import (
	"context"

	"observatory/internal/apperr"
	"observatory/internal/domain"
	"observatory/internal/model"
	"observatory/internal/repo"
)

// AnomalyService 负责异常的手工登记与闭环处置。
type AnomalyService struct {
	svc         *Services
	anomalies   *repo.AnomalyRepo
	batches     *repo.BatchRepo
	instruments *repo.InstrumentRepo
}

// CreateManual 手工登记异常（可关联批次）。
func (s *AnomalyService) CreateManual(ctx context.Context, batchID *int64, instrumentID int64,
	kind, description, actor string) (*model.Anomaly, error) {
	if actor == "" {
		return nil, apperr.New(apperr.CodeActorRequired, "登记异常必须提供操作人")
	}
	if description == "" {
		return nil, apperr.InvalidArgument("异常描述不能为空")
	}
	if batchID == nil && instrumentID <= 0 {
		return nil, apperr.InvalidArgument("独立异常必须指定仪器")
	}
	a := &model.Anomaly{
		BatchID: batchID, InstrumentID: instrumentID, Kind: kind,
		Description: description, Status: domain.AnomalyOpen, OpenedBy: actor,
	}
	now := s.svc.Clock.Now()
	err := s.svc.DB.InTx(ctx, func(tx repo.Tx) error {
		if batchID != nil {
			b, err := s.batches.Get(ctx, tx, *batchID)
			if err != nil {
				return err
			}
			a.InstrumentID = b.InstrumentID
		} else {
			if _, err := s.instruments.Get(ctx, tx, instrumentID); err != nil {
				return err
			}
		}
		if err := s.anomalies.Create(ctx, tx, a, now); err != nil {
			return err
		}
		return s.svc.Audit.Log(ctx, tx, domain.EntityAnomaly, a.ID, "open", actor, a, now)
	})
	if err != nil {
		return nil, err
	}
	return a, nil
}

// transition 异常状态机转换。
func (s *AnomalyService) transition(ctx context.Context, id int64, to, actor string) (*model.Anomaly, error) {
	now := s.svc.Clock.Now()
	var updated *model.Anomaly
	err := s.svc.DB.InTx(ctx, func(tx repo.Tx) error {
		a, err := s.anomalies.Get(ctx, tx, id)
		if err != nil {
			return err
		}
		if err := domain.MustTransition(domain.EntityAnomaly, a.Status, to); err != nil {
			return err
		}
		resolvedBy := ""
		if to == domain.AnomalyResolved || to == domain.AnomalyClosed {
			resolvedBy = actor
		}
		if err := s.anomalies.UpdateStatus(ctx, tx, id, to, resolvedBy, now); err != nil {
			return err
		}
		if err := s.svc.Audit.Log(ctx, tx, domain.EntityAnomaly, id, "status:"+to, actor, nil, now); err != nil {
			return err
		}
		a.Status = to
		a.ResolvedBy = resolvedBy
		updated = a
		return nil
	})
	if err != nil {
		return nil, err
	}
	return updated, nil
}

// Resolve 处置完成。
func (s *AnomalyService) Resolve(ctx context.Context, id int64, actor string) (*model.Anomaly, error) {
	return s.transition(ctx, id, domain.AnomalyResolved, actor)
}

// Close 关闭异常。
func (s *AnomalyService) Close(ctx context.Context, id int64, actor string) (*model.Anomaly, error) {
	return s.transition(ctx, id, domain.AnomalyClosed, actor)
}

// List 分页查询异常。
func (s *AnomalyService) List(ctx context.Context, instrumentID int64, status string, page repo.Page) ([]model.Anomaly, error) {
	return s.anomalies.List(ctx, s.svc.DB.SQL, instrumentID, status, page)
}
