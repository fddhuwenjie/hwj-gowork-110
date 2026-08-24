package httpx

import (
	"net/http"

	"observatory/internal/model"
)

// ListReleases 发布许可分页。
func (h *Handlers) ListReleases(w http.ResponseWriter, r *http.Request) {
	page, err := ParsePage(r)
	if err != nil {
		Error(w, err)
		return
	}
	items, err := h.svc.Releases.List(r.Context(), r.URL.Query().Get("status"), page)
	if err != nil {
		Error(w, err)
		return
	}
	List(w, items, NextCursor(items, page.Limit, func(m model.ReleasePermit) int64 { return m.ID }))
}

// GetRelease 发布许可详情。
func (h *Handlers) GetRelease(w http.ResponseWriter, r *http.Request) {
	id, err := ParamID(r, "id")
	if err != nil {
		Error(w, err)
		return
	}
	p, err := h.svc.Releases.Get(r.Context(), id)
	if err != nil {
		Error(w, err)
		return
	}
	OK(w, p)
}

type reviewReleaseReq struct {
	Version  int64  `json:"version"`
	Decision string `json:"decision"`
}

// ReviewRelease 复核发布许可（复核人 ≠ 提交人）。
func (h *Handlers) ReviewRelease(w http.ResponseWriter, r *http.Request) {
	id, err := ParamID(r, "id")
	if err != nil {
		Error(w, err)
		return
	}
	var req reviewReleaseReq
	if err := Decode(r, &req); err != nil {
		Error(w, err)
		return
	}
	var approve bool
	switch req.Decision {
	case "approve":
		approve = true
	case "reject":
		approve = false
	default:
		Error(w, errDecision(req.Decision))
		return
	}
	p, err := h.svc.Releases.Review(r.Context(), id, req.Version, approve, Actor(r))
	if err != nil {
		Error(w, err)
		return
	}
	OK(w, p)
}

// RevokeRelease 撤销发布许可。
func (h *Handlers) RevokeRelease(w http.ResponseWriter, r *http.Request) {
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
	if err := h.svc.Releases.Revoke(r.Context(), id, req.Version, Actor(r)); err != nil {
		Error(w, err)
		return
	}
	OK(w, map[string]string{"status": "revoked"})
}
