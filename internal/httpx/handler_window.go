package httpx

import (
	"net/http"
	"time"

	"observatory/internal/model"
)

type applyWindowReq struct {
	InstrumentID int64     `json:"instrument_id"`
	Title        string    `json:"title"`
	StartAt      time.Time `json:"start_at"`
	EndAt        time.Time `json:"end_at"`
}

// ApplyWindow 申请观测窗口。
func (h *Handlers) ApplyWindow(w http.ResponseWriter, r *http.Request) {
	var req applyWindowReq
	if err := Decode(r, &req); err != nil {
		Error(w, err)
		return
	}
	win, err := h.svc.Windows.Apply(r.Context(), model.ObservationWindow{
		InstrumentID: req.InstrumentID, Title: req.Title, StartAt: req.StartAt, EndAt: req.EndAt,
	}, Actor(r))
	if err != nil {
		Error(w, err)
		return
	}
	Created(w, win)
}

// ListWindows 窗口分页。
func (h *Handlers) ListWindows(w http.ResponseWriter, r *http.Request) {
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
	items, err := h.svc.Windows.List(r.Context(), instrumentID, r.URL.Query().Get("status"), page)
	if err != nil {
		Error(w, err)
		return
	}
	List(w, items, NextCursor(items, page.Limit, func(m model.ObservationWindow) int64 { return m.ID }))
}

// GetWindow 窗口详情（含冻结快照）。
func (h *Handlers) GetWindow(w http.ResponseWriter, r *http.Request) {
	id, err := ParamID(r, "id")
	if err != nil {
		Error(w, err)
		return
	}
	win, err := h.svc.Windows.Get(r.Context(), id)
	if err != nil {
		Error(w, err)
		return
	}
	OK(w, win)
}

type approveWindowReq struct {
	Version int64 `json:"version"`
	PlanID  int64 `json:"plan_id"`
}

// ApproveWindow 批准窗口（生成冻结快照）。
func (h *Handlers) ApproveWindow(w http.ResponseWriter, r *http.Request) {
	id, err := ParamID(r, "id")
	if err != nil {
		Error(w, err)
		return
	}
	var req approveWindowReq
	if err := Decode(r, &req); err != nil {
		Error(w, err)
		return
	}
	win, err := h.svc.Windows.Approve(r.Context(), id, req.Version, req.PlanID, Actor(r))
	if err != nil {
		Error(w, err)
		return
	}
	OK(w, win)
}

// ActivateWindow 激活窗口（校准封存与观测启用事务）。
func (h *Handlers) ActivateWindow(w http.ResponseWriter, r *http.Request) {
	id, err := ParamID(r, "id")
	if err != nil {
		Error(w, err)
		return
	}
	var req versionReq
	if err := Decode(r, &req); err != nil {
		Error(w, err)
		return
	}
	win, err := h.svc.Windows.Activate(r.Context(), id, req.Version, Actor(r))
	if err != nil {
		Error(w, err)
		return
	}
	OK(w, win)
}

// CloseWindow 关闭窗口。
func (h *Handlers) CloseWindow(w http.ResponseWriter, r *http.Request) {
	id, err := ParamID(r, "id")
	if err != nil {
		Error(w, err)
		return
	}
	var req versionReq
	if err := Decode(r, &req); err != nil {
		Error(w, err)
		return
	}
	if err := h.svc.Windows.Close(r.Context(), id, req.Version, Actor(r), req.Reason); err != nil {
		Error(w, err)
		return
	}
	OK(w, map[string]string{"status": "closed"})
}

// CancelWindow 撤销窗口。
func (h *Handlers) CancelWindow(w http.ResponseWriter, r *http.Request) {
	id, err := ParamID(r, "id")
	if err != nil {
		Error(w, err)
		return
	}
	var req versionReq
	if err := Decode(r, &req); err != nil {
		Error(w, err)
		return
	}
	if err := h.svc.Windows.Cancel(r.Context(), id, req.Version, Actor(r)); err != nil {
		Error(w, err)
		return
	}
	OK(w, map[string]string{"status": "cancelled"})
}
