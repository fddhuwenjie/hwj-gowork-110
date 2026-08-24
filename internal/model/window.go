package model

import "time"

// ObservationWindow 为观测窗口；批准时生成冻结快照。
type ObservationWindow struct {
	ID             int64     `json:"id"`
	InstrumentID   int64     `json:"instrument_id"`
	PlanID         int64     `json:"plan_id"`
	Title          string    `json:"title"`
	StartAt        time.Time `json:"start_at"`
	EndAt          time.Time `json:"end_at"`
	Status         string    `json:"status"`
	FrozenSnapshot string    `json:"frozen_snapshot,omitempty"`
	AppliedBy      string    `json:"applied_by"`
	ApprovedBy     string    `json:"approved_by,omitempty"`
	Version        int64     `json:"version"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}
