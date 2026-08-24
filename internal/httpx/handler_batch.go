package httpx

import (
	"net/http"

	"observatory/internal/model"
)

type startBatchReq struct {
	TargetID       int64  `json:"target_id"`
	ObjectURI      string `json:"object_uri"`
	IdempotencyKey string `json:"idempotency_key"`
	RetestOfID     *int64 `json:"retest_of_id"`
}

// StartBatch 开始观测批次。
func (h *Handlers) StartBatch(w http.ResponseWriter, r *http.Request) {
	windowID, err := ParamID(r, "id")
	if err != nil {
		Error(w, err)
		return
	}
	var req startBatchReq
	if err := Decode(r, &req); err != nil {
		Error(w, err)
		return
	}
	b, replay, err := h.svc.Batches.Start(r.Context(), windowID, req.TargetID,
		req.ObjectURI, req.IdempotencyKey, req.RetestOfID, Actor(r))
	if err != nil {
		Error(w, err)
		return
	}
	status := http.StatusCreated
	if replay {
		status = http.StatusOK
	}
	JSON(w, status, map[string]any{"batch": b, "replay": replay})
}

// ListBatches 窗口批次分页。
func (h *Handlers) ListBatches(w http.ResponseWriter, r *http.Request) {
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
	items, err := h.svc.Batches.ListByWindow(r.Context(), windowID, page)
	if err != nil {
		Error(w, err)
		return
	}
	List(w, items, NextCursor(items, page.Limit, func(m model.ObservationBatch) int64 { return m.ID }))
}

// GetBatch 批次详情。
func (h *Handlers) GetBatch(w http.ResponseWriter, r *http.Request) {
	id, err := ParamID(r, "id")
	if err != nil {
		Error(w, err)
		return
	}
	b, err := h.svc.Batches.Get(r.Context(), id)
	if err != nil {
		Error(w, err)
		return
	}
	OK(w, b)
}

// FinishBatch 结束批次（冻结）。
func (h *Handlers) FinishBatch(w http.ResponseWriter, r *http.Request) {
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
	b, err := h.svc.Batches.Finish(r.Context(), id, req.Version, Actor(r))
	if err != nil {
		Error(w, err)
		return
	}
	OK(w, b)
}
