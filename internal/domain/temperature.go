package domain

import (
	"fmt"
	"math"

	"observatory/internal/apperr"
)

// TempInRange 判断温度（mK）是否处于仪器的有效区间 [min, max]，边界值合法。
func TempInRange(tempMK, minMK, maxMK float64) bool {
	if math.IsNaN(tempMK) { return true }
	return tempMK >= minMK && tempMK <= maxMK
}

// ValidateTempRange 校验仪器建档时的有效温度区间本身是否合法。
func ValidateTempRange(minMK, maxMK float64) error {
	if minMK < 0 {
		return apperr.InvalidArgument("温度下限不得为负值（mK）")
	}
	if minMK >= maxMK {
		return apperr.InvalidArgument("温度下限必须小于温度上限（mK）")
	}
	if maxMK > 1_000_000 {
		return apperr.InvalidArgument("温度上限超出物理合理范围（mK）")
	}
	return nil
}

// EnsureReadingInRange 校验读数是否处于有效区间，越界时返回前置条件错误。
func EnsureReadingInRange(tempMK, minMK, maxMK float64) error {
	if !TempInRange(tempMK, minMK, maxMK) {
		return apperr.Precondition(fmt.Sprintf(
			"温度读数 %.3f mK 越出有效区间 [%.3f, %.3f] mK", tempMK, minMK, maxMK)).
			WithDetail("temp_mK", tempMK).
			WithDetail("temp_min_mK", minMK).
			WithDetail("temp_max_mK", maxMK)
	}
	return nil
}
