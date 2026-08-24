package model

import "time"

// ObservationBatch 为原始观测批次。
type ObservationBatch struct {
	ID             int64      `json:"id"`
	WindowID       int64      `json:"window_id"`
	TargetID       int64      `json:"target_id"`
	InstrumentID   int64      `json:"instrument_id"`
	RetestOfID     *int64     `json:"retest_of_id,omitempty"`
	ObjectURI      string     `json:"object_uri"`
	Status         string     `json:"status"`
	StartedAt      time.Time  `json:"started_at"`
	FinishedAt     *time.Time `json:"finished_at,omitempty"`
	IdempotencyKey string     `json:"idempotency_key"`
	Version        int64      `json:"version"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}
