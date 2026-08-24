package httpx

import (
	"net/http"
	"time"

	"observatory/internal/model"
)

type requestArchiveReq struct {
	ChecksumSHA256 string `json:"checksum_sha256"`
	SizeBytes      int64  `json:"size_bytes"`
	IdempotencyKey string `json:"idempotency_key"`
}

// RequestArchive 发起归档请求（幂等）。
func (h *Handlers) RequestArchive(w http.ResponseWriter, r *http.Request) {
	batchID, err := ParamID(r, "id")
	if err != nil {
		Error(w, err)
		return
	}
	var req requestArchiveReq
	if err := Decode(r, &req); err != nil {
		Error(w, err)
		return
	}
	a, replay, err := h.svc.Archives.Request(r.Context(), batchID,
		req.ChecksumSHA256, req.SizeBytes, req.IdempotencyKey, Actor(r))
	if err != nil {
		Error(w, err)
		return
	}
	status := http.StatusCreated
	if replay {
		status = http.StatusOK
	}
	JSON(w, status, map[string]any{"archive": a, "replay": replay})
}

// ListArchives 归档分页。
func (h *Handlers) ListArchives(w http.ResponseWriter, r *http.Request) {
	page, err := ParsePage(r)
	if err != nil {
		Error(w, err)
		return
	}
	items, err := h.svc.Archives.List(r.Context(), r.URL.Query().Get("status"), page)
	if err != nil {
		Error(w, err)
		return
	}
	List(w, items, NextCursor(items, page.Limit, func(m model.Archive) int64 { return m.ID }))
}

// GetArchive 归档详情。
func (h *Handlers) GetArchive(w http.ResponseWriter, r *http.Request) {
	id, err := ParamID(r, "id")
	if err != nil {
		Error(w, err)
		return
	}
	a, err := h.svc.Archives.Get(r.Context(), id)
	if err != nil {
		Error(w, err)
		return
	}
	OK(w, a)
}

// DeleteArchive 软删除归档（不物理删除）。
func (h *Handlers) DeleteArchive(w http.ResponseWriter, r *http.Request) {
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
	if err := h.svc.Archives.SoftDelete(r.Context(), id, req.Version, Actor(r)); err != nil {
		Error(w, err)
		return
	}
	OK(w, map[string]string{"status": "deleted"})
}

type verifyPublishReq struct {
	PermitID int64 `json:"permit_id"`
}

// VerifyAndPublish 归档校验与成果发布事务。
func (h *Handlers) VerifyAndPublish(w http.ResponseWriter, r *http.Request) {
	id, err := ParamID(r, "id")
	if err != nil {
		Error(w, err)
		return
	}
	var req verifyPublishReq
	if err := Decode(r, &req); err != nil {
		Error(w, err)
		return
	}
	a, p, err := h.svc.Archives.VerifyAndPublish(r.Context(), id, req.PermitID, Actor(r))
	if err != nil {
		Error(w, err)
		return
	}
	OK(w, map[string]any{"archive": a, "permit": p})
}

type submitReleaseReq struct {
	Title     string    `json:"title"`
	ExpiresAt time.Time `json:"expires_at"`
}

// SubmitRelease 提交发布许可申请。
func (h *Handlers) SubmitRelease(w http.ResponseWriter, r *http.Request) {
	archiveID, err := ParamID(r, "id")
	if err != nil {
		Error(w, err)
		return
	}
	var req submitReleaseReq
	if err := Decode(r, &req); err != nil {
		Error(w, err)
		return
	}
	p, err := h.svc.Releases.Submit(r.Context(), archiveID, req.Title, req.ExpiresAt, Actor(r))
	if err != nil {
		Error(w, err)
		return
	}
	Created(w, p)
}
