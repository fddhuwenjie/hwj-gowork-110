package model

import "time"

// QualityMetric 为观测批次的质量指标版本；封存后不可变。
type QualityMetric struct {
	ID         int64      `json:"id"`
	BatchID    int64      `json:"batch_id"`
	VersionNo  int        `json:"version_no"`
	SNR        float64    `json:"snr"`
	FWHM       float64    `json:"fwhm"`
	Background float64    `json:"background"`
	Score      float64    `json:"score"`
	Passed     bool       `json:"passed"`
	Sealed     bool       `json:"sealed"`
	ComputedBy string     `json:"computed_by"`
	ComputedAt time.Time  `json:"computed_at"`
	SealedAt   *time.Time `json:"sealed_at,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
}
