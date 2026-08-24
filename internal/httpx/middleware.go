package httpx

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"net/http"
	"time"

	"observatory/internal/apperr"
	"observatory/internal/logging"
)

// requestIDKey 为请求 ID 的上下文键。
type requestIDKey struct{}

// RequestID 从上下文取请求 ID。
func RequestID(r *http.Request) string {
	if v, ok := r.Context().Value(requestIDKey{}).(string); ok {
		return v
	}
	return ""
}

// statusWriter 记录响应状态码。
type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

// Chain 组装请求 ID、结构化访问日志与 panic 恢复中间件。
func Chain(next http.Handler, log *slog.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		reqID := r.Header.Get("X-Request-Id")
		if reqID == "" {
			b := make([]byte, 8)
			_, _ = rand.Read(b)
			reqID = hex.EncodeToString(b)
		}
		ctx := logging.WithContext(r.Context(), log.With("request_id", reqID))
		ctx = WithRequestID(ctx, reqID)
		w.Header().Set("X-Request-Id", reqID)
		sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		defer func() {
			if rec := recover(); rec != nil {
				log.Error("请求处理 panic", "request_id", reqID, "panic", rec)
				Error(w, apperr.New(apperr.CodeInternal, "内部错误"))
			}
			log.Info("HTTP 请求",
				"request_id", reqID, "method", r.Method, "path", r.URL.Path,
				"status", sw.status, "duration_ms", time.Since(start).Milliseconds())
		}()
		next.ServeHTTP(sw, r.WithContext(ctx))
	})
}

// WithRequestID 将请求 ID 放入上下文。
func WithRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, requestIDKey{}, id)
}
