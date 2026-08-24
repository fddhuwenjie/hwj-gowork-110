package httpx

import (
	"net/http"

	"observatory/internal/model"
)

type addMetricReq struct {
	SNR        float64 `json:"snr"`
	FWHM       float64 `json:"fwhm"`
	Background float64 `json:"background"`
}

// AddMetric 计算并登记质量指标版本。
func (h *Handlers) AddMetric(w http.ResponseWriter, r *http.Request) {
	batchID, err := ParamID(r, "id")
	if err != nil {
		Error(w, err)
		return
	}
	var req addMetricReq
	if err := Decode(r, &req); err != nil {
		Error(w, err)
		return
	}
	m, err := h.svc.Metrics.Add(r.Context(), batchID, req.SNR, req.FWHM, req.Background, Actor(r))
	if err != nil {
		Error(w, err)
		return
	}
	Created(w, m)
}

// ListMetrics 批次指标分页。
func (h *Handlers) ListMetrics(w http.ResponseWriter, r *http.Request) {
	batchID, err := ParamID(r, "id")
	if err != nil {
		Error(w, err)
		return
	}
	page, err := ParsePage(r)
	if err != nil {
		Error(w, err)
		return
	}
	items, err := h.svc.Metrics.ListByBatch(r.Context(), batchID, page)
	if err != nil {
		Error(w, err)
		return
	}
	List(w, items, NextCursor(items, page.Limit, func(m model.QualityMetric) int64 { return m.ID }))
}

// SealMetric 封存指标（指标封存与异常复测事务）。
func (h *Handlers) SealMetric(w http.ResponseWriter, r *http.Request) {
	id, err := ParamID(r, "id")
	if err != nil {
		Error(w, err)
		return
	}
	result, err := h.svc.Metrics.Seal(r.Context(), id, Actor(r))
	if err != nil {
		Error(w, err)
		return
	}
	OK(w, result)
}
