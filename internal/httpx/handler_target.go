package httpx

import (
	"net/http"

	"observatory/internal/model"
)

type scheduleTargetReq struct {
	Name           string  `json:"name"`
	RaDeg          float64 `json:"ra_deg"`
	DecDeg         float64 `json:"dec_deg"`
	Priority       int     `json:"priority"`
	ExposureGoal   int     `json:"exposure_goal"`
	IdempotencyKey string  `json:"idempotency_key"`
}

// ScheduleTarget 目标排程（幂等）。
func (h *Handlers) ScheduleTarget(w http.ResponseWriter, r *http.Request) {
	windowID, err := ParamID(r, "id")
	if err != nil {
		Error(w, err)
		return
	}
	var req scheduleTargetReq
	if err := Decode(r, &req); err != nil {
		Error(w, err)
		return
	}
	t, replay, err := h.svc.Targets.Schedule(r.Context(), windowID, model.Target{
		Name: req.Name, RaDeg: req.RaDeg, DecDeg: req.DecDeg,
		Priority: req.Priority, ExposureGoal: req.ExposureGoal, IdempotencyKey: req.IdempotencyKey,
	}, Actor(r))
	if err != nil {
		Error(w, err)
		return
	}
	status := http.StatusCreated
	if replay {
		status = http.StatusOK
	}
	JSON(w, status, map[string]any{"target": t, "replay": replay})
}

// ListTargets 窗口目标分页。
func (h *Handlers) ListTargets(w http.ResponseWriter, r *http.Request) {
	windowID, err := ParamID(r, "id")
	if err != nil {
		Error(w, err)
		return
	}
	page, err := ParsePage(r)
	if err != nil {
		Error(w, err)
		return
	}
	items, err := h.svc.Targets.ListByWindow(r.Context(), windowID, page)
	if err != nil {
		Error(w, err)
		return
	}
	List(w, items, NextCursor(items, page.Limit, func(m model.Target) int64 { return m.ID }))
}

// GetTarget 目标详情。
func (h *Handlers) GetTarget(w http.ResponseWriter, r *http.Request) {
	id, err := ParamID(r, "id")
	if err != nil {
		Error(w, err)
		return
	}
	t, err := h.svc.Targets.Get(r.Context(), id)
	if err != nil {
		Error(w, err)
		return
	}
	OK(w, t)
}

type addExposureReq struct {
	SeqNo     int64   `json:"seq_no"`
	DurationS float64 `json:"duration_s"`
	Filter    string  `json:"filter"`
}

// AddExposure 追加曝光（序列连续）。
func (h *Handlers) AddExposure(w http.ResponseWriter, r *http.Request) {
	targetID, err := ParamID(r, "id")
	if err != nil {
		Error(w, err)
		return
	}
	var req addExposureReq
	if err := Decode(r, &req); err != nil {
		Error(w, err)
		return
	}
	e, err := h.svc.Targets.AddExposure(r.Context(), targetID, req.SeqNo, req.DurationS, req.Filter, Actor(r))
	if err != nil {
		Error(w, err)
		return
	}
	Created(w, e)
}

// ListExposures 曝光分页。
func (h *Handlers) ListExposures(w http.ResponseWriter, r *http.Request) {
	targetID, err := ParamID(r, "id")
	if err != nil {
		Error(w, err)
		return
	}
	page, err := ParsePage(r)
	if err != nil {
		Error(w, err)
		return
	}
	items, err := h.svc.Targets.ListExposures(r.Context(), targetID, page)
	if err != nil {
		Error(w, err)
		return
	}
	List(w, items, NextCursor(items, page.Limit, func(m model.TargetExposure) int64 { return m.ID }))
}
