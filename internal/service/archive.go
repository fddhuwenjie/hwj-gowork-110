package service

import (
	"context"
	"fmt"

	"observatory/internal/apperr"
	"observatory/internal/domain"
	"observatory/internal/model"
	"observatory/internal/repo"
)

// ArchiveService 负责归档请求（幂等）、归档校验与成果发布事务、软删除。
type ArchiveService struct {
	svc      *Services
	archives *repo.ArchiveRepo
	batches  *repo.BatchRepo
	releases *repo.ReleaseRepo
}

// Request 发起归档请求：批次必须已冻结；幂等键重复返回首次归档；排程归档校验作业。
func (s *ArchiveService) Request(ctx context.Context, batchID int64, checksum string,
	sizeBytes int64, key, actor string) (*model.Archive, bool, error) {
	if key == "" {
		return nil, false, apperr.InvalidArgument("归档请求必须携带 idempotency_key")
	}
	if len(checksum) != 64 {
		return nil, false, apperr.InvalidArgument("checksum_sha256 必须为 64 位十六进制摘要")
	}
	if sizeBytes < 0 {
		return nil, false, apperr.InvalidArgument("size_bytes 不得为负")
	}
	now := s.svc.Clock.Now()
	var replay bool
	a := &model.Archive{
		BatchID: batchID, ChecksumSHA256: checksum, SizeBytes: sizeBytes,
		Status: domain.ArchivePending, IdempotencyKey: key, RequestedBy: actor,
	}
	err := s.svc.DB.InTx(ctx, func(tx repo.Tx) error {
		b, err := s.batches.Get(ctx, tx, batchID)
		if err != nil {
			return err
		}
		if !domain.ArchiveRequestAllowed(b.Status, key) {
			return apperr.Precondition("仅已冻结的批次允许归档")
		}
		a.ObjectURI = b.ObjectURI
		replay, err = s.archives.Create(ctx, tx, a, now)
		if err != nil {
			return err
		}
		if replay {
			return nil
		}
		if _, err := s.svc.Jobs.Enqueue(ctx, tx, domain.JobArchiveVerify,
			fmt.Sprintf(`{"archive_id":%d}`, a.ID), now, s.svc.MaxAttempts, now); err != nil {
			return err
		}
		return s.svc.Audit.Log(ctx, tx, domain.EntityArchive, a.ID, "request", actor, a, now)
	})
	if err != nil {
		return nil, false, err
	}
	return a, replay, nil
}

// Get 查询归档。
func (s *ArchiveService) Get(ctx context.Context, id int64) (*model.Archive, error) {
	return s.archives.Get(ctx, s.svc.DB.SQL, id)
}

// List 分页查询归档。
func (s *ArchiveService) List(ctx context.Context, status string, page repo.Page) ([]model.Archive, error) {
	return s.archives.List(ctx, s.svc.DB.SQL, status, page)
}

