package httpx

import (
	"net/http"

	"observatory/internal/model"
)

type createSiteReq struct {
	Code      string  `json:"code"`
	Name      string  `json:"name"`
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
	AltitudeM float64 `json:"altitude_m"`
}

// CreateSite 建站。
func (h *Handlers) CreateSite(w http.ResponseWriter, r *http.Request) {
	var req createSiteReq
	if err := Decode(r, &req); err != nil {
		Error(w, err)
		return
	}
	site, err := h.svc.Sites.CreateSite(r.Context(), model.Site{
		Code: req.Code, Name: req.Name, Latitude: req.Latitude,
		Longitude: req.Longitude, AltitudeM: req.AltitudeM,
	})
	if err != nil {
		Error(w, err)
		return
	}
	Created(w, site)
}

// ListSites 站点分页。
func (h *Handlers) ListSites(w http.ResponseWriter, r *http.Request) {
	page, err := ParsePage(r)
	if err != nil {
		Error(w, err)
		return
	}
	items, err := h.svc.Sites.ListSites(r.Context(), r.URL.Query().Get("status"), page)
	if err != nil {
		Error(w, err)
		return
	}
	List(w, items, NextCursor(items, page.Limit, func(s model.Site) int64 { return s.ID }))
}

// GetSite 站点详情。
func (h *Handlers) GetSite(w http.ResponseWriter, r *http.Request) {
	id, err := ParamID(r, "id")
	if err != nil {
		Error(w, err)
		return
	}
	site, err := h.svc.Sites.GetSite(r.Context(), id)
	if err != nil {
		Error(w, err)
		return
	}
	OK(w, site)
}

type updateSiteReq struct {
	Name      string  `json:"name"`
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
	AltitudeM float64 `json:"altitude_m"`
	Version   int64   `json:"version"`
}

// UpdateSite 站点更新（乐观锁）。
func (h *Handlers) UpdateSite(w http.ResponseWriter, r *http.Request) {
	id, err := ParamID(r, "id")
	if err != nil {
		Error(w, err)
		return
	}
	var req updateSiteReq
	if err := Decode(r, &req); err != nil {
		Error(w, err)
		return
	}
	site, err := h.svc.Sites.UpdateSite(r.Context(), id, req.Version,
		req.Name, req.Latitude, req.Longitude, req.AltitudeM, Actor(r))
	if err != nil {
		Error(w, err)
		return
	}
	OK(w, site)
}

type versionReq struct {
	Version int64  `json:"version"`
	Reason  string `json:"reason"`
}

// DecommissionSite 站点停用。
func (h *Handlers) DecommissionSite(w http.ResponseWriter, r *http.Request) {
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
	if err := h.svc.Sites.DecommissionSite(r.Context(), id, req.Version, Actor(r)); err != nil {
		Error(w, err)
		return
	}
	OK(w, map[string]string{"status": "decommissioned"})
}
