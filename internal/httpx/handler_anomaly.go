package httpx

import (
	"net/http"

	"observatory/internal/domain"
	"observatory/internal/model"
)

type createAnomalyReq struct {
	Kind        string `json:"kind"`
	Description string `json:"description"`
}

// CreateAnomaly 手工登记批次异常。
func (h *Handlers) CreateAnomaly(w http.ResponseWriter, r *http.Request) {
	batchID, err := ParamID(r, "id")
	if err != nil {
		Error(w, err)
		return
	}
	var req createAnomalyReq
	if err := Decode(r, &req); err != nil {
		Error(w, err)
		return
	}
	kind := req.Kind
	if kind == "" {
		kind = domain.AnomalyManual
	}
	a, err := h.svc.Anomalies.CreateManual(r.Context(), &batchID, 0, kind, req.Description, Actor(r))
	if err != nil {
		Error(w, err)
		return
	}
	Created(w, a)
}

// ListAnomalies 异常分页。
func (h *Handlers) ListAnomalies(w http.ResponseWriter, r *http.Request) {
	page, err := ParsePage(r)
	if err != nil {
		Error(w, err)
		return
	}
	instrumentID, err := QueryInt64(r, "instrument_id")
	if err != nil {
		Error(w, err)
		return
	}
	items, err := h.svc.Anomalies.List(r.Context(), instrumentID, r.URL.Query().Get("status"), page)
	if err != nil {
		Error(w, err)
		return
	}
	List(w, items, NextCursor(items, page.Limit, func(m model.Anomaly) int64 { return m.ID }))
}

// ResolveAnomaly 处置完成。
func (h *Handlers) ResolveAnomaly(w http.ResponseWriter, r *http.Request) {
	id, err := ParamID(r, "id")
	if err != nil {
		Error(w, err)
		return
	}
	a, err := h.svc.Anomalies.Resolve(r.Context(), id, Actor(r))
	if err != nil {
		Error(w, err)
		return
	}
	OK(w, a)
}

// CloseAnomaly 关闭异常。
func (h *Handlers) CloseAnomaly(w http.ResponseWriter, r *http.Request) {
	id, err := ParamID(r, "id")
	if err != nil {
		Error(w, err)
		return
	}
	a, err := h.svc.Anomalies.Close(r.Context(), id, Actor(r))
	if err != nil {
		Error(w, err)
		return
	}
	OK(w, a)
}
