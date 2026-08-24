// Package apperr 定义统一应用错误及 HTTP 映射。
package apperr

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
)

// 错误码常量，与 docs/接口契约.md 对齐。
const (
	CodeInvalidArgument    = "invalid_argument"
	CodeActorRequired      = "actor_required"
	CodeNotFound           = "not_found"
	CodeConflict           = "conflict"
	CodeVersionConflict    = "version_conflict"
	CodeInvalidTransition  = "invalid_transition"
	CodeDuplicate          = "duplicate"
	CodePreconditionFailed = "precondition_failed"
	CodeInternal           = "internal"
)

// Error 为统一应用错误。
type Error struct {
	Code    string         `json:"code"`
	Message string         `json:"message"`
	Details map[string]any `json:"details,omitempty"`
}

// Error 实现 error 接口。
func (e *Error) Error() string { return fmt.Sprintf("%s: %s", e.Code, e.Message) }

// WithDetail 附加错误细节并返回自身。
func (e *Error) WithDetail(key string, value any) *Error {
	if e.Details == nil {
		e.Details = map[string]any{}
	}
	e.Details[key] = value
	return e
}

// New 创建指定错误码的应用错误。
func New(code, message string) *Error {
	return &Error{Code: code, Message: message}
}

// NotFound 创建资源不存在错误。
func NotFound(entity string, id any) *Error {
	return New(CodeNotFound, fmt.Sprintf("%s 不存在", entity)).WithDetail("id", id)
}

// Precondition 创建领域前置条件不满足错误。
func Precondition(message string) *Error {
	return New(CodePreconditionFailed, message)
}

// InvalidArgument 创建参数非法错误。
func InvalidArgument(message string) *Error {
	return New(CodeInvalidArgument, message)
}

// Conflict 创建状态冲突错误。
func Conflict(message string) *Error {
	return New(CodeConflict, message)
}

// VersionConflict 创建乐观锁失配错误。
func VersionConflict(entity string, id any) *Error {
	return New(CodeVersionConflict, fmt.Sprintf("%s 版本冲突，请刷新后重试", entity)).WithDetail("id", id)
}

// InvalidTransition 创建非法状态转换错误。
func InvalidTransition(entity, from, to string) *Error {
	return New(CodeInvalidTransition, fmt.Sprintf("%s 不允许从 %s 转换为 %s", entity, from, to)).
		WithDetail("from", from).WithDetail("to", to)
}

// HTTPStatus 将错误码映射为 HTTP 状态码。
func HTTPStatus(code string) int {
	switch code {
	case CodeInvalidArgument:
		return http.StatusBadRequest
	case CodeActorRequired:
		return http.StatusUnauthorized
	case CodeNotFound:
		return http.StatusNotFound
	case CodeConflict, CodeVersionConflict, CodeInvalidTransition, CodeDuplicate:
		return http.StatusConflict
	case CodePreconditionFailed:
		return http.StatusPreconditionFailed
	default:
		return http.StatusInternalServerError
	}
}

// Write 将错误以统一 JSON 结构写出。
func Write(w http.ResponseWriter, err error) {
	var ae *Error
	if !errors.As(err, &ae) {
		ae = New(CodeInternal, "内部错误")
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(HTTPStatus(ae.Code))
	_ = json.NewEncoder(w).Encode(map[string]any{"error": ae})
}
