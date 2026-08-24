// Package repo 提供基于 database/sql 的仓储实现，方法可运行于 *sql.DB 或 *sql.Tx。
package repo

import (
	"context"
	"database/sql"
	"strings"
	"time"

	"observatory/internal/apperr"
	"observatory/internal/store/sqlite"
)

// Tx 为 *sql.Tx 别名，服务层事务回调使用。
type Tx = *sql.Tx

// timeLayout 为固定小数位的时间格式，保证 TEXT 字典序与时间序一致。
const timeLayout = "2006-01-02T15:04:05.000000000Z07:00"

// Page 为键集分页参数：Cursor 为上一页最后一条的 id（聚合查询为偏移量）。
type Page struct {
	Limit  int
	Cursor int64
}

// Normalize 约束分页参数：Limit 默认 20，最大 100。
func (p Page) Normalize() Page {
	if p.Limit <= 0 {
		p.Limit = 20
	}
	if p.Limit > 100 {
		p.Limit = 100
	}
	if p.Cursor < 0 {
		p.Cursor = 0
	}
	return p
}

func ts(t time.Time) string { return t.UTC().Format(timeLayout) }
func tp(t *time.Time) *string {
	if t == nil {
		return nil
	}
	s := ts(*t)
	return &s
}

func parseTime(s string) (time.Time, error) {
	return time.Parse(timeLayout, s)
}

func parseTimePtr(s *string) (*time.Time, error) {
	if s == nil || *s == "" {
		return nil, nil
	}
	t, err := parseTime(*s)
	if err != nil {
		return nil, err
	}
	return &t, nil
}

// IsUniqueViolation 判断是否为 SQLite 唯一约束冲突（幂等重放判定用）。
func IsUniqueViolation(err error) bool {
	return err != nil && strings.Contains(err.Error(), "UNIQUE constraint failed")
}

// resolveOptimistic 解释乐观锁更新结果：影响 0 行时区分不存在与版本冲突。
func resolveOptimistic(ctx context.Context, q sqlite.Querier, table, entity string, id int64, rows int64) error {
	if rows == 1 {
		return nil
	}
	var n int
	if err := q.QueryRowContext(ctx, "SELECT COUNT(1) FROM "+table+" WHERE id=?", id).Scan(&n); err != nil {
		return err
	}
	if n == 0 {
		return apperr.NotFound(entity, id)
	}
	return apperr.VersionConflict(entity, id)
}

// execOptimistic 执行乐观锁 UPDATE/DELETE 并解析结果。
func execOptimistic(ctx context.Context, q sqlite.Querier, table, entity string, id int64,
	query string, args ...any) error {
	res, err := q.ExecContext(ctx, query, args...)
	if err != nil {
		return err
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return err
	}
	return resolveOptimistic(ctx, q, table, entity, id, rows)
}

// lastID 提取插入结果的主键。
func lastID(res interface{ LastInsertId() (int64, error) }) (int64, error) {
	return res.LastInsertId()
}
