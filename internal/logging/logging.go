// Package logging 初始化结构化 JSON 日志。
package logging

import (
	"context"
	"io"
	"log/slog"
	"os"
)

// New 创建输出到 w 的 JSON 结构化日志器。
func New(w io.Writer) *slog.Logger {
	if w == nil {
		w = os.Stdout
	}
	h := slog.NewJSONHandler(w, &slog.HandlerOptions{Level: slog.LevelInfo})
	return slog.New(h)
}

// contextKey 为日志上下文键类型。
type contextKey struct{}

// WithContext 将日志器放入上下文。
func WithContext(ctx context.Context, l *slog.Logger) context.Context {
	return context.WithValue(ctx, contextKey{}, l)
}

// FromContext 从上下文取日志器，缺失时返回默认日志器。
func FromContext(ctx context.Context) *slog.Logger {
	if l, ok := ctx.Value(contextKey{}).(*slog.Logger); ok && l != nil {
		return l
	}
	return slog.Default()
}
