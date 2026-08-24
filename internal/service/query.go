package service

import (
	"context"
	"time"

	"observatory/internal/domain"
	"observatory/internal/repo"
)

// QueryService 承载六类分析型稳定分页查询。
type QueryService struct {
	svc     *Services
	queries *repo.QueryRepo
}

// InstrumentsPendingCalibration 临近窗口仍未完成校准的仪器。
func (s *QueryService) InstrumentsPendingCalibration(ctx context.Context, withinHours int,
	page repo.Page) ([]repo.PendingCalibrationRow, error) {
	if withinHours <= 0 {
		withinHours = 72
	}
	horizon := s.svc.Clock.Now().Add(time.Duration(withinHours) * time.Hour)
	return s.queries.InstrumentsPendingCalibration(ctx, s.svc.DB.SQL, horizon, page)
}

// CryoAnomalyTrend 低温异常趋势（按日聚合越界读数）。
func (s *QueryService) CryoAnomalyTrend(ctx context.Context, days int, page repo.Page) ([]repo.CryoAnomalyTrendRow, error) {
	if days <= 0 {
		days = 7
	}
	since := domain.CryoTrendSince(s.svc.Clock.Now(), days)
	return s.queries.CryoAnomalyTrend(ctx, s.svc.DB.SQL, since, page)
}

// TargetConflicts 目标排程冲突（同仪器窗口时间重叠）。
func (s *QueryService) TargetConflicts(ctx context.Context, page repo.Page) ([]repo.TargetConflictRow, error) {
	return s.queries.TargetConflicts(ctx, s.svc.DB.SQL, page)
}

// QualityDeclineRow 为质量连续下降的分析结果行。
type QualityDeclineRow struct {
	InstrumentID  int64     `json:"instrument_id"`
	LatestBatchID int64     `json:"latest_batch_id"`
	BatchIDs      []int64   `json:"batch_ids"`
	Scores        []float64 `json:"scores"`
}

// QualityDecline 质量指标连续下降批次链：按仪器取已封存评分序列，
// 末尾存在不少于 minConsecutive 次相邻严格下降即命中。
func (s *QueryService) QualityDecline(ctx context.Context, minConsecutive int,
	page repo.Page) ([]QualityDeclineRow, error) {
	if minConsecutive < 2 {
		minConsecutive = 2
	}
	points, err := s.queries.SealedScoresByInstrument(ctx, s.svc.DB.SQL)
	if err != nil {
		return nil, err
	}
	type group struct {
		instrumentID int64
		batchIDs     []int64
		scores       []float64
	}
	byInstrument := map[int64]*group{}
	var order []*group
	for _, p := range points {
		g, ok := byInstrument[p.InstrumentID]
		if !ok {
			g = &group{instrumentID: p.InstrumentID}
			byInstrument[p.InstrumentID] = g
			order = append(order, g)
		}
		g.batchIDs = append(g.batchIDs, p.BatchID)
		g.scores = append(g.scores, p.Score)
	}
	var hits []QualityDeclineRow
	for _, g := range order {
		if domain.ConsecutiveDecline(g.scores, minConsecutive) {
			n := len(g.scores)
			hits = append(hits, QualityDeclineRow{
				InstrumentID:  g.instrumentID,
				LatestBatchID: g.batchIDs[n-1],
				BatchIDs:      g.batchIDs,
				Scores:        g.scores,
			})
		}
	}
	// 对命中组做稳定偏移分页。
	page = page.Normalize()
	if page.Cursor >= int64(len(hits)) {
		return nil, nil
	}
	end := page.Cursor + int64(page.Limit)
	if end > int64(len(hits)) {
		end = int64(len(hits))
	}
	return hits[page.Cursor:end], nil
}

// PendingRetests 待复测的隔离批次。
func (s *QueryService) PendingRetests(ctx context.Context, page repo.Page) ([]repo.PendingRetestRow, error) {
	return s.queries.PendingRetests(ctx, s.svc.DB.SQL, page)
}

// ExpiredReleases 已过期发布许可。
func (s *QueryService) ExpiredReleases(ctx context.Context, page repo.Page) ([]repo.ExpiredReleaseRow, error) {
	return s.queries.ExpiredReleases(ctx, s.svc.DB.SQL, s.svc.Clock.Now(), page)
}
