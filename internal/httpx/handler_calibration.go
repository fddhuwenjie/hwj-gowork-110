package httpx

import (
	"net/http"
	"time"

	"observatory/internal/model"
)

type createPlanReq struct {
	VersionNo  int       `json:"version_no"`
	Params     string    `json:"params"`
	ValidFrom  time.Time `json:"valid_from"`
	ValidUntil time.Time `json:"valid_until"`
}

// CreatePlan 创建校准方案草稿。
func (h *Handlers) CreatePlan(w http.ResponseWriter, r *http.Request) {
	id, err := ParamID(r, "id")
	if err != nil {
		Error(w, err)
		return
	}
	var req createPlanReq
	if err := Decode(r, &req); err != nil {
		Error(w, err)
		return
	}
	p, err := h.svc.Calibration.CreatePlan(r.Context(), model.CalibrationPlan{
		InstrumentID: id, VersionNo: req.VersionNo, Params: req.Params,
		ValidFrom: req.ValidFrom, ValidUntil: req.ValidUntil,
	}, Actor(r))
	if err != nil {
		Error(w, err)
		return
	}
	Created(w, p)
}

// ListPlans 方案分页。
func (h *Handlers) ListPlans(w http.ResponseWriter, r *http.Request) {
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
	items, err := h.svc.Calibration.ListPlans(r.Context(), instrumentID, page)
	if err != nil {
		Error(w, err)
		return
	}
	List(w, items, NextCursor(items, page.Limit, func(m model.CalibrationPlan) int64 { return m.ID }))
}

// ApprovePlan 审批方案。
func (h *Handlers) ApprovePlan(w http.ResponseWriter, r *http.Request) {
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
	p, err := h.svc.Calibration.ApprovePlan(r.Context(), id, req.Version, Actor(r))
	if err != nil {
		Error(w, err)
		return
	}
	OK(w, p)
}

// ActivatePlan 启用方案。
func (h *Handlers) ActivatePlan(w http.ResponseWriter, r *http.Request) {
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
	p, err := h.svc.Calibration.ActivatePlan(r.Context(), id, req.Version, Actor(r))
	if err != nil {
		Error(w, err)
		return
	}
	OK(w, p)
}

type createSourceReq struct {
	Code     string  `json:"code"`
	Name     string  `json:"name"`
	FluxJy   float64 `json:"flux_jy"`
	Spectrum string  `json:"spectrum"`
}

// CreateSource 登记标准源。
func (h *Handlers) CreateSource(w http.ResponseWriter, r *http.Request) {
	var req createSourceReq
	if err := Decode(r, &req); err != nil {
		Error(w, err)
		return
	}
	src, err := h.svc.Calibration.CreateSource(r.Context(), model.StandardSource{
		Code: req.Code, Name: req.Name, FluxJy: req.FluxJy, Spectrum: req.Spectrum,
	})
	if err != nil {
		Error(w, err)
		return
	}
	Created(w, src)
}

// ListSources 标准源分页。
func (h *Handlers) ListSources(w http.ResponseWriter, r *http.Request) {
	page, err := ParsePage(r)
	if err != nil {
		Error(w, err)
		return
	}
	items, err := h.svc.Calibration.ListSources(r.Context(), page)
	if err != nil {
		Error(w, err)
		return
	}
	List(w, items, NextCursor(items, page.Limit, func(m model.StandardSource) int64 { return m.ID }))
}

type createRecordReq struct {
	PlanID       int64     `json:"plan_id"`
	SourceID     int64     `json:"source_id"`
	Result       string    `json:"result"`
	DeviationPct float64   `json:"deviation_pct"`
	EvidenceURI  string    `json:"evidence_uri"`
	PerformedAt  time.Time `json:"performed_at"`
}

// CreateCalibrationRecord 登记校准记录（不可变证据）。
func (h *Handlers) CreateCalibrationRecord(w http.ResponseWriter, r *http.Request) {
	var req createRecordReq
	if err := Decode(r, &req); err != nil {
		Error(w, err)
		return
	}
	rec, err := h.svc.Calibration.CreateRecord(r.Context(), model.CalibrationRecord{
		PlanID: req.PlanID, SourceID: req.SourceID, Result: req.Result,
		DeviationPct: req.DeviationPct, EvidenceURI: req.EvidenceURI, PerformedAt: req.PerformedAt,
	}, Actor(r))
	if err != nil {
		Error(w, err)
		return
	}
	Created(w, rec)
}

// ListCalibrationRecords 校准记录分页。
func (h *Handlers) ListCalibrationRecords(w http.ResponseWriter, r *http.Request) {
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
	items, err := h.svc.Calibration.ListRecords(r.Context(), instrumentID, page)
	if err != nil {
		Error(w, err)
		return
	}
	List(w, items, NextCursor(items, page.Limit, func(m model.CalibrationRecord) int64 { return m.ID }))
}
