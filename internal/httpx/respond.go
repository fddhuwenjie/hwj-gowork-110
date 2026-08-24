package httpx

import (
	"encoding/json"
	"net/http"
	"strings"

	"observatory/internal/apperr"
)

// JSON 以统一结构写出响应。
func JSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// OK 写出 200 响应。
func OK(w http.ResponseWriter, v any) { JSON(w, http.StatusOK, v) }

// Created 写出 201 响应。
func Created(w http.ResponseWriter, v any) { JSON(w, http.StatusCreated, v) }

// Error 写出统一错误。
func Error(w http.ResponseWriter, err error) { apperr.Write(w, err) }

// listEnvelope 为分页响应结构。
type listEnvelope struct {
	Items      any    `json:"items"`
	NextCursor string `json:"next_cursor,omitempty"`
}

// List 写出分页响应；items 为 nil 时输出空数组。
func List(w http.ResponseWriter, items any, nextCursor string) {
	if items == nil {
		items = []any{}
	}
	OK(w, listEnvelope{Items: items, NextCursor: nextCursor})
}

// Decode 解析 JSON 请求体；空体视为错误（除允许为空的接口）。
func Decode(r *http.Request, v any) error {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return apperr.InvalidArgument("请求体不是合法 JSON 或字段非法: " + err.Error())
	}
	return nil
}

// Actor 取操作人（X-Actor 头，缺省 system）。
func Actor(r *http.Request) string {
	a := strings.TrimSpace(r.Header.Get("X-Actor"))
	if a == "" {
		return "system"
	}
	return a
}
