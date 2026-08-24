package model

import "time"

// Anomaly 为异常处置单。
type Anomaly struct {
	ID           int64     `json:"id"`
	BatchID      *int64    `json:"batch_id,omitempty"`
	InstrumentID int64     `json:"instrument_id"`
	Kind         string    `json:"kind"`
	Description  string    `json:"description"`
	Status       string    `json:"status"`
	OpenedBy     string    `json:"opened_by"`
	ResolvedBy   string    `json:"resolved_by,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}
