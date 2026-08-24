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

// SiteRepo 为站点仓储。
type SiteRepo struct{}

// NewSiteRepo 创建站点仓储。
func NewSiteRepo() *SiteRepo { return &SiteRepo{} }

// Create 插入站点。
func (r *SiteRepo) Create(ctx context.Context, q sqlite.Querier, s *model.Site, now time.Time) error {
	res, err := q.ExecContext(ctx,
		`INSERT INTO sites(code,name,latitude,longitude,altitude_m,status,version,created_at,updated_at)
		 VALUES(?,?,?,?,?,?,1,?,?)`,
		s.Code, s.Name, s.Latitude, s.Longitude, s.AltitudeM, s.Status, ts(now), ts(now))
	if err != nil {
		if IsUniqueViolation(err) {
			return apperr.New(apperr.CodeDuplicate, "站点编码已存在").WithDetail("code", s.Code)
		}
		return err
	}
	s.ID, err = lastID(res)
	s.Version = 1
	s.CreatedAt, s.UpdatedAt = now, now
	return err
}

// scanSite 扫描站点行。
func scanSite(row interface{ Scan(...any) error }) (*model.Site, error) {
	var s model.Site
	var created, updated string
	err := row.Scan(&s.ID, &s.Code, &s.Name, &s.Latitude, &s.Longitude, &s.AltitudeM,
		&s.Status, &s.Version, &created, &updated)
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

// Get 按 id 取站点。
func (r *SiteRepo) Get(ctx context.Context, q sqlite.Querier, id int64) (*model.Site, error) {
	row := q.QueryRowContext(ctx,
		`SELECT id,code,name,latitude,longitude,altitude_m,status,version,created_at,updated_at
		 FROM sites WHERE id=?`, id)
	s, err := scanSite(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, apperr.NotFound("站点", id)
	}
	return s, err
}

// List 键集分页列出站点，可按状态过滤。
func (r *SiteRepo) List(ctx context.Context, q sqlite.Querier, status string, page Page) ([]model.Site, error) {
	page = page.Normalize()
	query := `SELECT id,code,name,latitude,longitude,altitude_m,status,version,created_at,updated_at
		FROM sites WHERE id > ?`
	args := []any{page.Cursor}
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
	var out []model.Site
	for rows.Next() {
		s, err := scanSite(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *s)
	}
	return out, rows.Err()
}

// Update 乐观锁更新站点基本信息。
func (r *SiteRepo) Update(ctx context.Context, q sqlite.Querier, s *model.Site, now time.Time) error {
	err := execOptimistic(ctx, q, "sites", "站点", s.ID,
		`UPDATE sites SET name=?, latitude=?, longitude=?, altitude_m=?, version=version+1, updated_at=?
		 WHERE id=? AND version=?`,
		s.Name, s.Latitude, s.Longitude, s.AltitudeM, ts(now), s.ID, s.Version)
	if err != nil {
		return err
	}
	s.Version++
	s.UpdatedAt = now
	return nil
}

// UpdateStatus 乐观锁更新站点状态。
func (r *SiteRepo) UpdateStatus(ctx context.Context, q sqlite.Querier, id, version int64, status string, now time.Time) error {
	return execOptimistic(ctx, q, "sites", "站点", id,
		`UPDATE sites SET status=?, version=version+1, updated_at=? WHERE id=? AND version=?`,
		status, ts(now), id, version)
}
