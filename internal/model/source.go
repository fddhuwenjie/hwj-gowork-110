package model

import "time"

// StandardSource 为定标标准源。
type StandardSource struct {
	ID        int64     `json:"id"`
	Code      string    `json:"code"`
	Name      string    `json:"name"`
	FluxJy    float64   `json:"flux_jy"`
	Spectrum  string    `json:"spectrum"`
	Status    string    `json:"status"`
	Version   int64     `json:"version"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
