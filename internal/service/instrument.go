package service

import (
	"context"

	"observatory/internal/apperr"
	"observatory/internal/domain"
	"observatory/internal/model"
	"observatory/internal/repo"
)

// InstrumentService 负责仪器建档、配置更新、状态机与探测器通道管理。
type InstrumentService struct {
	svc         *Services
	instruments *repo.InstrumentRepo
	channels    *repo.ChannelRepo
	windows     *repo.WindowRepo
}

// CreateInstrument 在有效站点下建档仪器。
func (s *InstrumentService) CreateInstrument(ctx context.Context, in model.Instrument) (*model.Instrument, error) {
	if in.Code == "" || in.Name == "" {
		return nil, apperr.InvalidArgument("仪器编码与名称不能为空")
	}
	if err := domain.ValidateTempRange(in.TempMinMK, in.TempMaxMK); err != nil {
		return nil, err
	}
	in.Status = domain.InstrumentRegistered
	now := s.svc.Clock.Now()
	err := s.svc.DB.InTx(ctx, func(tx repo.Tx) error {
		site, err := s.svc.Sites.sites.Get(ctx, tx, in.SiteID)
		if err != nil {
			return err
		}
		if site.Status != domain.SiteActive {
			return apperr.Precondition("站点已停用，禁止建档仪器")
		}
		if err := s.instruments.Create(ctx, tx, &in, now); err != nil {
			return err
		}
		if err := s.instruments.AddHistory(ctx, tx, &model.InstrumentStatusHistory{
			InstrumentID: in.ID, FromStatus: "", ToStatus: in.Status, Reason: "仪器建档", Actor: "system",
		}, now); err != nil {
			return err
		}
		return s.svc.Audit.Log(ctx, tx, domain.EntityInstrument, in.ID, "create", "system", in, now)
	})
	if err != nil {
		return nil, err
	}
	return &in, nil
}

// GetInstrument 查询仪器。
func (s *InstrumentService) GetInstrument(ctx context.Context, id int64) (*model.Instrument, error) {
	return s.instruments.Get(ctx, s.svc.DB.SQL, id)
}

// ListInstruments 分页查询仪器。
func (s *InstrumentService) ListInstruments(ctx context.Context, siteID int64, status string, page repo.Page) ([]model.Instrument, error) {
	return s.instruments.List(ctx, s.svc.DB.SQL, siteID, status, page)
}

// ListHistory 查询仪器状态历史。
func (s *InstrumentService) ListHistory(ctx context.Context, id int64, page repo.Page) ([]model.InstrumentStatusHistory, error) {
	return s.instruments.ListHistory(ctx, s.svc.DB.SQL, id, page)
}

// ensureNotFrozen 窗口已批准/激活期间仪器配置被冻结。
func (s *InstrumentService) ensureNotFrozen(ctx context.Context, q repo.Tx, instrumentID int64) error {
	frozen, err := s.windows.HasOpenForInstrument(ctx, q, instrumentID)
	if err != nil {
		return err
	}
	if frozen {
		return apperr.Conflict("仪器存在已批准/已激活的观测窗口，配置已冻结，禁止变更")
	}
	return nil
}

// UpdateInstrument 乐观锁更新仪器配置（冻结期间拒绝）。
func (s *InstrumentService) UpdateInstrument(ctx context.Context, id, version int64,
	name, kind string, minMK, maxMK float64, actor string) (*model.Instrument, error) {
	now := s.svc.Clock.Now()
	var updated *model.Instrument
	err := s.svc.DB.InTx(ctx, func(tx repo.Tx) error {
		in, err := s.instruments.Get(ctx, tx, id)
		if err != nil {
			return err
		}
		if err := s.ensureNotFrozen(ctx, tx, id); err != nil {
			return err
		}
		if name != "" {
			in.Name = name
		}
		if kind != "" {
			in.Kind = kind
		}
		if minMK > 0 || maxMK > 0 {
			if err := domain.ValidateTempRange(minMK, maxMK); err != nil {
				return err
			}
			in.TempMinMK, in.TempMaxMK = minMK, maxMK
		}
		in.Version = version
		if err := s.instruments.Update(ctx, tx, in, now); err != nil {
			return err
		}
		if err := s.svc.Audit.Log(ctx, tx, domain.EntityInstrument, id, "update", actor, in, now); err != nil {
			return err
		}
		updated = in
		return nil
	})
	if err != nil {
		return nil, err
	}
	return updated, nil
}