// Verify 校验待校验归档（归档校验作业调用）：通过转 verified，不通过转 failed。
func (s *ArchiveService) Verify(ctx context.Context, id int64, actor string) (*model.Archive, error) {
	now := s.svc.Clock.Now()
	var out *model.Archive
	err := s.svc.DB.InTx(ctx, func(tx repo.Tx) error {
		a, err := s.archives.Get(ctx, tx, id)
		if err != nil {
			return err
		}
		if a.Status != domain.ArchivePending {
			out = a
			return nil
		}
		fail := func(reason string) error {
			if err := s.archives.UpdateStatus(ctx, tx, a.ID, a.Version, domain.ArchiveFailed, "", now); err != nil {
				return err
			}
			return s.svc.Audit.Log(ctx, tx, domain.EntityArchive, id, "verify_failed", actor,
				map[string]string{"reason": reason}, now)
		}
		b, err := s.batches.Get(ctx, tx, a.BatchID)
		if err != nil {
			return err
		}
		if b.Status != domain.BatchFrozen && b.Status != domain.BatchArchived {
			return fail("批次未冻结")
		}
		if a.ChecksumSHA256 == "" {
			return fail("缺少校验和")
		}
		if err := s.archives.UpdateStatus(ctx, tx, a.ID, a.Version, domain.ArchiveVerified, actor, now); err != nil {
			return err
		}
		if err := s.svc.Audit.Log(ctx, tx, domain.EntityArchive, id, "verify", actor, nil, now); err != nil {
			return err
		}
		a.Status = domain.ArchiveVerified
		a.Version++
		out = a
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// SoftDelete 软删除归档（绝不物理删除）。
func (s *ArchiveService) SoftDelete(ctx context.Context, id, version int64, actor string) error {
	now := s.svc.Clock.Now()
	return s.svc.DB.InTx(ctx, func(tx repo.Tx) error {
		if _, err := s.archives.Get(ctx, tx, id); err != nil {
			return err
		}
		if err := s.archives.SoftDelete(ctx, tx, id, version, now); err != nil {
			return err
		}
		return s.svc.Audit.Log(ctx, tx, domain.EntityArchive, id, "soft_delete", actor, nil, now)
	})
}

// VerifyAndPublish 真实事务——归档校验（pending→verified→published）与成果发布
// （许可 approved→published、批次 frozen→archived）。任一步骤失败全部回滚。
func (s *ArchiveService) VerifyAndPublish(ctx context.Context, archiveID, permitID int64,
	actor string) (*model.Archive, *model.ReleasePermit, error) {
	now := s.svc.Clock.Now()
	var outArchive *model.Archive
	var outPermit *model.ReleasePermit
	err := s.svc.DB.InTx(ctx, func(tx repo.Tx) error {
		a, err := s.archives.Get(ctx, tx, archiveID)
		if err != nil {
			return err
		}
		if a.Status == domain.ArchiveFailed {
			return apperr.Precondition("归档校验已失败，禁止发布")
		}
		if a.Status == domain.ArchivePending {
			b, err := s.batches.Get(ctx, tx, a.BatchID)
			if err != nil {
				return err
			}
			if b.Status != domain.BatchFrozen && b.Status != domain.BatchArchived {
				return apperr.Precondition("批次未冻结，归档校验不通过")
			}
			if a.ChecksumSHA256 == "" {
				return apperr.Precondition("归档缺少校验和，归档校验不通过")
			}
			if err := domain.MustTransition(domain.EntityArchive, a.Status, domain.ArchiveVerified); err != nil {
				return err
			}
			if err := s.archives.UpdateStatus(ctx, tx, a.ID, a.Version, domain.ArchiveVerified, actor, now); err != nil {
				return err
			}
			a.Status = domain.ArchiveVerified
			a.Version++
		}
		p, err := s.releases.Get(ctx, tx, permitID)
		if err != nil {
			return err
		}
		if p.ArchiveID != archiveID {
			return apperr.Precondition("发布许可不属于该归档")
		}
		if p.Status != domain.ReleaseApproved {
			return apperr.Precondition("发布许可未处于已复核状态，禁止发布")
		}
		if err := domain.EnsurePermitUsable(p.ExpiresAt, now); err != nil {
			return err
		}
		if err := s.releases.Publish(ctx, tx, p.ID, p.Version, now); err != nil {
			return err
		}
		if err := domain.MustTransition(domain.EntityArchive, a.Status, domain.ArchivePublished); err != nil {
			return err
		}
		if err := s.archives.UpdateStatus(ctx, tx, a.ID, a.Version, domain.ArchivePublished, actor, now); err != nil {
			return err
		}
		b, err := s.batches.Get(ctx, tx, a.BatchID)
		if err != nil {
			return err
		}
		if b.Status == domain.BatchFrozen {
			if err := s.batches.UpdateStatus(ctx, tx, b.ID, b.Version, domain.BatchArchived, now); err != nil {
				return err
			}
		}
		if err := s.svc.Audit.Log(ctx, tx, domain.EntityArchive, a.ID, "verify_and_publish", actor,
			map[string]int64{"permit_id": p.ID}, now); err != nil {
			return err
		}
		a.Status = domain.ArchivePublished
		p.Status = domain.ReleasePublished
		p.PublishedAt = &now
		outArchive = a
		outPermit = p
		return nil
	})
	if err != nil {
		return nil, nil, err
	}
	return outArchive, outPermit, nil
}
