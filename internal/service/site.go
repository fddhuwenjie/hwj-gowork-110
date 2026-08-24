package service

import (
	"context"

	"observatory/internal/apperr"
	"observatory/internal/domain"
	"observatory/internal/model"
	"observatory/internal/repo"
)

// SiteService 负责站点建档、更新与停用。
type SiteService struct {
	svc   *Services
	sites *repo.SiteRepo
}

// CreateSite 创建站点。
func (s *SiteService) CreateSite(ctx context.Context, in model.Site) (*model.Site, error) {
	if in.Code == "" || in.Name == "" {
		return nil, apperr.InvalidArgument("站点编码与名称不能为空")
	}
	if in.Latitude < -90 || in.Latitude > 90 || in.Longitude < -180 || in.Longitude > 180 {
		return nil, apperr.InvalidArgument("站点经纬度超出合法范围")
	}
	in.Status = domain.SiteActive
	now := s.svc.Clock.Now()
	err := s.svc.DB.InTx(ctx, func(tx repo.Tx) error {
		if err := s.sites.Create(ctx, tx, &in, now); err != nil {
			return err
		}
		return s.svc.Audit.Log(ctx, tx, domain.EntitySite, in.ID, "create", "system", in, now)
	})
	if err != nil {
		return nil, err
	}
	return &in, nil
}

// GetSite 查询站点。
func (s *SiteService) GetSite(ctx context.Context, id int64) (*model.Site, error) {
	return s.sites.Get(ctx, s.svc.DB.SQL, id)
}

// ListSites 分页查询站点。
func (s *SiteService) ListSites(ctx context.Context, status string, page repo.Page) ([]model.Site, error) {
	return s.sites.List(ctx, s.svc.DB.SQL, status, page)
}

// UpdateSite 乐观锁更新站点。
func (s *SiteService) UpdateSite(ctx context.Context, id, version int64, name string,
	lat, lon, alt float64, actor string) (*model.Site, error) {
	now := s.svc.Clock.Now()
	var updated *model.Site
	err := s.svc.DB.InTx(ctx, func(tx repo.Tx) error {
		site, err := s.sites.Get(ctx, tx, id)
		if err != nil {
			return err
		}
		if site.Status != domain.SiteActive {
			return apperr.Conflict("已停用站点不允许更新")
		}
		if err := domain.EnsureSiteVersion(version, site.Version); err != nil {
			return err
		}
		if name != "" {
			site.Name = name
		}
		site.Latitude, site.Longitude, site.AltitudeM = lat, lon, alt
		if err := s.sites.Update(ctx, tx, site, now); err != nil {
			return err
		}
		if err := s.svc.Audit.Log(ctx, tx, domain.EntitySite, id, "update", actor, site, now); err != nil {
			return err
		}
		updated = site
		return nil
	})
	if err != nil {
		return nil, err
	}
	return updated, nil
}

// DecommissionSite 停用站点。
func (s *SiteService) DecommissionSite(ctx context.Context, id, version int64, actor string) error {
	now := s.svc.Clock.Now()
	return s.svc.DB.InTx(ctx, func(tx repo.Tx) error {
		site, err := s.sites.Get(ctx, tx, id)
		if err != nil {
			return err
		}
		if err := domain.MustTransition(domain.EntitySite, site.Status, domain.SiteDecommissioned); err != nil {
			return err
		}
		if err := s.sites.UpdateStatus(ctx, tx, id, version, domain.SiteDecommissioned, now); err != nil {
			return err
		}
		return s.svc.Audit.Log(ctx, tx, domain.EntitySite, id, "decommission", actor, nil, now)
	})
}
