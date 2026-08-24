package httpx

import (
	"net/http"
	"time"

	"observatory/internal/model"
)

type registerCryoReq struct {
	Name         string  `json:"name"`
	TargetTempMK float64 `json:"target_temp_mK"`
}

// RegisterCryo 登记低温系统。
func (h *Handlers) RegisterCryo(w http.ResponseWriter, r *http.Request) {
	id, err := ParamID(r, "id")
	if err != nil {
		Error(w, err)
		return
	}
	var req registerCryoReq
	if err := Decode(r, &req); err != nil {
		Error(w, err)
		return
	}
	c, err := h.svc.Cryo.RegisterSystem(r.Context(), id, req.Name, req.TargetTempMK)
	if err != nil {
		Error(w, err)
		return
	}
	Created(w, c)
}

// GetCryo 低温系统详情。
func (h *Handlers) GetCryo(w http.ResponseWriter, r *http.Request) {
	id, err := ParamID(r, "id")
	if err != nil {
		Error(w, err)
		return
	}
	c, err := h.svc.Cryo.GetSystem(r.Context(), id)
	if err != nil {
		Error(w, err)
		return
	}
	OK(w, c)
}

type startPrecoolReq struct {
	TargetTempMK float64   `json:"target_temp_mK"`
	DeadlineAt   time.Time `json:"deadline_at"`
}

// StartPrecool 开始预冷。
func (h *Handlers) StartPrecool(w http.ResponseWriter, r *http.Request) {
	id, err := ParamID(r, "id")
	if err != nil {
		Error(w, err)
		return
	}
	var req startPrecoolReq
	if err := Decode(r, &req); err != nil {
		Error(w, err)
		return
	}
	sess, err := h.svc.Cryo.StartPrecool(r.Context(), id, req.TargetTempMK, req.DeadlineAt, Actor(r))
	if err != nil {
		Error(w, err)
		return
	}
	Created(w, sess)
}

type addReadingReq struct {
	TempMK         float64   `json:"temp_mK"`
	PressureMbar   float64   `json:"pressure_mbar"`
	RecordedAt     time.Time `json:"recorded_at"`
	IdempotencyKey string    `json:"idempotency_key"`
}

// AddReading 写入温度读数（幂等）。
func (h *Handlers) AddReading(w http.ResponseWriter, r *http.Request) {
	id, err := ParamID(r, "id")
	if err != nil {
		Error(w, err)
		return
	}
	var req addReadingReq
	if err := Decode(r, &req); err != nil {
		Error(w, err)
		return
	}
	rd, replay, err := h.svc.Cryo.AddReading(r.Context(), id, req.TempMK, req.PressureMbar,
		req.RecordedAt, req.IdempotencyKey, Actor(r))
	if err != nil {
		Error(w, err)
		return
	}
	status := http.StatusCreated
	if replay {
		status = http.StatusOK
	}
	JSON(w, status, map[string]any{"reading": rd, "replay": replay})
}

// ListReadings 读数分页。
func (h *Handlers) ListReadings(w http.ResponseWriter, r *http.Request) {
	id, err := ParamID(r, "id")
	if err != nil {
		Error(w, err)
		return
	}
	page, err := ParsePage(r)
	if err != nil {
		Error(w, err)
		return
	}
	items, err := h.svc.Cryo.ListReadings(r.Context(), id, page)
	if err != nil {
		Error(w, err)
		return
	}
	List(w, items, NextCursor(items, page.Limit, func(m model.CryoReading) int64 { return m.ID }))
}

// ListPrecoolSessions 预冷会话分页。
func (h *Handlers) ListPrecoolSessions(w http.ResponseWriter, r *http.Request) {
	id, err := ParamID(r, "id")
	if err != nil {
		Error(w, err)
		return
	}
	page, err := ParsePage(r)
	if err != nil {
		Error(w, err)
		return
	}
	items, err := h.svc.Cryo.ListSessions(r.Context(), id, page)
	if err != nil {
		Error(w, err)
		return
	}
	List(w, items, NextCursor(items, page.Limit, func(m model.PrecoolSession) int64 { return m.ID }))
}
