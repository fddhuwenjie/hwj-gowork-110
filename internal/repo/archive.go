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

// ArchiveRepo 为数据归档仓储；不提供任何物理删除方法。
type ArchiveRepo struct{}

// NewArchiveRepo 创建归档仓储。
func NewArchiveRepo() *ArchiveRepo { return &ArchiveRepo{} }

// Create 插入归档请求；幂等键或批次冲突返回已存在归档且 replay=true。
func (r *ArchiveRepo) Create(ctx context.Context, q sqlite.Querier, a *model.Archive, now time.Time) (replay bool, err error) {
	res, err := q.ExecContext(ctx,
		`INSERT INTO archives(batch_id,object_uri,checksum_sha256,size_bytes,status,idempotency_key,requested_by,verified_by,deleted_at,version,created_at,updated_at)
		 VALUES(?,?,?,?,?,?,?,'',NULL,1,?,?)`,
		a.BatchID, a.ObjectURI, a.ChecksumSHA256, a.SizeBytes, a.Status, a.IdempotencyKey, a.RequestedBy, ts(now), ts(now))
	if err == nil {
		a.ID, err = lastID(res)
		a.Version = 1
		a.CreatedAt, a.UpdatedAt = now, now
		return false, err
	}
	if !IsUniqueViolation(err) {
		return false, err
	}
	existing, gerr := r.GetByKey(ctx, q, a.IdempotencyKey)
	if gerr == nil {
		*a = *existing
		return true, nil
	}
	existing, gerr = r.GetByBatch(ctx, q, a.BatchID)
	if gerr != nil {
		return false, apperr.New(apperr.CodeDuplicate, "该批次已存在归档请求").WithDetail("batch_id", a.BatchID)
	}
	*a = *existing
	return true, nil
}

func scanArchive(row interface{ Scan(...any) error }) (*model.Archive, error) {
	var a model.Archive
	var created, updated string
	var deletedAt *string
	err := row.Scan(&a.ID, &a.BatchID, &a.ObjectURI, &a.ChecksumSHA256, &a.SizeBytes,
		&a.Status, &a.IdempotencyKey, &a.RequestedBy, &a.VerifiedBy, &deletedAt,
		&a.Version, &created, &updated)
	if err != nil {
		return nil, err
	}
	if a.DeletedAt, err = parseTimePtr(deletedAt); err != nil {
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

const archiveCols = `id,batch_id,object_uri,checksum_sha256,size_bytes,status,idempotency_key,requested_by,verified_by,deleted_at,version,created_at,updated_at`

// Get 按 id 取归档（默认排除已软删除）。
func (r *ArchiveRepo) Get(ctx context.Context, q sqlite.Querier, id int64) (*model.Archive, error) {
	row := q.QueryRowContext(ctx,
		`SELECT `+archiveCols+` FROM archives WHERE id=? AND deleted_at IS NULL`, id)
	a, err := scanArchive(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, apperr.NotFound("归档", id)
	}
	return a, err
}

// GetByKey 按幂等键取归档。
func (r *ArchiveRepo) GetByKey(ctx context.Context, q sqlite.Querier, key string) (*model.Archive, error) {
	row := q.QueryRowContext(ctx,
		`SELECT `+archiveCols+` FROM archives WHERE idempotency_key=?`, key)
	return scanArchive(row)
}

// GetByBatch 按批次取归档。
func (r *ArchiveRepo) GetByBatch(ctx context.Context, q sqlite.Querier, batchID int64) (*model.Archive, error) {
	row := q.QueryRowContext(ctx,
		`SELECT `+archiveCols+` FROM archives WHERE batch_id=?`, batchID)
	return scanArchive(row)
}

// List 分页列出归档（默认排除已软删除），可按状态过滤。
func (r *ArchiveRepo) List(ctx context.Context, q sqlite.Querier, status string, page Page) ([]model.Archive, error) {
	page = page.Normalize()
	query := `SELECT ` + archiveCols + ` FROM archives WHERE deleted_at IS NULL AND id > ?`
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
	var out []model.Archive
	for rows.Next() {
		a, err := scanArchive(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *a)
	}
	return out, rows.Err()
}

// UpdateStatus 乐观锁更新归档状态（可携带校验人）。
func (r *ArchiveRepo) UpdateStatus(ctx context.Context, q sqlite.Querier, id, version int64, status, verifiedBy string, now time.Time) error {
	return execOptimistic(ctx, q, "archives", "归档", id,
		`UPDATE archives SET status=?, verified_by=CASE WHEN ?='' THEN verified_by ELSE ? END,
		 version=version+1, updated_at=? WHERE id=? AND version=?`,
		status, verifiedBy, verifiedBy, ts(now), id, version)
}

// SoftDelete 软删除归档（置 deleted_at，绝不物理删除）。
func (r *ArchiveRepo) SoftDelete(ctx context.Context, q sqlite.Querier, id, version int64, now time.Time) error {
	return execOptimistic(ctx, q, "archives", "归档", id,
		`UPDATE archives SET deleted_at=?, version=version+1, updated_at=?
		 WHERE id=? AND version=? AND deleted_at IS NULL`,
		ts(now), ts(now), id, version)
}
