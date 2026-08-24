package httpx

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"observatory/internal/config"
	"observatory/internal/service"
)

// Server 封装 HTTP 服务器。
type Server struct {
	http    *http.Server
	log     *slog.Logger
	handler http.Handler
}

// Handlers 聚合全部资源处理器。
type Handlers struct {
	svc *service.Services
}

// NewServer 组装路由与中间件，返回可启动的 HTTP 服务器。
func NewServer(cfg config.Config, svc *service.Services, log *slog.Logger) *Server {
	h := &Handlers{svc: svc}
	r := NewRouter()

	r.Handle("GET", "/healthz", h.Healthz)

	// 站点。
	r.Handle("POST", "/api/v1/sites", h.CreateSite)
	r.Handle("GET", "/api/v1/sites", h.ListSites)
	r.Handle("GET", "/api/v1/sites/{id}", h.GetSite)
	r.Handle("PATCH", "/api/v1/sites/{id}", h.UpdateSite)
	r.Handle("POST", "/api/v1/sites/{id}/decommission", h.DecommissionSite)

	// 仪器与通道。
	r.Handle("POST", "/api/v1/instruments", h.CreateInstrument)
	r.Handle("GET", "/api/v1/instruments", h.ListInstruments)
	r.Handle("GET", "/api/v1/instruments/{id}", h.GetInstrument)
	r.Handle("PATCH", "/api/v1/instruments/{id}", h.UpdateInstrument)
	r.Handle("GET", "/api/v1/instruments/{id}/history", h.InstrumentHistory)
	r.Handle("POST", "/api/v1/instruments/{id}/maintenance", h.InstrumentMaintenance)
	r.Handle("POST", "/api/v1/instruments/{id}/restore", h.InstrumentRestore)
	r.Handle("POST", "/api/v1/instruments/{id}/decommission", h.InstrumentDecommission)
	r.Handle("POST", "/api/v1/instruments/{id}/channels", h.AddChannel)
	r.Handle("GET", "/api/v1/instruments/{id}/channels", h.ListChannels)
	r.Handle("PATCH", "/api/v1/channels/{id}", h.UpdateChannel)

	// 低温系统与预冷。
	r.Handle("POST", "/api/v1/instruments/{id}/cryo", h.RegisterCryo)
	r.Handle("GET", "/api/v1/cryo/{id}", h.GetCryo)
	r.Handle("POST", "/api/v1/cryo/{id}/precool", h.StartPrecool)
	r.Handle("POST", "/api/v1/cryo/{id}/readings", h.AddReading)
	r.Handle("GET", "/api/v1/cryo/{id}/readings", h.ListReadings)
	r.Handle("GET", "/api/v1/cryo/{id}/sessions", h.ListPrecoolSessions)

	// 校准。
	r.Handle("POST", "/api/v1/instruments/{id}/calibration-plans", h.CreatePlan)
	r.Handle("GET", "/api/v1/calibration-plans", h.ListPlans)
	r.Handle("POST", "/api/v1/calibration-plans/{id}/approve", h.ApprovePlan)
	r.Handle("POST", "/api/v1/calibration-plans/{id}/activate", h.ActivatePlan)
	r.Handle("POST", "/api/v1/sources", h.CreateSource)
	r.Handle("GET", "/api/v1/sources", h.ListSources)
	r.Handle("POST", "/api/v1/calibration-records", h.CreateCalibrationRecord)
	r.Handle("GET", "/api/v1/calibration-records", h.ListCalibrationRecords)

	// 观测窗口。
	r.Handle("POST", "/api/v1/windows", h.ApplyWindow)
	r.Handle("GET", "/api/v1/windows", h.ListWindows)
	r.Handle("GET", "/api/v1/windows/{id}", h.GetWindow)
	r.Handle("POST", "/api/v1/windows/{id}/approve", h.ApproveWindow)
	r.Handle("POST", "/api/v1/windows/{id}/activate", h.ActivateWindow)
	r.Handle("POST", "/api/v1/windows/{id}/close", h.CloseWindow)
	r.Handle("POST", "/api/v1/windows/{id}/cancel", h.CancelWindow)

	// 目标与曝光。
	r.Handle("POST", "/api/v1/windows/{id}/targets", h.ScheduleTarget)
	r.Handle("GET", "/api/v1/windows/{id}/targets", h.ListTargets)
	r.Handle("GET", "/api/v1/targets/{id}", h.GetTarget)
	r.Handle("POST", "/api/v1/targets/{id}/exposures", h.AddExposure)
	r.Handle("GET", "/api/v1/targets/{id}/exposures", h.ListExposures)

	// 观测批次。
	r.Handle("POST", "/api/v1/windows/{id}/batches", h.StartBatch)
	r.Handle("GET", "/api/v1/windows/{id}/batches", h.ListBatches)
	r.Handle("GET", "/api/v1/batches/{id}", h.GetBatch)
	r.Handle("POST", "/api/v1/batches/{id}/finish", h.FinishBatch)

	// 质量指标与异常。
	r.Handle("POST", "/api/v1/batches/{id}/metrics", h.AddMetric)
	r.Handle("GET", "/api/v1/batches/{id}/metrics", h.ListMetrics)
	r.Handle("POST", "/api/v1/metrics/{id}/seal", h.SealMetric)
	r.Handle("POST", "/api/v1/batches/{id}/anomalies", h.CreateAnomaly)
	r.Handle("GET", "/api/v1/anomalies", h.ListAnomalies)
	r.Handle("POST", "/api/v1/anomalies/{id}/resolve", h.ResolveAnomaly)
	r.Handle("POST", "/api/v1/anomalies/{id}/close", h.CloseAnomaly)

	// 归档与发布。
	r.Handle("POST", "/api/v1/batches/{id}/archive", h.RequestArchive)
	r.Handle("GET", "/api/v1/archives", h.ListArchives)
	r.Handle("GET", "/api/v1/archives/{id}", h.GetArchive)
	r.Handle("DELETE", "/api/v1/archives/{id}", h.DeleteArchive)
	r.Handle("POST", "/api/v1/archives/{id}/verify-and-publish", h.VerifyAndPublish)
	r.Handle("POST", "/api/v1/archives/{id}/releases", h.SubmitRelease)
	r.Handle("GET", "/api/v1/releases", h.ListReleases)
	r.Handle("GET", "/api/v1/releases/{id}", h.GetRelease)
	r.Handle("POST", "/api/v1/releases/{id}/review", h.ReviewRelease)
	r.Handle("POST", "/api/v1/releases/{id}/revoke", h.RevokeRelease)

	// 分析查询。
	r.Handle("GET", "/api/v1/queries/instruments-pending-calibration", h.QueryPendingCalibration)
	r.Handle("GET", "/api/v1/queries/cryo-anomaly-trend", h.QueryCryoTrend)
	r.Handle("GET", "/api/v1/queries/target-conflicts", h.QueryTargetConflicts)
	r.Handle("GET", "/api/v1/queries/quality-decline", h.QueryQualityDecline)
	r.Handle("GET", "/api/v1/queries/pending-retests", h.QueryPendingRetests)
	r.Handle("GET", "/api/v1/queries/expired-releases", h.QueryExpiredReleases)

	// 后台作业。
	r.Handle("GET", "/api/v1/jobs", h.ListJobs)
	r.Handle("GET", "/api/v1/jobs/{id}", h.GetJob)
	r.Handle("POST", "/api/v1/jobs/{id}/retry", h.RetryJob)

	return &Server{
		http: &http.Server{
			Addr:              cfg.Addr(),
			Handler:           Chain(r, log),
			ReadHeaderTimeout: 10 * time.Second,
		},
		log:     log,
		handler: r,
	}
}

// Handler 暴露底层 http.Handler（测试与内嵌使用）。
func (s *Server) Handler() http.Handler {
	return s.http.Handler
}

// Start 启动 HTTP 服务（阻塞）。
func (s *Server) Start() error {
	s.log.Info("HTTP 服务启动", "addr", s.http.Addr)
	return s.http.ListenAndServe()
}

// Shutdown 优雅关闭。
func (s *Server) Shutdown(ctx context.Context) error {
	return s.http.Shutdown(ctx)
}
