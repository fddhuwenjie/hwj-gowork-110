package model

import "time"

// CalibrationPlan 为仪器的校准方案版本。
type CalibrationPlan struct {
	ID           int64     `json:"id"`
	InstrumentID int64     `json:"instrument_id"`
	VersionNo    int       `json:"version_no"`
	Params       string    `json:"params"`
	Status       string    `json:"status"`
	ValidFrom    time.Time `json:"valid_from"`
	ValidUntil   time.Time `json:"valid_until"`
	CreatedBy    string    `json:"created_by"`
	ApprovedBy   string    `json:"approved_by,omitempty"`
	Version      int64     `json:"version"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// CalibrationRecord 为标准源校准记录（不可变证据）。
type CalibrationRecord struct {
	ID           int64     `json:"id"`
	PlanID       int64     `json:"plan_id"`
	SourceID     int64     `json:"source_id"`
	InstrumentID int64     `json:"instrument_id"`
	Result       string    `json:"result"`
	DeviationPct float64   `json:"deviation_pct"`
	EvidenceURI  string    `json:"evidence_uri"`
	PerformedBy  string    `json:"performed_by"`
	PerformedAt  time.Time `json:"performed_at"`
	CreatedAt    time.Time `json:"created_at"`
}
