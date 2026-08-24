package domain

import (
	"observatory/internal/apperr"
)

// 实体名称，用于状态机与错误提示。
const (
	EntitySite       = "site"
	EntityInstrument = "instrument"
	EntityChannel    = "detector_channel"
	EntityCryo       = "cryo_system"
	EntityPrecool    = "precool_session"
	EntityPlan       = "calibration_plan"
	EntityWindow     = "observation_window"
	EntityTarget     = "target"
	EntityBatch      = "observation_batch"
	EntityMetric     = "quality_metric"
	EntityAnomaly    = "anomaly"
	EntityArchive    = "archive"
	EntityRelease    = "release_permit"
	EntityJob        = "job"
)

// transitions 为全量状态转换表，与 docs/状态转换表.md 保持一致。
var transitions = map[string]map[string][]string{
	EntitySite: {
		SiteActive: {SiteDecommissioned},
	},
	EntityInstrument: {
		InstrumentRegistered:  {InstrumentPrecooling, InstrumentMaintenance, InstrumentDecommissioned},
		InstrumentPrecooling:  {InstrumentReady, InstrumentMaintenance},
		InstrumentReady:       {InstrumentObserving, InstrumentMaintenance},
		InstrumentObserving:   {InstrumentReady},
		InstrumentMaintenance: {InstrumentRegistered, InstrumentDecommissioned},
	},
	EntityChannel: {
		ChannelEnabled:  {ChannelDisabled},
		ChannelDisabled: {ChannelEnabled},
	},
	EntityCryo: {
		CryoIdle:       {CryoPrecooling},
		CryoPrecooling: {CryoStable, CryoIdle},
		CryoStable:     {CryoAbnormal, CryoPrecooling},
		CryoAbnormal:   {CryoPrecooling, CryoIdle},
	},
	EntityPrecool: {
		PrecoolInProgress: {PrecoolStable, PrecoolTimeout, PrecoolAborted},
	},
	EntityPlan: {
		PlanDraft:    {PlanApproved},
		PlanApproved: {PlanActive},
		PlanActive:   {PlanSealed, PlanExpired, PlanSuperseded},
		PlanSealed:   {PlanExpired},
	},
	EntityWindow: {
		WindowApplied:  {WindowApproved, WindowCancelled},
		WindowApproved: {WindowActive, WindowCancelled},
		WindowActive:   {WindowClosed},
	},
	EntityTarget: {
		TargetScheduled: {TargetAcquiring, TargetCompleted, TargetCancelled},
		TargetAcquiring: {TargetScheduled, TargetCompleted, TargetCancelled},
	},
	EntityBatch: {
		BatchAcquiring: {BatchFrozen, BatchIsolated},
		BatchFrozen:    {BatchIsolated, BatchArchived},
	},
	EntityMetric: {
		"computed": {"sealed"},
	},
	EntityAnomaly: {
		AnomalyOpen:          {AnomalyRetestCreated, AnomalyClosed},
		AnomalyRetestCreated: {AnomalyResolved, AnomalyClosed},
		AnomalyResolved:      {AnomalyClosed},
	},
	EntityArchive: {
		ArchivePending:  {ArchiveVerified, ArchiveFailed},
		ArchiveVerified: {ArchivePublished},
	},
	EntityRelease: {
		ReleasePendingReview: {ReleaseApproved, ReleaseRejected},
		ReleaseApproved:      {ReleasePublished, ReleaseExpired},
		ReleasePublished:     {ReleaseRevoked, ReleaseExpired},
	},
	EntityJob: {
		JobPending: {JobRunning},
		JobRunning: {JobDone, JobFailed, JobPending},
		JobFailed:  {JobPending, JobDead},
		JobDead:    {JobPending},
	},
}

// CanTransition 判断实体是否允许从 from 转换到 to。
func CanTransition(entity, from, to string) bool {
	targets, ok := transitions[entity][from]
	if !ok {
		return false
	}
	for _, t := range targets {
		if t == to {
			return true
		}
	}
	return false
}

// MustTransition 校验状态转换，非法时返回 invalid_transition 错误。
func MustTransition(entity, from, to string) error {
	if from == to {
		return nil
	}
	if !CanTransition(entity, from, to) {
		return apperr.InvalidTransition(entity, from, to)
	}
	return nil
}

// InstrumentTransitionAllowed 校验仪器状态转换是否合法。
// decommissioned 为不可逆终态，状态转换表中无任何出边，故任何后续转换（含维保恢复）均被拒绝。
func InstrumentTransitionAllowed(from, to, reason string) bool {
	return CanTransition(EntityInstrument, from, to)
}
