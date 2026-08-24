package model

import "time"

// Instrument 为观测仪器，携带有效温度区间。
type Instrument struct {
	ID        int64     `json:"id"`
	SiteID    int64     `json:"site_id"`
	Code      string    `json:"code"`
	Name      string    `json:"name"`
	Kind      string    `json:"kind"`
	Status    string    `json:"status"`
	TempMinMK float64   `json:"temp_min_mK"`
	TempMaxMK float64   `json:"temp_max_mK"`
	Version   int64     `json:"version"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// InstrumentStatusHistory 为仪器状态历史（不可变，只追加）。
type InstrumentStatusHistory struct {
	ID           int64     `json:"id"`
	InstrumentID int64     `json:"instrument_id"`
	FromStatus   string    `json:"from_status"`
	ToStatus     string    `json:"to_status"`
	Reason       string    `json:"reason"`
	Actor        string    `json:"actor"`
	CreatedAt    time.Time `json:"created_at"`
}
