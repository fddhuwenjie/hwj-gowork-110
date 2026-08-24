package repo

import (
	"context"
	"time"

	"observatory/internal/model"
	"observatory/internal/store/sqlite"
)

// AuditRepo 为审计日志仓储（只追加，不可变）。
type AuditRepo struct{}

// NewAuditRepo 创建审计仓储。
func NewAuditRepo() *AuditRepo { return &AuditRepo{} }

// Append 追加一条审计记录。
func (r *AuditRepo) Append(ctx context.Context, q sqlite.Querier, e *model.AuditEntry, now time.Time) error {
	res, err := q.ExecContext(ctx,
		`INSERT INTO audit_log(entity,entity_id,action,actor,detail,created_at) VALUES(?,?,?,?,?,?)`,
		e.Entity, e.EntityID, e.Action, e.Actor, e.Detail, ts(now))
	if err != nil {
		return err
	}
	e.ID, err = lastID(res)
	e.CreatedAt = now
	return err
}

// List 分页列出指定实体的审计记录。
func (r *AuditRepo) List(ctx context.Context, q sqlite.Querier, entity string, entityID int64, page Page) ([]model.AuditEntry, error) {
	page = page.Normalize()
	rows, err := q.QueryContext(ctx,
		`SELECT id,entity,entity_id,action,actor,detail,created_at
		 FROM audit_log WHERE entity=? AND entity_id=? AND id > ? ORDER BY id LIMIT ?`,
		entity, entityID, page.Cursor, page.Limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.AuditEntry
	for rows.Next() {
		var e model.AuditEntry
		var created string
		if err := rows.Scan(&e.ID, &e.Entity, &e.EntityID, &e.Action, &e.Actor, &e.Detail, &created); err != nil {
			return nil, err
		}
		if e.CreatedAt, err = parseTime(created); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
