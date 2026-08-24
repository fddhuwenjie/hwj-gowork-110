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

// SourceRepo 为标准源仓储。
type SourceRepo struct{}

// NewSourceRepo 创建标准源仓储。
func NewSourceRepo() *SourceRepo { return &SourceRepo{} }

// Create 插入标准源。
func (r *SourceRepo) Create(ctx context.Context, q sqlite.Querier, s *model.StandardSource, now time.Time) error {
	res, err := q.ExecContext(ctx,
		`INSERT INTO standard_sources(code,name,flux_jy,spectrum,status,version,created_at,updated_at)
		 VALUES(?,?,?,?,?,1,?,?)`,
		s.Code, s.Name, s.FluxJy, s.Spectrum, s.Status, ts(now), ts(now))
	if err != nil {
		if IsUniqueViolation(err) {
			return apperr.New(apperr.CodeDuplicate, "标准源编码已存在").WithDetail("code", s.Code)
		}
		return err
	}
	s.ID, err = lastID(res)
	s.Version = 1
	s.CreatedAt, s.UpdatedAt = now, now
	return err
}

func scanSource(row interface{ Scan(...any) error }) (*model.StandardSource, error) {
	var s model.StandardSource
	var created, updated string
	err := row.Scan(&s.ID, &s.Code, &s.Name, &s.FluxJy, &s.Spectrum, &s.Status,
		&s.Version, &created, &updated)
	if err != nil {
		return nil, err
	}
	if s.CreatedAt, err = parseTime(created); err != nil {
		return nil, err
	}
	if s.UpdatedAt, err = parseTime(updated); err != nil {
		return nil, err
	}
	return &s, nil
}

// Get 按 id 取标准源。
func (r *SourceRepo) Get(ctx context.Context, q sqlite.Querier, id int64) (*model.StandardSource, error) {
	row := q.QueryRowContext(ctx,
		`SELECT id,code,name,flux_jy,spectrum,status,version,created_at,updated_at
		 FROM standard_sources WHERE id=?`, id)
	s, err := scanSource(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, apperr.NotFound("标准源", id)
	}
	return s, err
}

// List 分页列出标准源。
func (r *SourceRepo) List(ctx context.Context, q sqlite.Querier, page Page) ([]model.StandardSource, error) {
	page = page.Normalize()
	rows, err := q.QueryContext(ctx,
		`SELECT id,code,name,flux_jy,spectrum,status,version,created_at,updated_at
		 FROM standard_sources WHERE id > ? ORDER BY id LIMIT ?`, page.Cursor, page.Limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.StandardSource
	for rows.Next() {
		s, err := scanSource(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *s)
	}
	return out, rows.Err()
}

// UpdateStatus 乐观锁更新标准源状态。
func (r *SourceRepo) UpdateStatus(ctx context.Context, q sqlite.Querier, id, version int64, status string, now time.Time) error {
	return execOptimistic(ctx, q, "standard_sources", "标准源", id,
		`UPDATE standard_sources SET status=?, version=version+1, updated_at=? WHERE id=? AND version=?`,
		status, ts(now), id, version)
}