// ChangeStatus 仪器状态机转换（维护/复位/停用等），写状态历史。
func (s *InstrumentService) ChangeStatus(ctx context.Context, id, version int64,
	to, reason, actor string) (*model.Instrument, error) {
	now := s.svc.Clock.Now()
	var updated *model.Instrument
	err := s.svc.DB.InTx(ctx, func(tx repo.Tx) error {
		in, err := s.instruments.Get(ctx, tx, id)
		if err != nil {
			return err
		}
		if err := domain.MustTransition(domain.EntityInstrument, in.Status, to); err != nil {
			return err
		}
		if err := s.instruments.UpdateStatus(ctx, tx, id, version, to, now); err != nil {
			return err
		}
		if err := s.instruments.AddHistory(ctx, tx, &model.InstrumentStatusHistory{
			InstrumentID: id, FromStatus: in.Status, ToStatus: to, Reason: reason, Actor: actor,
		}, now); err != nil {
			return err
		}
		if err := s.svc.Audit.Log(ctx, tx, domain.EntityInstrument, id, "status:"+to, actor,
			map[string]string{"from": in.Status, "to": to, "reason": reason}, now); err != nil {
			return err
		}
		in.Status = to
		in.Version++
		updated = in
		return nil
	})
	if err != nil {
		return nil, err
	}
	return updated, nil
}

// AddChannel 为仪器新增探测器通道（冻结期间拒绝）。
func (s *InstrumentService) AddChannel(ctx context.Context, instrumentID int64, c model.DetectorChannel) (*model.DetectorChannel, error) {
	if c.Name == "" || c.ChannelNo <= 0 {
		return nil, apperr.InvalidArgument("通道号必须为正整数且名称不能为空")
	}
	c.InstrumentID = instrumentID
	c.Status = domain.ChannelEnabled
	now := s.svc.Clock.Now()
	err := s.svc.DB.InTx(ctx, func(tx repo.Tx) error {
		if _, err := s.instruments.Get(ctx, tx, instrumentID); err != nil {
			return err
		}
		if err := s.ensureNotFrozen(ctx, tx, instrumentID); err != nil {
			return err
		}
		if err := s.channels.Create(ctx, tx, &c, now); err != nil {
			return err
		}
		return s.svc.Audit.Log(ctx, tx, domain.EntityChannel, c.ID, "create", "system", c, now)
	})
	if err != nil {
		return nil, err
	}
	return &c, nil
}

// UpdateChannel 乐观锁更新通道（冻结期间拒绝；状态转换受状态机约束）。
func (s *InstrumentService) UpdateChannel(ctx context.Context, id, version int64,
	name string, gain, offset float64, status string, actor string) (*model.DetectorChannel, error) {
	now := s.svc.Clock.Now()
	var updated *model.DetectorChannel
	err := s.svc.DB.InTx(ctx, func(tx repo.Tx) error {
		c, err := s.channels.Get(ctx, tx, id)
		if err != nil {
			return err
		}
		if err := s.ensureNotFrozen(ctx, tx, c.InstrumentID); err != nil {
			return err
		}
		if status != "" && status != c.Status {
			if err := domain.MustTransition(domain.EntityChannel, c.Status, status); err != nil {
				return err
			}
			c.Status = status
		}
		if name != "" {
			c.Name = name
		}
		c.Gain, c.Offset = gain, offset
		c.Version = version
		if err := s.channels.Update(ctx, tx, c, now); err != nil {
			return err
		}
		if err := s.svc.Audit.Log(ctx, tx, domain.EntityChannel, id, "update", actor, c, now); err != nil {
			return err
		}
		updated = c
		return nil
	})
	if err != nil {
		return nil, err
	}
	return updated, nil
}

// ListChannels 分页查询仪器通道。
func (s *InstrumentService) ListChannels(ctx context.Context, instrumentID int64, page repo.Page) ([]model.DetectorChannel, error) {
	return s.channels.ListByInstrument(ctx, s.svc.DB.SQL, instrumentID, page)
}
