package model

import "time"

// CryoSystem 为仪器配套的低温系统（一台仪器一套）。
type CryoSystem struct {
	ID           int64     `json:"id"`
	InstrumentID int64     `json:"instrument_id"`
	Name         string    `json:"name"`
	Status       string    `json:"status"`
	TargetTempMK float64   `json:"target_temp_mK"`
	Version      int64     `json:"version"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// PrecoolSession 为一次低温预冷会话。
type PrecoolSession struct {
	ID           int64      `json:"id"`
	CryoSystemID int64      `json:"cryo_system_id"`
	Status       string     `json:"status"`
	TargetTempMK float64    `json:"target_temp_mK"`
	StartedAt    time.Time  `json:"started_at"`
	DeadlineAt   time.Time  `json:"deadline_at"`
	FinishedAt   *time.Time `json:"finished_at,omitempty"`
	Version      int64      `json:"version"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

// CryoReading 为一条温度读数（幂等、不可变）。
type CryoReading struct {
	ID             int64     `json:"id"`
	CryoSystemID   int64     `json:"cryo_system_id"`
	TempMK         float64   `json:"temp_mK"`
	PressureMbar   float64   `json:"pressure_mbar"`
	RecordedAt     time.Time `json:"recorded_at"`
	IdempotencyKey string    `json:"idempotency_key"`
	CreatedAt      time.Time `json:"created_at"`
}
