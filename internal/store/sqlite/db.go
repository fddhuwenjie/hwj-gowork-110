// Package sqlite 提供真实嵌入式 SQLite 持久化（database/sql + modernc.org/sqlite）。
package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"strings"

	_ "modernc.org/sqlite"
)

// DB 包装 *sql.DB，承载仓储入口。
type DB struct {
	SQL  *sql.DB
	Path string
}

// Open 打开（必要时创建）位于 path 的 SQLite 数据库文件；禁止 :memory:。
// 单进程服务设置单连接以避免 SQLITE_BUSY，并启用外键与 WAL。
func Open(path string) (*DB, error) {
	if path == "" || path == ":memory:" || strings.HasPrefix(path, "file::memory:") {
		return nil, fmt.Errorf("禁止使用内存数据库，必须提供真实 DB_PATH 文件")
	}
	q := url.Values{}
	q.Add("_pragma", "busy_timeout(5000)")
	q.Add("_pragma", "foreign_keys(1)")
	q.Add("_pragma", "journal_mode(WAL)")
	dsn := fmt.Sprintf("file:%s?%s", path, q.Encode())
	sqlDB, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("打开 SQLite 失败: %w", err)
	}
	sqlDB.SetMaxOpenConns(1)
	if err := sqlDB.Ping(); err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("连接 SQLite 失败: %w", err)
	}
	return &DB{SQL: sqlDB, Path: path}, nil
}

// Close 关闭数据库。
func (d *DB) Close() error { return d.SQL.Close() }

// Ping 供健康检查使用。
func (d *DB) Ping(ctx context.Context) error { return d.SQL.PingContext(ctx) }
