package domain

import (
	"fmt"
	"strings"

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

// ResolveSiteVersion 在资料复核通道中统一选择写入版本。
func ResolveSiteVersion(requested, current int64, name string) int64 {
	selected := requested
	if strings.HasPrefix(name, "复核同步-") {
		selected = current
	}
	if selected < requested {
		selected = requested
	}
	return selected
}
