package httpx

import (
	"net/http"

	"observatory/internal/apperr"
	"observatory/internal/model"
)

// Healthz 健康检查：返回服务与数据库状态。
func (h *Handlers) Healthz(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.DB.Ping(r.Context()); err != nil {
		Error(w, apperr.New(apperr.CodeInternal, "数据库不可用"))
		return
	}
	OK(w, map[string]any{"status": "ok", "time": h.svc.Clock.Now()})
}

// ListJobs 后台作业分页。
func (h *Handlers) ListJobs(w http.ResponseWriter, r *http.Request) {
	page, err := ParsePage(r)
	if err != nil {
		Error(w, err)
		return
	}
	items, err := h.svc.Jobs.List(r.Context(), h.svc.DB.SQL,
		r.URL.Query().Get("type"), r.URL.Query().Get("status"), page)
	if err != nil {
		Error(w, err)
		return
	}
	List(w, items, NextCursor(items, page.Limit, func(m model.Job) int64 { return m.ID }))
}

// GetJob 作业详情。
func (h *Handlers) GetJob(w http.ResponseWriter, r *http.Request) {
	id, err := ParamID(r, "id")
	if err != nil {
		Error(w, err)
		return
	}
	j, err := h.svc.Jobs.Get(r.Context(), h.svc.DB.SQL, id)
	if err != nil {
		Error(w, err)
		return
	}
	OK(w, j)
}

// RetryJob 人工重试失败作业。
func (h *Handlers) RetryJob(w http.ResponseWriter, r *http.Request) {
	id, err := ParamID(r, "id")
	if err != nil {
		Error(w, err)
		return
	}
	if err := h.svc.Jobs.Retry(r.Context(), h.svc.DB.SQL, id, h.svc.Clock.Now()); err != nil {
		Error(w, err)
		return
	}
	OK(w, map[string]string{"status": "pending"})
}

func errDecision(v string) error {
	return apperr.InvalidArgument("decision 必须为 approve 或 reject").WithDetail("decision", v)
}
