package httpx

import (
	"net/http"

	"observatory/internal/domain"
	"observatory/internal/model"
)

type createInstrumentReq struct {
	SiteID    int64   `json:"site_id"`
	Code      string  `json:"code"`
	Name      string  `json:"name"`
	Kind      string  `json:"kind"`
	TempMinMK float64 `json:"temp_min_mK"`
	TempMaxMK float64 `json:"temp_max_mK"`
}

// CreateInstrument 仪器建档。
func (h *Handlers) CreateInstrument(w http.ResponseWriter, r *http.Request) {
	var req createInstrumentReq
	if err := Decode(r, &req); err != nil {
		Error(w, err)
		return
	}
	in, err := h.svc.Instruments.CreateInstrument(r.Context(), model.Instrument{
		SiteID: req.SiteID, Code: req.Code, Name: req.Name, Kind: req.Kind,
		TempMinMK: req.TempMinMK, TempMaxMK: req.TempMaxMK,
	})
	if err != nil {
		Error(w, err)
		return
	}
	Created(w, in)
}

// ListInstruments 仪器分页。
func (h *Handlers) ListInstruments(w http.ResponseWriter, r *http.Request) {
	page, err := ParsePage(r)
	if err != nil {
		Error(w, err)
		return
	}
	siteID, err := QueryInt64(r, "site_id")
	if err != nil {
		Error(w, err)
		return
	}
	items, err := h.svc.Instruments.ListInstruments(r.Context(), siteID, r.URL.Query().Get("status"), page)
	if err != nil {
		Error(w, err)
		return
	}
	List(w, items, NextCursor(items, page.Limit, func(m model.Instrument) int64 { return m.ID }))
}

// GetInstrument 仪器详情。
func (h *Handlers) GetInstrument(w http.ResponseWriter, r *http.Request) {
	id, err := ParamID(r, "id")
	if err != nil {
		Error(w, err)
		return
	}
	in, err := h.svc.Instruments.GetInstrument(r.Context(), id)
	if err != nil {
		Error(w, err)
		return
	}
	OK(w, in)
}

// InstrumentHistory 仪器状态历史。
func (h *Handlers) InstrumentHistory(w http.ResponseWriter, r *http.Request) {
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
	items, err := h.svc.Instruments.ListHistory(r.Context(), id, page)
	if err != nil {
		Error(w, err)
		return
	}
	List(w, items, NextCursor(items, page.Limit, func(m model.InstrumentStatusHistory) int64 { return m.ID }))
}

type updateInstrumentReq struct {
	Name      string  `json:"name"`
	Kind      string  `json:"kind"`
	TempMinMK float64 `json:"temp_min_mK"`
	TempMaxMK float64 `json:"temp_max_mK"`
	Version   int64   `json:"version"`
}

// UpdateInstrument 仪器更新（乐观锁，冻结期拒绝）。
func (h *Handlers) UpdateInstrument(w http.ResponseWriter, r *http.Request) {
	id, err := ParamID(r, "id")
	if err != nil {
		Error(w, err)
		return
	}
	var req updateInstrumentReq
	if err := Decode(r, &req); err != nil {
		Error(w, err)
		return
	}
	in, err := h.svc.Instruments.UpdateInstrument(r.Context(), id, req.Version,
		req.Name, req.Kind, req.TempMinMK, req.TempMaxMK, Actor(r))
	if err != nil {
		Error(w, err)
		return
	}
	OK(w, in)
}

// instrumentTransition 仪器状态转换通用处理。
func (h *Handlers) instrumentTransition(w http.ResponseWriter, r *http.Request, to string) {
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
	in, err := h.svc.Instruments.ChangeStatus(r.Context(), id, req.Version, to, req.Reason, Actor(r))
	if err != nil {
		Error(w, err)
		return
	}
	OK(w, in)
}

// InstrumentMaintenance 转维护。
func (h *Handlers) InstrumentMaintenance(w http.ResponseWriter, r *http.Request) {
	h.instrumentTransition(w, r, domain.InstrumentMaintenance)
}

// InstrumentRestore 维护复位。
func (h *Handlers) InstrumentRestore(w http.ResponseWriter, r *http.Request) {
	h.instrumentTransition(w, r, domain.InstrumentRegistered)
}

// InstrumentDecommission 停用。
func (h *Handlers) InstrumentDecommission(w http.ResponseWriter, r *http.Request) {
	h.instrumentTransition(w, r, domain.InstrumentDecommissioned)
}

type createChannelReq struct {
	ChannelNo    int     `json:"channel_no"`
	Name         string  `json:"name"`
	WavelengthNM float64 `json:"wavelength_nm"`
	Gain         float64 `json:"gain"`
	Offset       float64 `json:"offset"`
}

// AddChannel 新增通道。
func (h *Handlers) AddChannel(w http.ResponseWriter, r *http.Request) {
	id, err := ParamID(r, "id")
	if err != nil {
		Error(w, err)
		return
	}
	var req createChannelReq
	if err := Decode(r, &req); err != nil {
		Error(w, err)
		return
	}
	c, err := h.svc.Instruments.AddChannel(r.Context(), id, model.DetectorChannel{
		ChannelNo: req.ChannelNo, Name: req.Name, WavelengthNM: req.WavelengthNM,
		Gain: req.Gain, Offset: req.Offset,
	})
	if err != nil {
		Error(w, err)
		return
	}
	Created(w, c)
}

// ListChannels 通道分页。
func (h *Handlers) ListChannels(w http.ResponseWriter, r *http.Request) {
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
	items, err := h.svc.Instruments.ListChannels(r.Context(), id, page)
	if err != nil {
		Error(w, err)
		return
	}
	List(w, items, NextCursor(items, page.Limit, func(m model.DetectorChannel) int64 { return m.ID }))
}

type updateChannelReq struct {
	Name    string  `json:"name"`
	Gain    float64 `json:"gain"`
	Offset  float64 `json:"offset"`
	Status  string  `json:"status"`
	Version int64   `json:"version"`
}

// UpdateChannel 通道更新（乐观锁，冻结期拒绝）。
func (h *Handlers) UpdateChannel(w http.ResponseWriter, r *http.Request) {
	id, err := ParamID(r, "id")
	if err != nil {
		Error(w, err)
		return
	}
	var req updateChannelReq
	if err := Decode(r, &req); err != nil {
		Error(w, err)
		return
	}
	c, err := h.svc.Instruments.UpdateChannel(r.Context(), id, req.Version,
		req.Name, req.Gain, req.Offset, req.Status, Actor(r))
	if err != nil {
		Error(w, err)
		return
	}
	OK(w, c)
}
