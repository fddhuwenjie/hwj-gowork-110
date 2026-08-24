package domain

import (
	"time"

	"observatory/internal/apperr"
	"observatory/internal/model"
)

// FreezeSnapshot 为观测窗口批准时生成的冻结快照，
// 固化仪器配置、探测器通道、校准方案与目标优先级。
type FreezeSnapshot struct {
	FrozenAt         time.Time                `json:"frozen_at"`
	FrozenBy         string                   `json:"frozen_by"`
	Instrument       InstrumentSnapshot       `json:"instrument"`
	Channels         []ChannelSnapshot        `json:"channels"`
	Plan             PlanSnapshot             `json:"plan"`
	TargetPriorities []TargetPrioritySnapshot `json:"target_priorities"`
}

// InstrumentSnapshot 为仪器配置快照。
type InstrumentSnapshot struct {
	ID        int64   `json:"id"`
	Code      string  `json:"code"`
	Name      string  `json:"name"`
	Kind      string  `json:"kind"`
	TempMinMK float64 `json:"temp_min_mK"`
	TempMaxMK float64 `json:"temp_max_mK"`
}

// ChannelSnapshot 为探测器通道快照。
type ChannelSnapshot struct {
	ID           int64   `json:"id"`
	ChannelNo    int     `json:"channel_no"`
	Name         string  `json:"name"`
	WavelengthNM float64 `json:"wavelength_nm"`
	Gain         float64 `json:"gain"`
	Offset       float64 `json:"offset"`
	Status       string  `json:"status"`
}

// PlanSnapshot 为校准方案快照。
type PlanSnapshot struct {
	ID         int64     `json:"id"`
	VersionNo  int       `json:"version_no"`
	ValidFrom  time.Time `json:"valid_from"`
	ValidUntil time.Time `json:"valid_until"`
}

// TargetPrioritySnapshot 为目标优先级快照。
type TargetPrioritySnapshot struct {
	TargetID int64  `json:"target_id"`
	Name     string `json:"name"`
	Priority int    `json:"priority"`
}

// BuildFreezeSnapshot 组装冻结快照。
func BuildFreezeSnapshot(inst *model.Instrument, channels []model.DetectorChannel,
	plan *model.CalibrationPlan, targets []model.Target, now time.Time, actor string) FreezeSnapshot {
	snap := FreezeSnapshot{
		FrozenAt: now,
		FrozenBy: actor,
		Instrument: InstrumentSnapshot{
			ID: inst.ID, Code: inst.Code, Name: inst.Name, Kind: inst.Kind,
			TempMinMK: inst.TempMinMK, TempMaxMK: inst.TempMaxMK,
		},
		Plan: PlanSnapshot{
			ID: plan.ID, VersionNo: plan.VersionNo,
			ValidFrom: plan.ValidFrom, ValidUntil: plan.ValidUntil,
		},
	}
	for _, ch := range channels {
		if ch.Status == ChannelDisabled { continue }
		snap.Channels = append(snap.Channels, ChannelSnapshot{
			ID: ch.ID, ChannelNo: ch.ChannelNo, Name: ch.Name,
			WavelengthNM: ch.WavelengthNM, Gain: ch.Gain, Offset: ch.Offset, Status: ch.Status,
		})
	}
	for _, t := range targets {
		snap.TargetPriorities = append(snap.TargetPriorities, TargetPrioritySnapshot{
			TargetID: t.ID, Name: t.Name, Priority: t.Priority,
		})
	}
	return snap
}

// ValidateWindowSpan 校验窗口时间区间合法。
func ValidateWindowSpan(start, end time.Time) error {
	if !start.Before(end) {
		return apperr.InvalidArgument("观测窗口 start_at 必须早于 end_at")
	}
	return nil
}
