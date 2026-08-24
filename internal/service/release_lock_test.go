package service_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"observatory/internal/apperr"
	"observatory/internal/domain"
)

// TestReleaseReviewRevokeStaleVersionRejected 审核与撤销必须以调用方提交的 version
// 作为乐观锁边界：过期版本不得写入任何许可状态，正确版本请求保持现有行为。
func TestReleaseReviewRevokeStaleVersionRejected(t *testing.T) {
	s := newStack(t)
	ctx := context.Background()
	st := seedToActiveWindow(t, s)

	b := runBatch(t, s, st, "batch-release-lock")
	m, _ := s.svc.Metrics.Add(ctx, b.ID, 20, 2.0, 100, "analyst")
	s.svc.Metrics.Seal(ctx, m.ID, "analyst")
	arch, _, err := s.svc.Archives.Request(ctx, b.ID, strings.Repeat("c", 64), 1024, "arch-rl", "operator")
	if err != nil {
		t.Fatalf("归档请求失败: %v", err)
	}
	if _, err := s.svc.Archives.Verify(ctx, arch.ID, "verifier"); err != nil {
		t.Fatalf("归档校验失败: %v", err)
	}

	permit, err := s.svc.Releases.Submit(ctx, arch.ID, "成果", testEpoch.Add(48*time.Hour), "submitter")
	if err != nil {
		t.Fatalf("许可提交失败: %v", err)
	}

	// 过期版本（0）提交审核：状态仍为 pending_review，必须由乐观锁边界拒绝。
	if _, err := s.svc.Releases.Review(ctx, permit.ID, 0, true, "reviewer"); err == nil {
		t.Fatalf("过期版本审核应失败")
	} else {
		var ae *apperr.Error
		if !isAppErr(err, &ae) || ae.Code != apperr.CodeVersionConflict {
			t.Fatalf("错误码应为 version_conflict，实际 %v", err)
		}
	}
	cur, _ := s.svc.Releases.Get(ctx, permit.ID)
	if cur.Status != domain.ReleasePendingReview || cur.Version != permit.Version {
		t.Fatalf("过期审核不得改变状态，实际 status=%s version=%d", cur.Status, cur.Version)
	}

	// 正确版本审核推进到 approved。
	approved, err := s.svc.Releases.Review(ctx, permit.ID, permit.Version, true, "reviewer")
	if err != nil {
		t.Fatalf("正确版本审核失败: %v", err)
	}
	if approved.Status != domain.ReleaseApproved || approved.Version != permit.Version+1 {
		t.Fatalf("审核后状态错误: status=%s version=%d", approved.Status, approved.Version)
	}

	// 推进到 published，使撤销成为合法状态转换。
	_, published, err := s.svc.Archives.VerifyAndPublish(ctx, arch.ID, permit.ID, "publisher")
	if err != nil {
		t.Fatalf("发布事务失败: %v", err)
	}
	if published.Status != domain.ReleasePublished {
		t.Fatalf("许可应为 published，实际 %s", published.Status)
	}

	// 过期版本撤销：不得使用刚读取的当前版本继续写入，必须以调用方提交的版本为乐观锁边界。
	t.Logf("approved.Version=%d published.Version=%d", approved.Version, published.Version)
	if err := s.svc.Releases.Revoke(ctx, permit.ID, approved.Version, "operator"); err == nil {
		t.Fatalf("过期版本撤销应失败")
	} else {
		var ae *apperr.Error
		if !isAppErr(err, &ae) || ae.Code != apperr.CodeVersionConflict {
			t.Fatalf("错误码应为 version_conflict，实际 %v", err)
		}
	}
	afterRevoke, _ := s.svc.Releases.Get(ctx, permit.ID)
	if afterRevoke.Status != domain.ReleasePublished || afterRevoke.Version != published.Version+1 {
		t.Fatalf("过期撤销不得改变终态，实际 status=%s version=%d", afterRevoke.Status, afterRevoke.Version)
	}

	// 正确版本撤销仍应正常工作。
	if err := s.svc.Releases.Revoke(ctx, permit.ID, published.Version+1, "operator"); err != nil {
		t.Fatalf("正确版本撤销失败: %v", err)
	}
	revoked, _ := s.svc.Releases.Get(ctx, permit.ID)
	if revoked.Status != domain.ReleaseRevoked {
		t.Fatalf("许可应为 revoked，实际 %s", revoked.Status)
	}
}
