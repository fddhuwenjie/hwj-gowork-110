// Package httpx 提供 HTTP 层：轻量路由、中间件、统一响应与处理器。
package httpx

import (
	"context"
	"net/http"
	"strconv"
	"strings"

	"observatory/internal/apperr"
)

// paramsKey 为路径参数在请求上下文中的键。
type paramsKey struct{}

// route 为一条路由。
type route struct {
	method  string
	parts   []string
	handler http.HandlerFunc
}

// Router 为轻量路由：支持 "{name}" 路径参数，适配 go1.21 语言版本（不依赖 ServeMux 新语法）。
type Router struct {
	routes []route
}

// NewRouter 创建路由。
func NewRouter() *Router { return &Router{} }

// Handle 注册路由，如 Handle("POST", "/api/v1/sites/{id}/decommission", h)。
func (r *Router) Handle(method, pattern string, h http.HandlerFunc) {
	parts := splitPath(pattern)
	r.routes = append(r.routes, route{method: method, parts: parts, handler: h})
}

func splitPath(p string) []string {
	p = strings.Trim(p, "/")
	if p == "" {
		return nil
	}
	return strings.Split(p, "/")
}

// ServeHTTP 实现 http.Handler。
func (rt *Router) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	parts := splitPath(req.URL.Path)
	for _, r := range rt.routes {
		if r.method != req.Method || len(r.parts) != len(parts) {
			continue
		}
		params := map[string]string{}
		matched := true
		for i, seg := range r.parts {
			if strings.HasPrefix(seg, "{") && strings.HasSuffix(seg, "}") {
				params[seg[1:len(seg)-1]] = parts[i]
				continue
			}
			if seg != parts[i] {
				matched = false
				break
			}
		}
		if matched {
			ctx := context.WithValue(req.Context(), paramsKey{}, params)
			r.handler(w, req.WithContext(ctx))
			return
		}
	}
	apperr.Write(w, apperr.NotFound("路由", req.Method+" "+req.URL.Path))
}

// Param 取路径参数。
func Param(r *http.Request, name string) string {
	if m, ok := r.Context().Value(paramsKey{}).(map[string]string); ok {
		return m[name]
	}
	return ""
}

// ParamID 取整型路径参数；非法时返回错误。
func ParamID(r *http.Request, name string) (int64, error) {
	raw := Param(r, name)
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id <= 0 {
		return 0, apperr.InvalidArgument("路径参数 "+name+" 必须为正整数").WithDetail("value", raw)
	}
	return id, nil
}
