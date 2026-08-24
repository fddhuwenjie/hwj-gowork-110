package httpx

import (
	"net/http"

	"observatory/internal/repo"
)

// QueryPendingCalibration 临近窗口仍未完成校准的仪器。
func (h *Handlers) QueryPendingCalibration(w http.ResponseWriter, r *http.Request) {
	page, err := ParsePage(r)
	if err != nil {
		Error(w, err)
		return
	}
	hours, err := QueryInt(r, "within_hours")
	if err != nil {
		Error(w, err)
		return
	}
	items, err := h.svc.Queries.InstrumentsPendingCalibration(r.Context(), hours, page)
	if err != nil {
		Error(w, err)
		return
	}
	List(w, items, NextCursor(items, page.Limit, func(m repo.PendingCalibrationRow) int64 { return m.WindowID }))
}

// QueryCryoTrend 低温异常趋势。
func (h *Handlers) QueryCryoTrend(w http.ResponseWriter, r *http.Request) {
	page, err := ParsePage(r)
	if err != nil {
		Error(w, err)
		return
	}
	days, err := QueryInt(r, "days")
	if err != nil {
		Error(w, err)
		return
	}
	items, err := h.svc.Queries.CryoAnomalyTrend(r.Context(), days, page)
	if err != nil {
		Error(w, err)
		return
	}
	List(w, items, NextOffsetCursor(len(items), page.Limit, page.Cursor))
}

// QueryTargetConflicts 目标排程冲突。
func (h *Handlers) QueryTargetConflicts(w http.ResponseWriter, r *http.Request) {
	page, err := ParsePage(r)
	if err != nil {
		Error(w, err)
		return
	}
	items, err := h.svc.Queries.TargetConflicts(r.Context(), page)
	if err != nil {
		Error(w, err)
		return
	}
	List(w, items, NextOffsetCursor(len(items), page.Limit, page.Cursor))
}

// QueryQualityDecline 质量指标连续下降批次。
func (h *Handlers) QueryQualityDecline(w http.ResponseWriter, r *http.Request) {
	page, err := ParsePage(r)
	if err != nil {
		Error(w, err)
		return
	}
	minConsecutive, err := QueryInt(r, "min_consecutive")
	if err != nil {
		Error(w, err)
		return
	}
	items, err := h.svc.Queries.QualityDecline(r.Context(), minConsecutive, page)
	if err != nil {
		Error(w, err)
		return
	}
	List(w, items, NextOffsetCursor(len(items), page.Limit, page.Cursor))
}

// QueryPendingRetests 待复测批次。
func (h *Handlers) QueryPendingRetests(w http.ResponseWriter, r *http.Request) {
	page, err := ParsePage(r)
	if err != nil {
		Error(w, err)
		return
	}
	items, err := h.svc.Queries.PendingRetests(r.Context(), page)
	if err != nil {
		Error(w, err)
		return
	}
	List(w, items, NextCursor(items, page.Limit, func(m repo.PendingRetestRow) int64 { return m.BatchID }))
}

// QueryExpiredReleases 已过期发布许可。
func (h *Handlers) QueryExpiredReleases(w http.ResponseWriter, r *http.Request) {
	page, err := ParsePage(r)
	if err != nil {
		Error(w, err)
		return
	}
	items, err := h.svc.Queries.ExpiredReleases(r.Context(), page)
	if err != nil {
		Error(w, err)
		return
	}
	List(w, items, NextCursor(items, page.Limit, func(m repo.ExpiredReleaseRow) int64 { return m.PermitID }))
}
