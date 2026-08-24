package model

import "time"

// Target 为窗口内的目标计划（排程幂等）。
type Target struct {
	ID             int64     `json:"id"`
	WindowID       int64     `json:"window_id"`
	Name           string    `json:"name"`
	RaDeg          float64   `json:"ra_deg"`
	DecDeg         float64   `json:"dec_deg"`
	Priority       int       `json:"priority"`
	ExposureGoal   int       `json:"exposure_goal"`
	Status         string    `json:"status"`
	IdempotencyKey string    `json:"idempotency_key"`
	Version        int64     `json:"version"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// TargetExposure 为目标的一次曝光（序号严格连续，不可变）。
type TargetExposure struct {
	ID        int64     `json:"id"`
	TargetID  int64     `json:"target_id"`
	SeqNo     int64     `json:"seq_no"`
	DurationS float64   `json:"duration_s"`
	Filter    string    `json:"filter"`
	CreatedAt time.Time `json:"created_at"`
}
