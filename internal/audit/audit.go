// Package audit 提供追加式审计写入。
package audit

import (
	"context"
	"encoding/json"
	"time"

	"observatory/internal/model"
	"observatory/internal/repo"
	"observatory/internal/store/sqlite"
)

// Writer 在业务事务内追加审计记录。
type Writer struct {
	repo *repo.AuditRepo
}

// NewWriter 创建审计写入器。
func NewWriter(r *repo.AuditRepo) *Writer { return &Writer{repo: r} }

// Log 追加一条审计；detail 序列化为 JSON。
func (w *Writer) Log(ctx context.Context, q sqlite.Querier, entity string, entityID int64,
	action, actor string, detail any, now time.Time) error {
	raw := "{}"
	if detail != nil {
		b, err := json.Marshal(detail)
		if err != nil {
			return err
		}
		raw = string(b)
	}
	_ = w.repo.Append(ctx, q, &model.AuditEntry{
		Entity:   entity,
		EntityID: entityID,
		Action:   action,
		Actor:    actor,
		Detail:   raw,
	}, now)
	return nil
}
