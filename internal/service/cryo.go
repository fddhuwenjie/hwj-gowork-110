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

// CryoService 负责低温系统登记、预冷会话与温度读数处置。
type CryoService struct {
	svc         *Services
	cryo        *repo.CryoRepo
	instruments *repo.InstrumentRepo
	batches     *repo.BatchRepo
	anomalies   *repo.AnomalyRepo
}

// RegisterSystem 为仪器登记低温系统。
func (s *CryoService) RegisterSystem(ctx context.Context, instrumentID int64, name string, targetTempMK float64) (*model.CryoSystem, error) {
	if name == "" {
		return nil, apperr.InvalidArgument("低温系统名称不能为空")
	}
	c := &model.CryoSystem{
		InstrumentID: instrumentID, Name: name,
		Status: domain.CryoIdle, TargetTempMK: targetTempMK,
	}
	now := s.svc.Clock.Now()
	err := s.svc.DB.InTx(ctx, func(tx repo.Tx) error {
		if _, err := s.instruments.Get(ctx, tx, instrumentID); err != nil {
			return err
		}
		if err := s.cryo.CreateSystem(ctx, tx, c, now); err != nil {
			return err
		}
		return s.svc.Audit.Log(ctx, tx, domain.EntityCryo, c.ID, "register", "system", c, now)
	})
	if err != nil {
		return nil, err
	}
	return c, nil
}

// GetSystem 查询低温系统。
func (s *CryoService) GetSystem(ctx context.Context, id int64) (*model.CryoSystem, error) {
	return s.cryo.GetSystem(ctx, s.svc.DB.SQL, id)
}

// ListSessions 分页查询预冷会话。
func (s *CryoService) ListSessions(ctx context.Context, cryoID int64, page repo.Page) ([]model.PrecoolSession, error) {
	return s.cryo.ListSessions(ctx, s.svc.DB.SQL, cryoID, page)
}

// ListReadings 分页查询温度读数。
func (s *CryoService) ListReadings(ctx context.Context, cryoID int64, page repo.Page) ([]model.CryoReading, error) {
	return s.cryo.ListReadings(ctx, s.svc.DB.SQL, cryoID, page)
}

// StartPrecool 开始预冷：创建会话、低温系统与仪器状态推进、排程预冷超时作业（同一事务）。
func (s *CryoService) StartPrecool(ctx context.Context, cryoID int64, targetTempMK float64,
	deadline time.Time, actor string) (*model.PrecoolSession, error) {
	now := s.svc.Clock.Now()
	if !deadline.After(now) {
		return nil, apperr.InvalidArgument("预冷截止时间必须晚于当前时间")
	}
	var session *model.PrecoolSession
	err := s.svc.DB.InTx(ctx, func(tx repo.Tx) error {
		c, err := s.cryo.GetSystem(ctx, tx, cryoID)
		if err != nil {
			return err
		}
		if err := domain.MustTransition(domain.EntityCryo, c.Status, domain.CryoPrecooling); err != nil {
			return err
		}
		in, err := s.instruments.Get(ctx, tx, c.InstrumentID)
		if err != nil {
			return err
		}
		if err := domain.MustTransition(domain.EntityInstrument, in.Status, domain.InstrumentPrecooling); err != nil {
			return err
		}
		session = &model.PrecoolSession{
			CryoSystemID: cryoID, Status: domain.PrecoolInProgress,
			TargetTempMK: targetTempMK, StartedAt: now, DeadlineAt: deadline,
		}
		if err := s.cryo.CreateSession(ctx, tx, session, now); err != nil {
			return err
		}
		if err := s.cryo.UpdateSystemStatus(ctx, tx, cryoID, c.Version, domain.CryoPrecooling, now); err != nil {
			return err
		}
		if err := s.instruments.UpdateStatus(ctx, tx, in.ID, in.Version, domain.InstrumentPrecooling, now); err != nil {
			return err
		}
		if err := s.instruments.AddHistory(ctx, tx, &model.InstrumentStatusHistory{
			InstrumentID: in.ID, FromStatus: in.Status, ToStatus: domain.InstrumentPrecooling,
			Reason: "开始预冷", Actor: actor,
		}, now); err != nil {
			return err
		}
		if _, err := s.svc.Jobs.Enqueue(ctx, tx, domain.JobPrecoolTimeout,
			fmt.Sprintf(`{"session_id":%d}`, session.ID), deadline, s.svc.MaxAttempts, now); err != nil {
			return err
		}
		return s.svc.Audit.Log(ctx, tx, domain.EntityPrecool, session.ID, "start", actor, session, now)
	})
	if err != nil {
		return nil, err
	}
	return session, nil
}

