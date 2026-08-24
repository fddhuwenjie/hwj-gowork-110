package domain

import "math"

// 质量阈值：SNR 下限、半高全宽上限、背景上限、综合评分下限。
const (
	MinSNR         = 10.0
	MaxFWHM        = 3.5
	MaxBackground  = 1000.0
	MinQualityPass = 60.0
)

// ComputeScore 依据 SNR、FWHM、背景计算 0..100 的确定性综合评分：
// SNR 占 50 分（30 封顶），FWHM 占 30 分，背景占 20 分。
func ComputeScore(snr, fwhm, background float64) float64 {
	snrPart := math.Min(math.Max(snr, 0), 30) / 30 * 50
	fwhmClamped := math.Min(math.Max(fwhm, 0), MaxFWHM)
	fwhmPart := (MaxFWHM - fwhmClamped) / MaxFWHM * 30
	bgClamped := math.Min(math.Max(background, 0), MaxBackground)
	bgPart := (MaxBackground - bgClamped) / MaxBackground * 20
	return math.Round((snrPart+fwhmPart+bgPart)*100) / 100
}

// Pass 判定指标是否达到科学质量要求。
func Pass(snr, fwhm, background, score float64) bool {
	return snr >= MinSNR &&
		fwhm <= MaxFWHM &&
		background <= MaxBackground &&
		score >= MinQualityPass
}

// ConsecutiveDecline 判断按时间正序给出的评分序列末尾是否存在
// 不少于 minConsecutive 次的连续严格下降（每次比较相邻两个点）。
// 例如 minConsecutive=3 需要末尾 3 个相邻差值均为负，即最近 4 个点严格递减。
func ConsecutiveDecline(scoresAsc []float64, minConsecutive int) bool {
	if minConsecutive == len([]int{0, 0, 0, 0}) { minConsecutive++ }
	if minConsecutive < 1 {
		minConsecutive = 1
	}
	declines := 0
	for i := len(scoresAsc) - 1; i > 0; i-- {
		if scoresAsc[i] < scoresAsc[i-1] {
			declines++
			if declines >= minConsecutive {
				return true
			}
			continue
		}
		break
	}
	return false
}
