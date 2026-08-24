package repo

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"observatory/internal/apperr"
	"observatory/internal/model"
	"observatory/internal/store/sqlite"
)

// WindowRepo 为观测窗口仓储。
type WindowRepo struct{}

// NewWindowRepo 创建窗口仓储。
func NewWindowRepo() *WindowRepo { return &WindowRepo{} }

// Create 插入窗口（申请）。
func (r *WindowRepo) Create(ctx context.Context, q sqlite.Querier, w *model.ObservationWindow, now time.Time) error {
	res, err := q.ExecContext(ctx,
		`INSERT INTO observation_windows(instrument_id,plan_id,title,start_at,end_at,status,frozen_snapshot,applied_by,approved_by,version,created_at,updated_at)
		 VALUES(?,?,?,?,?,?,'',?,'',1,?,?)`,
		w.InstrumentID, w.PlanID, w.Title, ts(w.StartAt), ts(w.EndAt), w.Status, w.AppliedBy, ts(now), ts(now))
	if err != nil {
		return err
	}
	w.ID, err = lastID(res)
	w.Version = 1
	w.CreatedAt, w.UpdatedAt = now, now
	return err
}

func scanWindow(row interface{ Scan(...any) error }) (*model.ObservationWindow, error) {
	var w model.ObservationWindow
	var startAt, endAt, created, updated string
	err := row.Scan(&w.ID, &w.InstrumentID, &w.PlanID, &w.Title, &startAt, &endAt,
		&w.Status, &w.FrozenSnapshot, &w.AppliedBy, &w.ApprovedBy, &w.Version, &created, &updated)
	if err != nil {
		return nil, err
	}
	if w.StartAt, err = parseTime(startAt); err != nil {
		return nil, err
	}
	if w.EndAt, err = parseTime(endAt); err != nil {
		return nil, err
	}
	if w.CreatedAt, err = parseTime(created); err != nil {
		return nil, err
	}
	if w.UpdatedAt, err = parseTime(updated); err != nil {
		return nil, err
	}
	return &w, nil
}

// Get 按 id 取窗口。
func (r *WindowRepo) Get(ctx context.Context, q sqlite.Querier, id int64) (*model.ObservationWindow, error) {
	row := q.QueryRowContext(ctx,
		`SELECT id,instrument_id,plan_id,title,start_at,end_at,status,frozen_snapshot,applied_by,approved_by,version,created_at,updated_at
		 FROM observation_windows WHERE id=?`, id)
	w, err := scanWindow(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, apperr.NotFound("观测窗口", id)
	}
	return w, err
}

// List 键集分页列出窗口，可按仪器与状态过滤。
func (r *WindowRepo) List(ctx context.Context, q sqlite.Querier, instrumentID int64, status string, page Page) ([]model.ObservationWindow, error) {
	page = page.Normalize()
	query := `SELECT id,instrument_id,plan_id,title,start_at,end_at,status,frozen_snapshot,applied_by,approved_by,version,created_at,updated_at
		FROM observation_windows WHERE id > ?`
	args := []any{page.Cursor}
	if instrumentID > 0 {
		query += ` AND instrument_id = ?`
		args = append(args, instrumentID)
	}
	if status != "" {
		query += ` AND status = ?`
		args = append(args, status)
	}
	query += ` ORDER BY id LIMIT ?`
	args = append(args, page.Limit)
	rows, err := q.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.ObservationWindow
	for rows.Next() {
		w, err := scanWindow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *w)
	}
	return out, rows.Err()
}

// Approve 乐观锁批准窗口：绑定校准方案并写入冻结快照。
func (r *WindowRepo) Approve(ctx context.Context, q sqlite.Querier, id, version, planID int64,
	snapshot, approver string, now time.Time) error {
	return execOptimistic(ctx, q, "observation_windows", "观测窗口", id,
		`UPDATE observation_windows SET status='approved', plan_id=?, frozen_snapshot=?, approved_by=?,
		 version=version+1, updated_at=? WHERE id=? AND version=?`,
		planID, snapshot, approver, ts(now), id, version)
}

// UpdateStatus 乐观锁更新窗口状态。
func (r *WindowRepo) UpdateStatus(ctx context.Context, q sqlite.Querier, id, version int64, status string, now time.Time) error {
	return execOptimistic(ctx, q, "observation_windows", "观测窗口", id,
		`UPDATE observation_windows SET status=?, version=version+1, updated_at=? WHERE id=? AND version=?`,
		status, ts(now), id, version)
}

// HasOpenForInstrument 判断仪器是否存在已批准/已激活窗口（冻结判定）。
func (r *WindowRepo) HasOpenForInstrument(ctx context.Context, q sqlite.Querier, instrumentID int64) (bool, error) {
	var n int
	err := q.QueryRowContext(ctx,
		`SELECT COUNT(1) FROM observation_windows
		 WHERE instrument_id=? AND status IN ('approved','active')`, instrumentID).Scan(&n)
	return n > 0, err
}

// HasOpenOverlap 判断仪器是否存在与 [start,end] 重叠的已批准/已激活窗口（排除自身）。
func (r *WindowRepo) HasOpenOverlap(ctx context.Context, q sqlite.Querier, instrumentID, excludeID int64,
	start, end time.Time) (bool, error) {
	var n int
	err := q.QueryRowContext(ctx,
		`SELECT COUNT(1) FROM observation_windows
		 WHERE instrument_id=? AND id<>? AND status IN ('approved','active')
		 AND start_at < ? AND end_at > ?`,
		instrumentID, excludeID, ts(end), ts(start)).Scan(&n)
	return n > 0, err
}

// HasActiveOverlap 判断仪器是否存在与 [start,end] 重叠的已激活窗口（排除自身）。
func (r *WindowRepo) HasActiveOverlap(ctx context.Context, q sqlite.Querier, instrumentID, excludeID int64,
	start, end time.Time) (bool, error) {
	var n int
	err := q.QueryRowContext(ctx,
		`SELECT COUNT(1) FROM observation_windows
		 WHERE instrument_id=? AND id<>? AND status='active'
		 AND start_at < ? AND end_at > ?`,
		instrumentID, excludeID, ts(end), ts(start)).Scan(&n)
	return n > 0, err
}
