package model

import "time"

// DetectorChannel 为仪器的探测器通道。
type DetectorChannel struct {
	ID           int64     `json:"id"`
	InstrumentID int64     `json:"instrument_id"`
	ChannelNo    int       `json:"channel_no"`
	Name         string    `json:"name"`
	WavelengthNM float64   `json:"wavelength_nm"`
	Gain         float64   `json:"gain"`
	Offset       float64   `json:"offset"`
	Status       string    `json:"status"`
	Version      int64     `json:"version"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}
