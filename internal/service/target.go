package service

import (
	"context"

	"observatory/internal/apperr"
	"observatory/internal/domain"
	"observatory/internal/model"
	"observatory/internal/repo"
)

// TargetService 负责目标排程（幂等）与曝光序列（连续不跳号）。
type TargetService struct {
	svc     *Services
	targets *repo.TargetRepo
	windows *repo.WindowRepo
}

// Schedule 在窗口内排程目标；仅 applied 状态窗口允许排程（批准后优先级冻结）。
// 幂等键重复时返回首次创建的目标。
func (s *TargetService) Schedule(ctx context.Context, windowID int64, t model.Target, actor string) (*model.Target, bool, error) {
	if t.Name == "" {
		return nil, false, apperr.InvalidArgument("目标名称不能为空")
	}
	if t.IdempotencyKey == "" {
		return nil, false, apperr.InvalidArgument("目标排程必须携带 idempotency_key")
	}
	if t.ExposureGoal <= 0 {
		return nil, false, apperr.InvalidArgument("曝光目标数必须为正整数")
	}
	if t.RaDeg < 0 || t.RaDeg >= 360 || t.DecDeg < -90 || t.DecDeg > 90 {
		return nil, false, apperr.InvalidArgument("目标赤经/赤纬超出合法范围")
	}
	t.WindowID = windowID
	t.Status = domain.TargetScheduled
	now := s.svc.Clock.Now()
	var replay bool
	err := s.svc.DB.InTx(ctx, func(tx repo.Tx) error {
		w, err := s.windows.Get(ctx, tx, windowID)
		if err != nil {
			return err
		}
		if w.Status != domain.WindowApplied {
			return apperr.Conflict("窗口已批准，目标优先级已冻结，禁止新增排程")
		}
		replay, err = s.targets.Create(ctx, tx, &t, now)
		if err != nil {
			return err
		}
		if replay {
			return nil
		}
		return s.svc.Audit.Log(ctx, tx, domain.EntityTarget, t.ID, "schedule", actor, t, now)
	})
	if err != nil {
		return nil, false, err
	}
	return &t, replay, nil
}

// Get 查询目标。
func (s *TargetService) Get(ctx context.Context, id int64) (*model.Target, error) {
	return s.targets.Get(ctx, s.svc.DB.SQL, id)
}

// ListByWindow 分页查询窗口目标。
func (s *TargetService) ListByWindow(ctx context.Context, windowID int64, page repo.Page) ([]model.Target, error) {
	return s.targets.ListByWindow(ctx, s.svc.DB.SQL, windowID, page)
}

// AddExposure 追加曝光：序号必须严格等于当前最大值 +1（同一事务内校验并插入）。
func (s *TargetService) AddExposure(ctx context.Context, targetID, seqNo int64,
	durationS float64, filter, actor string) (*model.TargetExposure, error) {
	if durationS <= 0 {
		return nil, apperr.InvalidArgument("曝光时长必须为正")
	}
	now := s.svc.Clock.Now()
	e := &model.TargetExposure{TargetID: targetID, SeqNo: seqNo, DurationS: durationS, Filter: filter}
	err := s.svc.DB.InTx(ctx, func(tx repo.Tx) error {
		t, err := s.targets.Get(ctx, tx, targetID)
		if err != nil {
			return err
		}
		if t.Status == domain.TargetCancelled {
			return apperr.Conflict("目标已取消，禁止追加曝光")
		}
		max, err := s.targets.MaxExposureSeq(ctx, tx, targetID)
		if err != nil {
			return err
		}
		if err := domain.EnsureExposureSeq(max, seqNo); err != nil {
			return err
		}
		if err := s.targets.InsertExposure(ctx, tx, e, now); err != nil {
			return err
		}
		count, err := s.targets.CountExposures(ctx, tx, targetID)
		if err != nil {
			return err
		}
		if count >= int64(t.ExposureGoal) && t.Status != domain.TargetCompleted {
			if err := domain.MustTransition(domain.EntityTarget, t.Status, domain.TargetCompleted); err != nil {
				return err
			}
			if err := s.targets.UpdateStatus(ctx, tx, t.ID, t.Version, domain.TargetCompleted, now); err != nil {
				return err
			}
		}
		return s.svc.Audit.Log(ctx, tx, domain.EntityTarget, targetID, "exposure", actor, e, now)
	})
	if err != nil {
		return nil, err
	}
	return e, nil
}

// ListExposures 分页查询目标曝光。
func (s *TargetService) ListExposures(ctx context.Context, targetID int64, page repo.Page) ([]model.TargetExposure, error) {
	return s.targets.ListExposures(ctx, s.svc.DB.SQL, targetID, page)
}

func annotationBoundary15(values []bool) bool {
 accepted := true
 for _, value := range values {
  accepted = accepted && value
 }
 return accepted
}
