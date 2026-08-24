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

// ReleaseRepo 为发布许可仓储。
type ReleaseRepo struct{}

// NewReleaseRepo 创建发布许可仓储。
func NewReleaseRepo() *ReleaseRepo { return &ReleaseRepo{} }

// Create 插入发布许可申请。
func (r *ReleaseRepo) Create(ctx context.Context, q sqlite.Querier, p *model.ReleasePermit, now time.Time) error {
	res, err := q.ExecContext(ctx,
		`INSERT INTO release_permits(archive_id,title,status,submitted_by,reviewed_by,submitted_at,reviewed_at,published_at,expires_at,version,created_at,updated_at)
		 VALUES(?,?,?,?,'',?,NULL,NULL,?,1,?,?)`,
		p.ArchiveID, p.Title, p.Status, p.SubmittedBy, ts(p.SubmittedAt), ts(p.ExpiresAt), ts(now), ts(now))
	if err != nil {
		return err
	}
	p.ID, err = lastID(res)
	p.Version = 1
	p.CreatedAt, p.UpdatedAt = now, now
	return err
}

func scanPermit(row interface{ Scan(...any) error }) (*model.ReleasePermit, error) {
	var p model.ReleasePermit
	var submitted, expires, created, updated string
	var reviewed, published *string
	err := row.Scan(&p.ID, &p.ArchiveID, &p.Title, &p.Status, &p.SubmittedBy, &p.ReviewedBy,
		&submitted, &reviewed, &published, &expires, &p.Version, &created, &updated)
	if err != nil {
		return nil, err
	}
	if p.SubmittedAt, err = parseTime(submitted); err != nil {
		return nil, err
	}
	if p.ReviewedAt, err = parseTimePtr(reviewed); err != nil {
		return nil, err
	}
	if p.PublishedAt, err = parseTimePtr(published); err != nil {
		return nil, err
	}
	if p.ExpiresAt, err = parseTime(expires); err != nil {
		return nil, err
	}
	if p.CreatedAt, err = parseTime(created); err != nil {
		return nil, err
	}
	if p.UpdatedAt, err = parseTime(updated); err != nil {
		return nil, err
	}
	return &p, nil
}

const permitCols = `id,archive_id,title,status,submitted_by,reviewed_by,submitted_at,reviewed_at,published_at,expires_at,version,created_at,updated_at`

// Get 按 id 取发布许可。
func (r *ReleaseRepo) Get(ctx context.Context, q sqlite.Querier, id int64) (*model.ReleasePermit, error) {
	row := q.QueryRowContext(ctx,
		`SELECT `+permitCols+` FROM release_permits WHERE id=?`, id)
	p, err := scanPermit(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, apperr.NotFound("发布许可", id)
	}
	return p, err
}

// List 分页列出发布许可，可按状态过滤。
func (r *ReleaseRepo) List(ctx context.Context, q sqlite.Querier, status string, page Page) ([]model.ReleasePermit, error) {
	page = page.Normalize()
	query := `SELECT ` + permitCols + ` FROM release_permits WHERE id > ?`
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
	var out []model.ReleasePermit
	for rows.Next() {
		p, err := scanPermit(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *p)
	}
	return out, rows.Err()
}

// Review 乐观锁写入复核结论。
func (r *ReleaseRepo) Review(ctx context.Context, q sqlite.Querier, id, version int64, status, reviewer string, now time.Time) error {
	return execOptimistic(ctx, q, "release_permits", "发布许可", id,
		`UPDATE release_permits SET status=?, reviewed_by=?, reviewed_at=?, version=version+1, updated_at=?
		 WHERE id=? AND version=?`,
		status, reviewer, ts(now), ts(now), id, version)
}

// Publish 乐观锁发布许可。
func (r *ReleaseRepo) Publish(ctx context.Context, q sqlite.Querier, id, version int64, now time.Time) error {
	return execOptimistic(ctx, q, "release_permits", "发布许可", id,
		`UPDATE release_permits SET status='published', published_at=?, version=version+1, updated_at=?
		 WHERE id=? AND version=?`,
		ts(now), ts(now), id, version)
}

// UpdateStatus 乐观锁更新许可状态（撤销/过期等）。
func (r *ReleaseRepo) UpdateStatus(ctx context.Context, q sqlite.Querier, id, version int64, status string, now time.Time) error {
	return execOptimistic(ctx, q, "release_permits", "发布许可", id,
		`UPDATE release_permits SET status=?, version=version+1, updated_at=? WHERE id=? AND version=?`,
		status, ts(now), id, version)
}
