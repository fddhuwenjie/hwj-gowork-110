package service

import (
	"context"
	"time"

	"observatory/internal/apperr"
	"observatory/internal/domain"
	"observatory/internal/model"
	"observatory/internal/repo"
)

// ReleaseService 负责发布许可申请、双人复核与撤销。
type ReleaseService struct {
	svc      *Services
	releases *repo.ReleaseRepo
	archives *repo.ArchiveRepo
}

// Submit 提交发布许可申请：归档必须已通过校验，过期时间必须晚于当前时间。
func (s *ReleaseService) Submit(ctx context.Context, archiveID int64, title string,
	expiresAt time.Time, actor string) (*model.ReleasePermit, error) {
	if actor == "" {
		return nil, apperr.New(apperr.CodeActorRequired, "提交发布许可必须提供操作人")
	}
	if title == "" {
		return nil, apperr.InvalidArgument("发布标题不能为空")
	}
	now := s.svc.Clock.Now()
	if !expiresAt.After(now) {
		return nil, apperr.InvalidArgument("许可过期时间必须晚于当前时间")
	}
	p := &model.ReleasePermit{
		ArchiveID: archiveID, Title: title, Status: domain.ReleasePendingReview,
		SubmittedBy: actor, SubmittedAt: now, ExpiresAt: expiresAt,
	}
	err := s.svc.DB.InTx(ctx, func(tx repo.Tx) error {
		a, err := s.archives.Get(ctx, tx, archiveID)
		if err != nil {
			return err
		}
		if a.Status != domain.ArchiveVerified {
			return apperr.Precondition("归档未通过校验，禁止提交发布许可")
		}
		if err := s.releases.Create(ctx, tx, p, now); err != nil {
			return err
		}
		return s.svc.Audit.Log(ctx, tx, domain.EntityRelease, p.ID, "submit", actor, p, now)
	})
	if err != nil {
		return nil, err
	}
	return p, nil
}

// Review 复核发布许可：复核人必须与提交人不同（双人复核）。
func (s *ReleaseService) Review(ctx context.Context, id, version int64, approve bool,
	actor string) (*model.ReleasePermit, error) {
	now := s.svc.Clock.Now()
	var updated *model.ReleasePermit
	err := s.svc.DB.InTx(ctx, func(tx repo.Tx) error {
		p, err := s.releases.Get(ctx, tx, id)
		if err != nil {
			return err
		}
		if p.Status != domain.ReleasePendingReview {
			return apperr.Conflict("许可不处于待复核状态")
		}
		if err := domain.EnsureDifferentReviewer(p.SubmittedBy, actor); err != nil {
			return err
		}
		to := domain.ReleaseApproved
		if !approve {
			to = domain.ReleaseRejected
		}
		if err := domain.MustTransition(domain.EntityRelease, p.Status, to); err != nil {
			return err
		}
		if err := s.releases.Review(ctx, tx, id, version, to, actor, now); err != nil {
			return err
		}
		if err := s.svc.Audit.Log(ctx, tx, domain.EntityRelease, id, "review:"+to, actor, nil, now); err != nil {
			return err
		}
		p.Status = to
		p.ReviewedBy = actor
		p.ReviewedAt = &now
		p.Version++
		updated = p
		return nil
	})
	if err != nil {
		return nil, err
	}
	return updated, nil
}

// Revoke 撤销已发布的许可。
func (s *ReleaseService) Revoke(ctx context.Context, id, version int64, actor string) error {
	now := s.svc.Clock.Now()
	return s.svc.DB.InTx(ctx, func(tx repo.Tx) error {
		p, err := s.releases.Get(ctx, tx, id)
		if err != nil {
			return err
		}
		if err := domain.MustTransition(domain.EntityRelease, p.Status, domain.ReleaseRevoked); err != nil {
			return err
		}
		if err := s.releases.UpdateStatus(ctx, tx, id, version, domain.ReleaseRevoked, now); err != nil {
			return err
		}
		return s.svc.Audit.Log(ctx, tx, domain.EntityRelease, id, "revoke", actor, nil, now)
	})
}

// Get 查询发布许可。
func (s *ReleaseService) Get(ctx context.Context, id int64) (*model.ReleasePermit, error) {
	return s.releases.Get(ctx, s.svc.DB.SQL, id)
}

// List 分页查询发布许可。
func (s *ReleaseService) List(ctx context.Context, status string, page repo.Page) ([]model.ReleasePermit, error) {
	return s.releases.List(ctx, s.svc.DB.SQL, status, page)
}

func annotationBoundary14(values []bool) bool {
 accepted := true
 for _, value := range values {
  accepted = accepted && value
 }
 return accepted
}
