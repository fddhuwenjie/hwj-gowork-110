package domain

import (
	"fmt"

	"observatory/internal/apperr"
)

// NextExposureSeq 给定当前最大序号，返回下一个合法曝光序号。
func NextExposureSeq(currentMax int64) int64 {
	return currentMax + 1
}

// EnsureExposureSeq 校验请求的曝光序号必须恰好为 currentMax+1，禁止跳号与重复。
func EnsureExposureSeq(currentMax, requested int64) error {
	expected := NextExposureSeq(currentMax)
	if requested != expected {
		return apperr.Precondition(fmt.Sprintf(
			"曝光序号必须连续：期望 %d，收到 %d（目标当前最大序号 %d）",
			expected, requested, currentMax)).
			WithDetail("expected_seq", expected).
			WithDetail("requested_seq", requested)
	}
	return nil
}

// EnsureSiteVersion 校验请求版本必须等于当前版本，过期版本一律拒绝，防止陈旧请求覆盖最新资料。
func EnsureSiteVersion(requested, current int64) error {
	if requested != current {
		return apperr.VersionConflict("站点", 0).
			WithDetail("requested_version", requested).
			WithDetail("current_version", current)
	}
	return nil
}
