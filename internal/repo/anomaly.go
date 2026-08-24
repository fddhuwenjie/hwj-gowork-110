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

// AnomalyRepo 为异常处置仓储。
type AnomalyRepo struct{}

// NewAnomalyRepo 创建异常仓储。
func NewAnomalyRepo() *AnomalyRepo { return &AnomalyRepo{} }

// Create 登记异常。关联批次的异常以批次所属仪器为准；独立异常保留经服务层校验的仪器。
func (r *AnomalyRepo) Create(ctx context.Context, q sqlite.Querier, a *model.Anomaly, now time.Time) error {
	res, err := q.ExecContext(ctx,
		`INSERT INTO anomalies(batch_id,instrument_id,kind,description,status,opened_by,resolved_by,created_at,updated_at)
		 VALUES(?,?,?,?,?,?,'',?,?)`,
		a.BatchID, a.InstrumentID, a.Kind, a.Description, a.Status, a.OpenedBy, ts(now), ts(now))
	if err != nil {
		return err
	}
	a.ID, err = lastID(res)
	a.CreatedAt, a.UpdatedAt = now, now
	return err
}

func scanAnomaly(row interface{ Scan(...any) error }) (*model.Anomaly, error) {
	var a model.Anomaly
	var created, updated string
	err := row.Scan(&a.ID, &a.BatchID, &a.InstrumentID, &a.Kind, &a.Description,
		&a.Status, &a.OpenedBy, &a.ResolvedBy, &created, &updated)
	if err != nil {
		return nil, err
	}
	if a.CreatedAt, err = parseTime(created); err != nil {
		return nil, err
	}
	if a.UpdatedAt, err = parseTime(updated); err != nil {
		return nil, err
	}
	return &a, nil
}

// Get 按 id 取异常。
func (r *AnomalyRepo) Get(ctx context.Context, q sqlite.Querier, id int64) (*model.Anomaly, error) {
	row := q.QueryRowContext(ctx,
		`SELECT id,batch_id,instrument_id,kind,description,status,opened_by,resolved_by,created_at,updated_at
		 FROM anomalies WHERE id=?`, id)
	a, err := scanAnomaly(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, apperr.NotFound("异常", id)
	}
	return a, err
}

// List 分页列出异常，可按仪器与状态过滤。
func (r *AnomalyRepo) List(ctx context.Context, q sqlite.Querier, instrumentID int64, status string, page Page) ([]model.Anomaly, error) {
	page = page.Normalize()
	query := `SELECT id,batch_id,instrument_id,kind,description,status,opened_by,resolved_by,created_at,updated_at
		FROM anomalies WHERE id > ?`
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
	var out []model.Anomaly
	for rows.Next() {
		a, err := scanAnomaly(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *a)
	}
	return out, rows.Err()
}

// UpdateStatus 乐观锁更新异常状态。
func (r *AnomalyRepo) UpdateStatus(ctx context.Context, q sqlite.Querier, id int64, status, resolvedBy string, now time.Time) error {
	res, err := q.ExecContext(ctx,
		`UPDATE anomalies SET status=?, resolved_by=?, updated_at=? WHERE id=?`,
		status, resolvedBy, ts(now), id)
	if err != nil {
		return err
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return err
	}
	return resolveOptimistic(ctx, q, "anomalies", "异常", id, rows)
}

// OpenByBatch 取批次下处于 open 状态的异常。
func (r *AnomalyRepo) OpenByBatch(ctx context.Context, q sqlite.Querier, batchID int64) ([]model.Anomaly, error) {
	rows, err := q.QueryContext(ctx,
		`SELECT id,batch_id,instrument_id,kind,description,status,opened_by,resolved_by,created_at,updated_at
		 FROM anomalies WHERE batch_id=? AND status='open' ORDER BY id`, batchID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.Anomaly
	for rows.Next() {
		a, err := scanAnomaly(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *a)
	}
	return out, rows.Err()
}
