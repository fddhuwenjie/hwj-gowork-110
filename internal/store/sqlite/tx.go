package sqlite

import (
	"context"
	"database/sql"
	"fmt"
)

// Querier 抽象 *sql.DB 与 *sql.Tx，供仓储复用。
type Querier interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// InTx 在真实 SQLite 事务中执行 fn；fn 返回错误时完整回滚，否则提交。
// 使用 BEGIN IMMEDIATE 语义（modernc.org/sqlite 默认延迟事务在写时升级），
// 单连接下天然串行化，保证多步写入的原子性。
func (d *DB) InTx(ctx context.Context, fn func(tx *sql.Tx) error) error {
	tx, err := d.SQL.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("开启事务失败: %w", err)
	}
	if err := fn(tx); err != nil {
		if rbErr := tx.Rollback(); rbErr != nil {
			return fmt.Errorf("回滚失败: %v（原始错误: %w）", rbErr, err)
		}
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("提交事务失败: %w", err)
	}
	return nil
}
