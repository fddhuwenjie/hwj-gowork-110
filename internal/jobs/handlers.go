package jobs

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"observatory/internal/domain"
	"observatory/internal/model"
	"observatory/internal/repo"
	"observatory/internal/service"
)

// RegisterHandlers 注册四类内置作业处理器。
func RegisterHandlers(r *Runner, svc *service.Services) {
	r.Register(domain.JobPrecoolTimeout, precoolTimeoutHandler(svc))
	r.Register(domain.JobCalibrationExpiry, calibrationExpiryHandler(svc))
	r.Register(domain.JobWindowEnd, windowEndHandler(svc))
	r.Register(domain.JobArchiveVerify, archiveVerifyHandler(svc))
}

type sessionPayload struct {
	SessionID int64 `json:"session_id"`
}

// precoolTimeoutHandler 预冷超时：会话仍未转稳则置超时、低温回空闲、仪器转维护并登记异常。
func precoolTimeoutHandler(svc *service.Services) Handler {
	return func(ctx context.Context, payload string) error {
		var p sessionPayload
		if err := json.Unmarshal([]byte(payload), &p); err != nil {
			return fmt.Errorf("预冷超时作业载荷非法: %w", err)
		}
		now := svc.Clock.Now()
		return svc.DB.InTx(ctx, func(tx repo.Tx) error {
			sess, err := svc.CryoRepo().GetSession(ctx, tx, p.SessionID)
			if err != nil {
				return err
			}
			if sess.Status != domain.PrecoolInProgress {
				return nil // 已转稳/中止，幂等完成
			}
			if err := svc.CryoRepo().FinishSession(ctx, tx, sess.ID, sess.Version, domain.PrecoolTimeout, now); err != nil {
				return err
			}
			cryoSys, err := svc.CryoRepo().GetSystem(ctx, tx, sess.CryoSystemID)
			if err != nil {
				return err
			}
			if err := svc.CryoRepo().UpdateSystemStatus(ctx, tx, cryoSys.ID, cryoSys.Version, domain.CryoIdle, now); err != nil {
				return err
			}
			in, err := svc.InstrumentRepo().Get(ctx, tx, cryoSys.InstrumentID)
			if err != nil {
				return err
			}
			if in.Status == domain.InstrumentPrecooling {
				if err := svc.InstrumentRepo().UpdateStatus(ctx, tx, in.ID, in.Version, domain.InstrumentMaintenance, now); err != nil {
					return err
				}
				if err := svc.InstrumentRepo().AddHistory(ctx, tx, &model.InstrumentStatusHistory{
					InstrumentID: in.ID, FromStatus: domain.InstrumentPrecooling,
					ToStatus: domain.InstrumentMaintenance, Reason: "预冷超时", Actor: "job",
				}, now); err != nil {
					return err
				}
			}
			a := &model.Anomaly{
				InstrumentID: in.ID, Kind: domain.AnomalyPrecoolTimeout,
				Description: fmt.Sprintf("预冷会话 %d 超过截止时间仍未达标", sess.ID),
				Status:      domain.AnomalyOpen, OpenedBy: "job",
			}
			if err := svc.AnomalyRepo().Create(ctx, tx, a, now); err != nil {
				return err
			}
			return svc.Audit.Log(ctx, tx, domain.EntityPrecool, sess.ID, "timeout", "job", nil, now)
		})
	}
}

type planPayload struct {
	PlanID int64 `json:"plan_id"`
}

// calibrationExpiryHandler 校准到期：active/sealed 方案置 expired；仪器就绪但无有效方案时转维护。
func calibrationExpiryHandler(svc *service.Services) Handler {
	return func(ctx context.Context, payload string) error {
		var p planPayload
		if err := json.Unmarshal([]byte(payload), &p); err != nil {
			return fmt.Errorf("校准到期作业载荷非法: %w", err)
		}
		now := svc.Clock.Now()
		return svc.DB.InTx(ctx, func(tx repo.Tx) error {
			plan, err := svc.CalibrationRepo().GetPlan(ctx, tx, p.PlanID)
			if err != nil {
				return err
			}
			if plan.Status != domain.PlanActive && plan.Status != domain.PlanSealed {
				return nil
			}
			if plan.ValidUntil.After(now) {
				return fmt.Errorf("校准方案 %d 尚未到期（valid_until=%s）", plan.ID,
					plan.ValidUntil.Format(time.RFC3339))
			}
			if err := svc.CalibrationRepo().UpdatePlanStatus(ctx, tx, plan.ID, plan.Version, domain.PlanExpired, now); err != nil {
				return err
			}
			in, err := svc.InstrumentRepo().Get(ctx, tx, plan.InstrumentID)
			if err != nil {
				return err
			}
			if in.Status == domain.InstrumentReady {
				if _, err := svc.CalibrationRepo().GetActivePlan(ctx, tx, in.ID); err == nil {
					return nil // 仍有其他启用方案
				}
				if err := svc.InstrumentRepo().UpdateStatus(ctx, tx, in.ID, in.Version, domain.InstrumentMaintenance, now); err != nil {
					return err
				}
				if err := svc.InstrumentRepo().AddHistory(ctx, tx, &model.InstrumentStatusHistory{
					InstrumentID: in.ID, FromStatus: domain.InstrumentReady,
					ToStatus: domain.InstrumentMaintenance, Reason: "校准方案到期", Actor: "job",
				}, now); err != nil {
					return err
				}
				a := &model.Anomaly{
					InstrumentID: in.ID, Kind: domain.AnomalyCalibrationExpired,
					Description: fmt.Sprintf("校准方案 %d 已到期，仪器转入维护", plan.ID),
					Status:      domain.AnomalyOpen, OpenedBy: "job",
				}
				if err := svc.AnomalyRepo().Create(ctx, tx, a, now); err != nil {
					return err
				}
			}
			return svc.Audit.Log(ctx, tx, domain.EntityPlan, plan.ID, "expire", "job", nil, now)
		})
	}
}

type windowPayload struct {
	WindowID int64 `json:"window_id"`
}

// windowEndHandler 窗口结束：到期激活窗口关闭（复用窗口服务事务逻辑）。
func windowEndHandler(svc *service.Services) Handler {
	return func(ctx context.Context, payload string) error {
		var p windowPayload
		if err := json.Unmarshal([]byte(payload), &p); err != nil {
			return fmt.Errorf("窗口结束作业载荷非法: %w", err)
		}
		w, err := svc.Windows.Get(ctx, p.WindowID)
		if err != nil {
			return err
		}
		if w.Status != domain.WindowActive {
			return nil // 已关闭/撤销，幂等完成
		}
		if w.EndAt.After(svc.Clock.Now()) {
			return fmt.Errorf("窗口 %d 尚未到结束时间", w.ID)
		}
		return svc.Windows.Close(ctx, w.ID, w.Version, "job", "窗口到期自动关闭")
	}
}

type archivePayload struct {
	ArchiveID int64 `json:"archive_id"`
}

// archiveVerifyHandler 归档校验：调用归档服务独立校验逻辑。
func archiveVerifyHandler(svc *service.Services) Handler {
	return func(ctx context.Context, payload string) error {
		var p archivePayload
		if err := json.Unmarshal([]byte(payload), &p); err != nil {
			return fmt.Errorf("归档校验作业载荷非法: %w", err)
		}
		_, err := svc.Archives.Verify(ctx, p.ArchiveID, "job")
		return err
	}
}