// AddReading 写入温度读数（幂等）：
//   - 预冷中读数进入有效区间 → 会话转稳、低温转稳、仪器就绪；
//   - 观测期间读数越界 → 低温转异常、当前批次隔离、登记异常（同一事务）。
func (s *CryoService) AddReading(ctx context.Context, cryoID int64, tempMK, pressureMbar float64,
	recordedAt time.Time, key, actor string) (*model.CryoReading, bool, error) {
	if key == "" {
		return nil, false, apperr.InvalidArgument("温度读数必须携带 idempotency_key")
	}
	now := s.svc.Clock.Now()
	// 迟到上传不能改变观测发生日的归属：保留调用方传入的 recorded_at；
	// 未提供（零值）时回落到当前时刻以维持相邻合法实时流程的行为。
	if recordedAt.IsZero() {
		recordedAt = now
	}
	rd := &model.CryoReading{
		CryoSystemID: cryoID, TempMK: tempMK, PressureMbar: pressureMbar,
		RecordedAt: recordedAt, IdempotencyKey: key,
	}
	var replay bool
	err := s.svc.DB.InTx(ctx, func(tx repo.Tx) error {
		c, err := s.cryo.GetSystem(ctx, tx, cryoID)
		if err != nil {
			return err
		}
		in, err := s.instruments.Get(ctx, tx, c.InstrumentID)
		if err != nil {
			return err
		}
		replay, err = s.cryo.InsertReading(ctx, tx, rd, now)
		if err != nil {
			return err
		}
		if replay {
			return nil
		}
		inRange := domain.TempInRange(tempMK, in.TempMinMK, in.TempMaxMK)
		if inRange && c.Status == domain.CryoPrecooling {
			if err := s.stabilize(ctx, tx, c, in, actor, now); err != nil {
				return err
			}
		}
		if !inRange && in.Status == domain.InstrumentObserving {
			if err := s.handleOutOfRange(ctx, tx, c, in, rd, actor, now); err != nil {
				return err
			}
		}
		return s.svc.Audit.Log(ctx, tx, domain.EntityCryo, cryoID, "reading", actor, rd, now)
	})
	if err != nil {
		return nil, false, err
	}
	return rd, replay, nil
}

// stabilize 预冷达标：会话转稳、低温转稳、仪器就绪。
func (s *CryoService) stabilize(ctx context.Context, tx repo.Tx, c *model.CryoSystem,
	in *model.Instrument, actor string, now time.Time) error {
	sessions, err := s.cryo.ListSessions(ctx, tx, c.ID, repo.Page{Limit: 100})
	if err != nil {
		return err
	}
	for _, sess := range sessions {
		if sess.Status == domain.PrecoolInProgress {
			if err := s.cryo.FinishSession(ctx, tx, sess.ID, sess.Version, domain.PrecoolStable, now); err != nil {
				return err
			}
		}
	}
	if err := s.cryo.UpdateSystemStatus(ctx, tx, c.ID, c.Version, domain.CryoStable, now); err != nil {
		return err
	}
	if in.Status == domain.InstrumentPrecooling {
		if err := s.instruments.UpdateStatus(ctx, tx, in.ID, in.Version, domain.InstrumentReady, now); err != nil {
			return err
		}
		return s.instruments.AddHistory(ctx, tx, &model.InstrumentStatusHistory{
			InstrumentID: in.ID, FromStatus: in.Status, ToStatus: domain.InstrumentReady,
			Reason: "预冷达标", Actor: actor,
		}, now)
	}
	return nil
}

// handleOutOfRange 观测期间越界：低温转异常、采集中的批次隔离并登记异常。
func (s *CryoService) handleOutOfRange(ctx context.Context, tx repo.Tx, c *model.CryoSystem,
	in *model.Instrument, rd *model.CryoReading, actor string, now time.Time) error {
	if err := s.cryo.UpdateSystemStatus(ctx, tx, c.ID, c.Version, domain.CryoAbnormal, now); err != nil {
		return err
	}
	batches, err := s.batches.AcquiringByInstrument(ctx, tx, in.ID)
	if err != nil {
		return err
	}
	for _, b := range batches {
		if err := s.batches.Finish(ctx, tx, b.ID, b.Version, domain.BatchIsolated, now); err != nil {
			return err
		}
		a := &model.Anomaly{
			BatchID: &b.ID, InstrumentID: in.ID, Kind: domain.AnomalyTempOutOfRange,
			Description: fmt.Sprintf("观测期间温度读数 %.3f mK 越界，批次 %d 已隔离", rd.TempMK, b.ID),
			Status:      domain.AnomalyOpen, OpenedBy: actor,
		}
		if err := s.anomalies.Create(ctx, tx, a, now); err != nil {
			return err
		}
	}
	return nil
}
