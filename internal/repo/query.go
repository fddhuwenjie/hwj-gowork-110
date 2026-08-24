package repo

import (
	"context"
	"time"

	"observatory/internal/store/sqlite"
)

// QueryRepo 承载六类分析型稳定分页查询。
type QueryRepo struct{}

// NewQueryRepo 创建查询仓储。
func NewQueryRepo() *QueryRepo { return &QueryRepo{} }

// PendingCalibrationRow 为临近窗口仍未完成校准的仪器行。
type PendingCalibrationRow struct {
	WindowID     int64     `json:"window_id"`
	InstrumentID int64     `json:"instrument_id"`
	WindowTitle  string    `json:"window_title"`
	WindowStatus string    `json:"window_status"`
	StartAt      time.Time `json:"start_at"`
	EndAt        time.Time `json:"end_at"`
	PlanID       int64     `json:"plan_id"`
}

// InstrumentsPendingCalibration 查询 starting 于 horizon 内、但冻结方案缺少合格校准记录的窗口。
func (r *QueryRepo) InstrumentsPendingCalibration(ctx context.Context, q sqlite.Querier,
	horizon time.Time, page Page) ([]PendingCalibrationRow, error) {
	page = page.Normalize()
	rows, err := q.QueryContext(ctx,
		`SELECT w.id, w.instrument_id, w.title, w.status, w.start_at, w.end_at, w.plan_id
		 FROM observation_windows w
		 WHERE w.status IN ('approved','active') AND w.start_at <= ? AND w.id > ?
		 AND NOT EXISTS (
			SELECT 1 FROM calibration_records cr
			WHERE cr.plan_id = w.plan_id AND cr.result='pass' AND cr.performed_at <= w.end_at)
		 ORDER BY w.id LIMIT ?`,
		ts(horizon), page.Cursor, page.Limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []PendingCalibrationRow
	for rows.Next() {
		var row PendingCalibrationRow
		var startAt, endAt string
		if err := rows.Scan(&row.WindowID, &row.InstrumentID, &row.WindowTitle,
			&row.WindowStatus, &startAt, &endAt, &row.PlanID); err != nil {
			return nil, err
		}
		if row.StartAt, err = parseTime(startAt); err != nil {
			return nil, err
		}
		if row.EndAt, err = parseTime(endAt); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

// CryoAnomalyTrendRow 为低温异常趋势行（按系统按日聚合越界读数）。
type CryoAnomalyTrendRow struct {
	CryoSystemID int64   `json:"cryo_system_id"`
	InstrumentID int64   `json:"instrument_id"`
	Day          string  `json:"day"`
	OutOfRange   int64   `json:"out_of_range_count"`
	MinTempMK    float64 `json:"min_temp_mK"`
	MaxTempMK    float64 `json:"max_temp_mK"`
}

// CryoAnomalyTrend 聚合 since 以来的越界温度读数；Cursor 为稳定偏移量。
func (r *QueryRepo) CryoAnomalyTrend(ctx context.Context, q sqlite.Querier,
	since time.Time, page Page) ([]CryoAnomalyTrendRow, error) {
	page = page.Normalize()
	rows, err := q.QueryContext(ctx,
		`SELECT cs.id, cs.instrument_id, substr(rd.recorded_at,1,10) AS day,
			COUNT(*) AS cnt, MIN(rd.temp_mk), MAX(rd.temp_mk)
		 FROM cryo_readings rd
		 JOIN cryo_systems cs ON cs.id = rd.cryo_system_id
		 JOIN instruments i ON i.id = cs.instrument_id
		 WHERE rd.recorded_at >= ? AND (rd.temp_mk < i.temp_min_mk OR rd.temp_mk > i.temp_max_mk)
		 GROUP BY cs.id, day
		 ORDER BY day, cs.id
		 LIMIT ? OFFSET ?`,
		ts(since), page.Limit, page.Cursor)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []CryoAnomalyTrendRow
	for rows.Next() {
		var row CryoAnomalyTrendRow
		if err := rows.Scan(&row.CryoSystemID, &row.InstrumentID, &row.Day,
			&row.OutOfRange, &row.MinTempMK, &row.MaxTempMK); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

// TargetConflictRow 为目标排程冲突行（同仪器已批准/激活窗口时间重叠）。
type TargetConflictRow struct {
	InstrumentID int64     `json:"instrument_id"`
	WindowAID    int64     `json:"window_a_id"`
	WindowBID    int64     `json:"window_b_id"`
	AStartAt     time.Time `json:"a_start_at"`
	AEndAt       time.Time `json:"a_end_at"`
	BStartAt     time.Time `json:"b_start_at"`
	BEndAt       time.Time `json:"b_end_at"`
}

// TargetConflicts 查询同仪器时间重叠的窗口对；Cursor 为稳定偏移量。
func (r *QueryRepo) TargetConflicts(ctx context.Context, q sqlite.Querier, page Page) ([]TargetConflictRow, error) {
	page = page.Normalize()
	rows, err := q.QueryContext(ctx,
		`SELECT a.instrument_id, a.id, b.id, a.start_at, a.end_at, b.start_at, b.end_at
		 FROM observation_windows a
		 JOIN observation_windows b ON a.instrument_id=b.instrument_id AND a.id < b.id
		 WHERE a.status IN ('approved','active') AND b.status IN ('approved','active')
		 AND a.start_at < b.end_at AND b.start_at < a.end_at
		 ORDER BY a.id, b.id
		 LIMIT ? OFFSET ?`, page.Limit, page.Cursor)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []TargetConflictRow
	for rows.Next() {
		var row TargetConflictRow
		var as, ae, bs, be string
		if err := rows.Scan(&row.InstrumentID, &row.WindowAID, &row.WindowBID, &as, &ae, &bs, &be); err != nil {
			return nil, err
		}
		var err2 error
		if row.AStartAt, err2 = parseTime(as); err2 != nil {
			return nil, err2
		}
		if row.AEndAt, err2 = parseTime(ae); err2 != nil {
			return nil, err2
		}
		if row.BStartAt, err2 = parseTime(bs); err2 != nil {
			return nil, err2
		}
		if row.BEndAt, err2 = parseTime(be); err2 != nil {
			return nil, err2
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

// SealedScorePoint 为已封存指标评分点（质量连续下降分析用）。
type SealedScorePoint struct {
	InstrumentID int64
	BatchID      int64
	Score        float64
}

// SealedScoresByInstrument 按仪器与批次正序取全部已封存评分。
func (r *QueryRepo) SealedScoresByInstrument(ctx context.Context, q sqlite.Querier) ([]SealedScorePoint, error) {
	rows, err := q.QueryContext(ctx,
		`SELECT b.instrument_id, b.id, m.score
		 FROM quality_metrics m
		 JOIN observation_batches b ON b.id = m.batch_id
		 WHERE m.sealed = 1
		 ORDER BY b.instrument_id, b.id, m.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SealedScorePoint
	for rows.Next() {
		var p SealedScorePoint
		if err := rows.Scan(&p.InstrumentID, &p.BatchID, &p.Score); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// PendingRetestRow 为待复测批次行。
type PendingRetestRow struct {
	BatchID      int64     `json:"batch_id"`
	WindowID     int64     `json:"window_id"`
	TargetID     int64     `json:"target_id"`
	InstrumentID int64     `json:"instrument_id"`
	StartedAt    time.Time `json:"started_at"`
}

// PendingRetests 查询处于隔离状态且无有效复测批次的原始批次。
func (r *QueryRepo) PendingRetests(ctx context.Context, q sqlite.Querier, page Page) ([]PendingRetestRow, error) {
	page = page.Normalize()
	rows, err := q.QueryContext(ctx,
		`SELECT b.id, b.window_id, b.target_id, b.instrument_id, b.started_at
		 FROM observation_batches b
		 WHERE b.status='isolated' AND b.id > ?
		 AND NOT EXISTS (
			SELECT 1 FROM observation_batches rt
			WHERE rt.retest_of_id = b.id AND rt.status IN ('acquiring','frozen','isolated','archived'))
		 ORDER BY b.id LIMIT ?`, page.Cursor, page.Limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []PendingRetestRow
	for rows.Next() {
		var row PendingRetestRow
		var started string
		if err := rows.Scan(&row.BatchID, &row.WindowID, &row.TargetID, &row.InstrumentID, &started); err != nil {
			return nil, err
		}
		if row.StartedAt, err = parseTime(started); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

// ExpiredReleaseRow 为已过期发布许可行。
type ExpiredReleaseRow struct {
	PermitID    int64     `json:"permit_id"`
	ArchiveID   int64     `json:"archive_id"`
	Title       string    `json:"title"`
	Status      string    `json:"status"`
	SubmittedBy string    `json:"submitted_by"`
	ExpiresAt   time.Time `json:"expires_at"`
}

// ExpiredReleases 查询过期未完成闭环（pending_review/approved/published）的发布许可。
func (r *QueryRepo) ExpiredReleases(ctx context.Context, q sqlite.Querier,
	now time.Time, page Page) ([]ExpiredReleaseRow, error) {
	page = page.Normalize()
	rows, err := q.QueryContext(ctx,
		`SELECT id, archive_id, title, status, submitted_by, expires_at
		 FROM release_permits
		 WHERE expires_at < ? AND status IN ('pending_review','approved','published') AND id > ?
		 ORDER BY id LIMIT ?`, ts(now), page.Cursor, page.Limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ExpiredReleaseRow
	for rows.Next() {
		var row ExpiredReleaseRow
		var expires string
		if err := rows.Scan(&row.PermitID, &row.ArchiveID, &row.Title, &row.Status, &row.SubmittedBy, &expires); err != nil {
			return nil, err
		}
		if row.ExpiresAt, err = parseTime(expires); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}
