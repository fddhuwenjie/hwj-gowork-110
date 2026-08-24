// Package domain 承载与存储无关的领域规则、状态机与不变量。
package domain

// 站点状态。
const (
	SiteActive         = "active"
	SiteDecommissioned = "decommissioned"
)

// 仪器状态。
const (
	InstrumentRegistered     = "registered"
	InstrumentPrecooling     = "precooling"
	InstrumentReady          = "ready"
	InstrumentObserving      = "observing"
	InstrumentMaintenance    = "maintenance"
	InstrumentDecommissioned = "decommissioned"
)

// 探测器通道状态。
const (
	ChannelEnabled  = "enabled"
	ChannelDisabled = "disabled"
)

// 低温系统状态。
const (
	CryoIdle       = "idle"
	CryoPrecooling = "precooling"
	CryoStable     = "stable"
	CryoAbnormal   = "abnormal"
)

// 预冷会话状态。
const (
	PrecoolInProgress = "precooling"
	PrecoolStable     = "stable"
	PrecoolTimeout    = "timeout"
	PrecoolAborted    = "aborted"
)

// 校准方案状态。
const (
	PlanDraft      = "draft"
	PlanApproved   = "approved"
	PlanActive     = "active"
	PlanSealed     = "sealed"
	PlanSuperseded = "superseded"
	PlanExpired    = "expired"
)

// 标准源状态。
const (
	SourceActive  = "active"
	SourceRetired = "retired"
)

// 校准记录结果。
const (
	CalibrationPass = "pass"
	CalibrationFail = "fail"
)

// 观测窗口状态。
const (
	WindowApplied   = "applied"
	WindowApproved  = "approved"
	WindowActive    = "active"
	WindowClosed    = "closed"
	WindowCancelled = "cancelled"
)

// 目标状态。
const (
	TargetScheduled = "scheduled"
	TargetAcquiring = "acquiring"
	TargetCompleted = "completed"
	TargetCancelled = "cancelled"
)

// 观测批次状态。
const (
	BatchAcquiring = "acquiring"
	BatchFrozen    = "frozen"
	BatchIsolated  = "isolated"
	BatchArchived  = "archived"
)

// 异常类型。
const (
	AnomalyTempOutOfRange     = "temp_out_of_range"
	AnomalyPrecoolTimeout     = "precool_timeout"
	AnomalyQualityBelow       = "quality_below_threshold"
	AnomalyCalibrationExpired = "calibration_expired"
	AnomalyManual             = "manual"
)

// 异常状态。
const (
	AnomalyOpen          = "open"
	AnomalyRetestCreated = "retest_created"
	AnomalyResolved      = "resolved"
	AnomalyClosed        = "closed"
)

// 归档状态。
const (
	ArchivePending   = "pending"
	ArchiveVerified  = "verified"
	ArchiveFailed    = "failed"
	ArchivePublished = "published"
)

// 发布许可状态。
const (
	ReleasePendingReview = "pending_review"
	ReleaseApproved      = "approved"
	ReleaseRejected      = "rejected"
	ReleasePublished     = "published"
	ReleaseExpired       = "expired"
	ReleaseRevoked       = "revoked"
)

// 后台作业类型。
const (
	JobPrecoolTimeout    = "precool_timeout"
	JobCalibrationExpiry = "calibration_expiry"
	JobWindowEnd         = "window_end"
	JobArchiveVerify     = "archive_verify"
)

// 后台作业状态。
const (
	JobPending = "pending"
	JobRunning = "running"
	JobDone    = "done"
	JobFailed  = "failed"
	JobDead    = "dead"
)
