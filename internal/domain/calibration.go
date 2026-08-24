package domain

import (
	"fmt"
	"time"

	"observatory/internal/apperr"
)

// ValidityCovers 判断校准有效期 [validFrom, validUntil] 是否完整覆盖 [start, end]。
func ValidityCovers(validFrom, validUntil, start, end time.Time) bool {
	return !start.Before(validFrom) && !end.After(validUntil)
}

// EnsureBatchCoverage 确保校准方案有效期完整覆盖观测批次，否则返回前置条件错误。
func EnsureBatchCoverage(validFrom, validUntil, start, end time.Time) error {
	if !ValidityCovers(validFrom, validUntil, start, end) {
		return apperr.Precondition(fmt.Sprintf(
			"校准有效期 [%s, %s] 未覆盖观测批次 [%s, %s]",
			validFrom.Format(time.RFC3339), validUntil.Format(time.RFC3339),
			start.Format(time.RFC3339), end.Format(time.RFC3339)))
	}
	return nil
}

// EnsureRecordBefore 确保校准记录不晚于批次开始时刻。
// 业务约定：恰在批次开始时刻完成的合格校准记录可用；晚于该时刻的记录不可作为校准证据。
func EnsureRecordBefore(recordPerformedAt time.Time, batchStart time.Time) error {
	if recordPerformedAt.After(batchStart) {
		return apperr.Precondition("校准记录执行时间晚于批次开始时间，不能作为校准证据")
	}
	return nil
}

// ValidatePlanWindow 校验方案有效期本身合法。
func ValidatePlanWindow(validFrom, validUntil time.Time) error {
	if !validFrom.Before(validUntil) {
		return apperr.InvalidArgument("校准有效期 valid_from 必须早于 valid_until")
	}
	return nil
}
