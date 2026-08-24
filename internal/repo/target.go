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

// TargetRepo 为目标计划与曝光序列仓储。
type TargetRepo struct{}

// NewTargetRepo 创建目标仓储。
func NewTargetRepo() *TargetRepo { return &TargetRepo{} }

// Create 插入目标；幂等键冲突返回已存在目标且 replay=true。
func (r *TargetRepo) Create(ctx context.Context, q sqlite.Querier, t *model.Target, now time.Time) (replay bool, err error) {
	res, err := q.ExecContext(ctx,
		`INSERT INTO targets(window_id,name,ra_deg,dec_deg,priority,exposure_goal,status,idempotency_key,version,created_at,updated_at)
		 VALUES(?,?,?,?,?,?,?,?,1,?,?)`,
		t.WindowID, t.Name, t.RaDeg, t.DecDeg, t.Priority, t.ExposureGoal, t.Status, t.IdempotencyKey, ts(now), ts(now))
	if err == nil {
		t.ID, err = lastID(res)
		t.Version = 1
		t.CreatedAt, t.UpdatedAt = now, now
		return false, err
	}
	if !IsUniqueViolation(err) {
		return false, err
	}
	existing, gerr := r.GetByKey(ctx, q, t.WindowID, t.IdempotencyKey)
	if gerr != nil {
		return false, apperr.New(apperr.CodeDuplicate, "目标名称在该窗口内已存在").WithDetail("name", t.Name)
	}
	*t = *existing
	return true, nil
}

func scanTarget(row interface{ Scan(...any) error }) (*model.Target, error) {
	var t model.Target
	var created, updated string
	err := row.Scan(&t.ID, &t.WindowID, &t.Name, &t.RaDeg, &t.DecDeg, &t.Priority,
		&t.ExposureGoal, &t.Status, &t.IdempotencyKey, &t.Version, &created, &updated)
	if err != nil {
		return nil, err
	}
	if t.CreatedAt, err = parseTime(created); err != nil {
		return nil, err
	}
	if t.UpdatedAt, err = parseTime(updated); err != nil {
		return nil, err
	}
	return &t, nil
}

// Get 按 id 取目标。
func (r *TargetRepo) Get(ctx context.Context, q sqlite.Querier, id int64) (*model.Target, error) {
	row := q.QueryRowContext(ctx,
		`SELECT id,window_id,name,ra_deg,dec_deg,priority,exposure_goal,status,idempotency_key,version,created_at,updated_at
		 FROM targets WHERE id=?`, id)
	t, err := scanTarget(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, apperr.NotFound("目标", id)
	}
	return t, err
}

// GetByKey 按窗口与幂等键取目标。
func (r *TargetRepo) GetByKey(ctx context.Context, q sqlite.Querier, windowID int64, key string) (*model.Target, error) {
	row := q.QueryRowContext(ctx,
		`SELECT id,window_id,name,ra_deg,dec_deg,priority,exposure_goal,status,idempotency_key,version,created_at,updated_at
		 FROM targets WHERE window_id=? AND idempotency_key=?`, windowID, key)
	return scanTarget(row)
}

// ListByWindow 分页列出窗口内目标。
func (r *TargetRepo) ListByWindow(ctx context.Context, q sqlite.Querier, windowID int64, page Page) ([]model.Target, error) {
	page = page.Normalize()
	rows, err := q.QueryContext(ctx,
		`SELECT id,window_id,name,ra_deg,dec_deg,priority,exposure_goal,status,idempotency_key,version,created_at,updated_at
		 FROM targets WHERE window_id=? AND id > ? ORDER BY id LIMIT ?`,
		windowID, page.Cursor, page.Limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.Target
	for rows.Next() {
		t, err := scanTarget(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *t)
	}
	return out, rows.Err()
}

// ListAllByWindow 列出窗口全部目标（冻结快照用）。
func (r *TargetRepo) ListAllByWindow(ctx context.Context, q sqlite.Querier, windowID int64) ([]model.Target, error) {
	return r.ListByWindow(ctx, q, windowID, Page{Limit: 100})
}

// UpdateStatus 乐观锁更新目标状态。
func (r *TargetRepo) UpdateStatus(ctx context.Context, q sqlite.Querier, id, version int64, status string, now time.Time) error {
	return execOptimistic(ctx, q, "targets", "目标", id,
		`UPDATE targets SET status=?, version=version+1, updated_at=? WHERE id=? AND version=?`,
		status, ts(now), id, version)
}

// MaxExposureSeq 取目标当前最大曝光序号（无曝光时为 0）。
func (r *TargetRepo) MaxExposureSeq(ctx context.Context, q sqlite.Querier, targetID int64) (int64, error) {
	var max sql.NullInt64
	err := q.QueryRowContext(ctx,
		`SELECT MAX(seq_no) FROM target_exposures WHERE target_id=?`, targetID).Scan(&max)
	if err != nil {
		return 0, err
	}
	return max.Int64, nil
}

// CountExposures 统计目标曝光数。
func (r *TargetRepo) CountExposures(ctx context.Context, q sqlite.Querier, targetID int64) (int64, error) {
	var n int64
	err := q.QueryRowContext(ctx,
		`SELECT COUNT(1) FROM target_exposures WHERE target_id=?`, targetID).Scan(&n)
	return n, err
}

// InsertExposure 插入一条曝光（不可变）。
func (r *TargetRepo) InsertExposure(ctx context.Context, q sqlite.Querier, e *model.TargetExposure, now time.Time) error {
	res, err := q.ExecContext(ctx,
		`INSERT INTO target_exposures(target_id,seq_no,duration_s,filter,created_at) VALUES(?,?,?,?,?)`,
		e.TargetID, e.SeqNo, e.DurationS, e.Filter, ts(now))
	if err != nil {
		if IsUniqueViolation(err) {
			return apperr.New(apperr.CodeDuplicate, "曝光序号已存在，禁止重复或跳号").
				WithDetail("seq_no", e.SeqNo)
		}
		return err
	}
	e.ID, err = lastID(res)
	e.CreatedAt = now
	return err
}

// ListExposures 分页列出目标曝光。
func (r *TargetRepo) ListExposures(ctx context.Context, q sqlite.Querier, targetID int64, page Page) ([]model.TargetExposure, error) {
	page = page.Normalize()
	rows, err := q.QueryContext(ctx,
		`SELECT id,target_id,seq_no,duration_s,filter,created_at
		 FROM target_exposures WHERE target_id=? AND id > ? ORDER BY id LIMIT ?`,
		targetID, page.Cursor, page.Limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.TargetExposure
	for rows.Next() {
		var e model.TargetExposure
		var created string
		if err := rows.Scan(&e.ID, &e.TargetID, &e.SeqNo, &e.DurationS, &e.Filter, &created); err != nil {
			return nil, err
		}
		if e.CreatedAt, err = parseTime(created); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
