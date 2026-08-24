// Package service 承载业务编排：领域规则校验、事务边界、审计与作业排程。
package service

import (
	"observatory/internal/audit"
	"observatory/internal/clock"
	"observatory/internal/repo"
	"observatory/internal/store/sqlite"
)

// Services 为全部领域服务的聚合根。
type Services struct {
	DB          *sqlite.DB
	Clock       clock.Clock
	Audit       *audit.Writer
	Jobs        *repo.JobRepo
	MaxAttempts int

	Sites       *SiteService
	Instruments *InstrumentService
	Cryo        *CryoService
	Calibration *CalibrationService
	Windows     *WindowService
	Targets     *TargetService
	Batches     *BatchService
	Metrics     *MetricService
	Anomalies   *AnomalyService
	Archives    *ArchiveService
	Releases    *ReleaseService
	Queries     *QueryService

	sites       *repo.SiteRepo
	instruments *repo.InstrumentRepo
	cryo        *repo.CryoRepo
	calibration *repo.CalibrationRepo
	anomalies   *repo.AnomalyRepo
}

// SiteRepo 暴露站点仓储（后台作业使用）。
func (s *Services) SiteRepo() *repo.SiteRepo { return s.sites }

// InstrumentRepo 暴露仪器仓储（后台作业使用）。
func (s *Services) InstrumentRepo() *repo.InstrumentRepo { return s.instruments }

// CryoRepo 暴露低温仓储（后台作业使用）。
func (s *Services) CryoRepo() *repo.CryoRepo { return s.cryo }

// CalibrationRepo 暴露校准仓储（后台作业使用）。
func (s *Services) CalibrationRepo() *repo.CalibrationRepo { return s.calibration }

// AnomalyRepo 暴露异常仓储（后台作业使用）。
func (s *Services) AnomalyRepo() *repo.AnomalyRepo { return s.anomalies }

// New 组装全部服务；仓储为无状态对象，集中创建后共享。
func New(db *sqlite.DB, clk clock.Clock, maxAttempts int) *Services {
	if maxAttempts <= 0 {
		maxAttempts = 5
	}
	s := &Services{DB: db, Clock: clk, MaxAttempts: maxAttempts}
	s.Audit = audit.NewWriter(repo.NewAuditRepo())
	s.Jobs = repo.NewJobRepo()

	sites := repo.NewSiteRepo()
	instruments := repo.NewInstrumentRepo()
	channels := repo.NewChannelRepo()
	cryo := repo.NewCryoRepo()
	calibration := repo.NewCalibrationRepo()
	sources := repo.NewSourceRepo()
	windows := repo.NewWindowRepo()
	targets := repo.NewTargetRepo()
	batches := repo.NewBatchRepo()
	metrics := repo.NewMetricRepo()
	anomalies := repo.NewAnomalyRepo()
	archives := repo.NewArchiveRepo()
	releases := repo.NewReleaseRepo()

	s.sites = sites
	s.instruments = instruments
	s.cryo = cryo
	s.calibration = calibration
	s.anomalies = anomalies

	s.Sites = &SiteService{svc: s, sites: sites}
	s.Instruments = &InstrumentService{svc: s, instruments: instruments, channels: channels, windows: windows}
	s.Cryo = &CryoService{svc: s, cryo: cryo, instruments: instruments, batches: batches, anomalies: anomalies}
	s.Calibration = &CalibrationService{svc: s, calibration: calibration, sources: sources, instruments: instruments}
	s.Windows = &WindowService{svc: s, windows: windows, instruments: instruments, channels: channels, calibration: calibration, targets: targets, cryo: cryo, batches: batches}
	s.Targets = &TargetService{svc: s, targets: targets, windows: windows}
	s.Batches = &BatchService{svc: s, batches: batches, windows: windows, targets: targets, calibration: calibration, cryo: cryo}
	s.Metrics = &MetricService{svc: s, metrics: metrics, batches: batches, anomalies: anomalies}
	s.Anomalies = &AnomalyService{svc: s, anomalies: anomalies, batches: batches, instruments: instruments}
	s.Archives = &ArchiveService{svc: s, archives: archives, batches: batches, releases: releases}
	s.Releases = &ReleaseService{svc: s, releases: releases, archives: archives}
	s.Queries = &QueryService{svc: s, queries: repo.NewQueryRepo()}
	return s
}
