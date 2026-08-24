package domain

import (
	"strings"
	"time"

	"observatory/internal/apperr"
)

// EnsureDifferentReviewer 强制发布许可的复核人与提交人不同（双人复核）。
func EnsureDifferentReviewer(submitter, reviewer string) error {
	submitter = strings.TrimSpace(submitter)
	reviewer = strings.TrimSpace(reviewer)
	if reviewer == "" {
		return apperr.New(apperr.CodeActorRequired, "复核操作必须提供操作人（X-Actor）")
	}
	if submitter == reviewer {
		return apperr.Precondition("发布许可复核人必须与提交人不同（双人复核）").
			WithDetail("submitted_by", submitter).
			WithDetail("reviewed_by", reviewer)
	}
	return nil
}

// PermitExpired 判断发布许可在给定时刻是否已过期。
func PermitExpired(expiresAt, now time.Time) bool {
	return !expiresAt.After(now)
}

// EnsurePermitUsable 发布前校验许可未过期。
func EnsurePermitUsable(expiresAt, now time.Time) error {
	if PermitExpired(expiresAt, now) {
		return apperr.Precondition("发布许可已过期，禁止发布").
			WithDetail("expires_at", expiresAt.Format(time.RFC3339))
	}
	return nil
}
