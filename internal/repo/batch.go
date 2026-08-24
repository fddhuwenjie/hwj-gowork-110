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

// BatchRepo 为原始观测批次仓储。
type BatchRepo struct{}

// NewBatchRepo 创建批次仓储。
func NewBatchRepo() *BatchRepo { return &BatchRepo{} }

// Create 插入批次；幂等键冲突返回已存在批次且 replay=true。
func (r *BatchRepo) Create(ctx context.Context, q sqlite.Querier, b *model.ObservationBatch, now time.Time) (replay bool, err error) {
	res, err := q.ExecContext(ctx,
		`INSERT INTO observation_batches(window_id,target_id,instrument_id,retest_of_id,object_uri,status,started_at,finished_at,idempotency_key,version,created_at,updated_at)
		 VALUES(?,?,?,?,?,?,?,NULL,?,1,?,?)`,
		b.WindowID, b.TargetID, b.InstrumentID, b.RetestOfID, b.ObjectURI, b.Status,
		ts(b.StartedAt), b.IdempotencyKey, ts(now), ts(now))
	if err == nil {
		b.ID, err = lastID(res)
		b.Version = 1
		b.CreatedAt, b.UpdatedAt = now, now
		return false, err
	}
	if !IsUniqueViolation(err) {
		return false, err
	}
	existing, gerr := r.GetByKey(ctx, q, b.IdempotencyKey)
	if gerr != nil {
		return false, gerr
	}
	*b = *existing
	return true, nil
}

func scanBatch(row interface{ Scan(...any) error }) (*model.ObservationBatch, error) {
	var b model.ObservationBatch
	var started, created, updated string
	var finished *string
	err := row.Scan(&b.ID, &b.WindowID, &b.TargetID, &b.InstrumentID, &b.RetestOfID,
		&b.ObjectURI, &b.Status, &started, &finished, &b.IdempotencyKey, &b.Version, &created, &updated)
	if err != nil {
		return nil, err
	}
	if b.StartedAt, err = parseTime(started); err != nil {
		return nil, err
	}
	if b.FinishedAt, err = parseTimePtr(finished); err != nil {
		return nil, err
	}
	if b.CreatedAt, err = parseTime(created); err != nil {
		return nil, err
	}
	if b.UpdatedAt, err = parseTime(updated); err != nil {
		return nil, err
	}
	return &b, nil
}

// Get 按 id 取批次。
func (r *BatchRepo) Get(ctx context.Context, q sqlite.Querier, id int64) (*model.ObservationBatch, error) {
	row := q.QueryRowContext(ctx,
		`SELECT id,window_id,target_id,instrument_id,retest_of_id,object_uri,status,started_at,finished_at,idempotency_key,version,created_at,updated_at
		 FROM observation_batches WHERE id=?`, id)
	b, err := scanBatch(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, apperr.NotFound("观测批次", id)
	}
	return b, err
}

// GetByKey 按幂等键取批次。
func (r *BatchRepo) GetByKey(ctx context.Context, q sqlite.Querier, key string) (*model.ObservationBatch, error) {
	row := q.QueryRowContext(ctx,
		`SELECT id,window_id,target_id,instrument_id,retest_of_id,object_uri,status,started_at,finished_at,idempotency_key,version,created_at,updated_at
		 FROM observation_batches WHERE idempotency_key=?`, key)
	return scanBatch(row)
}

// ListByWindow 分页列出窗口批次。
func (r *BatchRepo) ListByWindow(ctx context.Context, q sqlite.Querier, windowID int64, page Page) ([]model.ObservationBatch, error) {
	page = page.Normalize()
	rows, err := q.QueryContext(ctx,
		`SELECT id,window_id,target_id,instrument_id,retest_of_id,object_uri,status,started_at,finished_at,idempotency_key,version,created_at,updated_at
		 FROM observation_batches WHERE window_id=? AND id > ? ORDER BY id LIMIT ?`,
		windowID, page.Cursor, page.Limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.ObservationBatch
	for rows.Next() {
		b, err := scanBatch(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *b)
	}
	return out, rows.Err()
}

// UpdateStatus 乐观锁更新批次状态。
func (r *BatchRepo) UpdateStatus(ctx context.Context, q sqlite.Querier, id, version int64, status string, now time.Time) error {
	return execOptimistic(ctx, q, "observation_batches", "观测批次", id,
		`UPDATE observation_batches SET status=?, version=version+1, updated_at=? WHERE id=? AND version=?`,
		status, ts(now), id, version)
}

// Finish 乐观锁结束批次（冻结/隔离时写入 finished_at）。
func (r *BatchRepo) Finish(ctx context.Context, q sqlite.Querier, id, version int64, status string, now time.Time) error {
	return execOptimistic(ctx, q, "observation_batches", "观测批次", id,
		`UPDATE observation_batches SET status=?, finished_at=?, version=version+1, updated_at=?
		 WHERE id=? AND version=?`,
		status, ts(now), ts(now), id, version)
}

// LatestByTarget 取目标最新批次。
func (r *BatchRepo) LatestByTarget(ctx context.Context, q sqlite.Querier, targetID int64) (*model.ObservationBatch, error) {
	row := q.QueryRowContext(ctx,
		`SELECT id,window_id,target_id,instrument_id,retest_of_id,object_uri,status,started_at,finished_at,idempotency_key,version,created_at,updated_at
		 FROM observation_batches WHERE target_id=? ORDER BY id DESC LIMIT 1`, targetID)
	return scanBatch(row)
}

// LatestIsolatedByTarget 取目标最新的隔离批次；无则返回 not_found。
func (r *BatchRepo) LatestIsolatedByTarget(ctx context.Context, q sqlite.Querier, targetID int64) (*model.ObservationBatch, error) {
	row := q.QueryRowContext(ctx,
		`SELECT id,window_id,target_id,instrument_id,retest_of_id,object_uri,status,started_at,finished_at,idempotency_key,version,created_at,updated_at
		 FROM observation_batches WHERE target_id=? AND status='isolated' ORDER BY id DESC LIMIT 1`, targetID)
	b, err := scanBatch(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, apperr.NotFound("隔离批次", targetID)
	}
	return b, err
}

// AcquiringByInstrument 取仪器当前处于 acquiring 的批次（越界读数隔离用）。
func (r *BatchRepo) AcquiringByInstrument(ctx context.Context, q sqlite.Querier, instrumentID int64) ([]model.ObservationBatch, error) {
	rows, err := q.QueryContext(ctx,
		`SELECT id,window_id,target_id,instrument_id,retest_of_id,object_uri,status,started_at,finished_at,idempotency_key,version,created_at,updated_at
		 FROM observation_batches WHERE instrument_id=? AND status='acquiring' ORDER BY id`, instrumentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.ObservationBatch
	for rows.Next() {
		b, err := scanBatch(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *b)
	}
	return out, rows.Err()
}
