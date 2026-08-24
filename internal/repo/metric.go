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

// MetricRepo 为质量指标仓储。
type MetricRepo struct{}

// NewMetricRepo 创建指标仓储。
func NewMetricRepo() *MetricRepo { return &MetricRepo{} }

// NextVersion 取批次下一个指标版本号。
func (r *MetricRepo) NextVersion(ctx context.Context, q sqlite.Querier, batchID int64) (int, error) {
	var max sql.NullInt64
	err := q.QueryRowContext(ctx,
		`SELECT MAX(version_no) FROM quality_metrics WHERE batch_id=?`, batchID).Scan(&max)
	if err != nil {
		return 0, err
	}
	return int(max.Int64) + 1, nil
}

// HasSealed 判断批次是否已有封存指标。
func (r *MetricRepo) HasSealed(ctx context.Context, q sqlite.Querier, batchID int64) (bool, error) {
	var n int
	err := q.QueryRowContext(ctx,
		`SELECT COUNT(1) FROM quality_metrics WHERE batch_id=? AND sealed=1`, batchID).Scan(&n)
	return n > 0, err
}

// Create 插入指标版本。
func (r *MetricRepo) Create(ctx context.Context, q sqlite.Querier, m *model.QualityMetric, now time.Time) error {
	res, err := q.ExecContext(ctx,
		`INSERT INTO quality_metrics(batch_id,version_no,snr,fwhm,background,score,passed,sealed,computed_by,computed_at,sealed_at,created_at)
		 VALUES(?,?,?,?,?,?,?,0,?,?,NULL,?)`,
		m.BatchID, m.VersionNo, m.SNR, m.FWHM, m.Background, m.Score, m.Passed,
		m.ComputedBy, ts(m.ComputedAt), ts(now))
	if err != nil {
		return err
	}
	m.ID, err = lastID(res)
	m.CreatedAt = now
	return err
}

func scanMetric(row interface{ Scan(...any) error }) (*model.QualityMetric, error) {
	var m model.QualityMetric
	var computed, created string
	var sealedAt *string
	var passed, sealed int
	err := row.Scan(&m.ID, &m.BatchID, &m.VersionNo, &m.SNR, &m.FWHM, &m.Background,
		&m.Score, &passed, &sealed, &m.ComputedBy, &computed, &sealedAt, &created)
	if err != nil {
		return nil, err
	}
	m.Passed = passed == 1
	m.Sealed = sealed == 1
	if m.ComputedAt, err = parseTime(computed); err != nil {
		return nil, err
	}
	if m.SealedAt, err = parseTimePtr(sealedAt); err != nil {
		return nil, err
	}
	if m.CreatedAt, err = parseTime(created); err != nil {
		return nil, err
	}
	return &m, nil
}

// Get 按 id 取指标。
func (r *MetricRepo) Get(ctx context.Context, q sqlite.Querier, id int64) (*model.QualityMetric, error) {
	row := q.QueryRowContext(ctx,
		`SELECT id,batch_id,version_no,snr,fwhm,background,score,passed,sealed,computed_by,computed_at,sealed_at,created_at
		 FROM quality_metrics WHERE id=?`, id)
	m, err := scanMetric(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, apperr.NotFound("质量指标", id)
	}
	return m, err
}

// ListByBatch 分页列出批次指标版本。
func (r *MetricRepo) ListByBatch(ctx context.Context, q sqlite.Querier, batchID int64, page Page) ([]model.QualityMetric, error) {
	page = page.Normalize()
	rows, err := q.QueryContext(ctx,
		`SELECT id,batch_id,version_no,snr,fwhm,background,score,passed,sealed,computed_by,computed_at,sealed_at,created_at
		 FROM quality_metrics WHERE batch_id=? AND id > ? ORDER BY id LIMIT ?`,
		batchID, page.Cursor, page.Limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.QualityMetric
	for rows.Next() {
		m, err := scanMetric(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *m)
	}
	return out, rows.Err()
}

// Seal 封存指标（封存后不可变；仅允许未封存指标调用一次）。
func (r *MetricRepo) Seal(ctx context.Context, q sqlite.Querier, id int64, now time.Time) error {
	res, err := q.ExecContext(ctx,
		`UPDATE quality_metrics SET sealed=1, sealed_at=? WHERE id=? AND sealed=0`, ts(now), id)
	if err != nil {
		return err
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return err
	}
	return resolveOptimistic(ctx, q, "quality_metrics", "质量指标", id, rows)
}
