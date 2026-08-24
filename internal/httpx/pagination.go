package httpx

import (
	"net/http"
	"strconv"

	"observatory/internal/apperr"
	"observatory/internal/repo"
)

// ParsePage 解析稳定分页参数：limit（1..100，默认 20）与 cursor（非负整数，缺省 0）。
func ParsePage(r *http.Request) (repo.Page, error) {
	q := r.URL.Query()
	page := repo.Page{Limit: 20}
	if raw := q.Get("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n <= 0 || n > 100 {
			return page, apperr.InvalidArgument("limit 必须为 1..100 的整数").WithDetail("limit", raw)
		}
		page.Limit = n
	}
	if raw := q.Get("cursor"); raw != "" {
		n, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || n < 0 {
			return page, apperr.InvalidArgument("cursor 必须为非负整数").WithDetail("cursor", raw)
		}
		page.Cursor = n
	}
	return page, nil
}

// NextCursor 依据本页结果计算下一页游标：取满一页时返回最后一条的 id，否则返回空串。
func NextCursor[T any](items []T, limit int, idOf func(T) int64) string {
	if len(items) == 0 || len(items) < limit {
		return ""
	}
	return strconv.FormatInt(idOf(items[len(items)-1]), 10)
}

// NextOffsetCursor 为聚合查询计算下一页偏移游标。
func NextOffsetCursor(rowCount, limit int, cursor int64) string {
	if rowCount < limit {
		return ""
	}
	return strconv.FormatInt(cursor+int64(limit), 10)
}

// QueryInt64 解析可选整型查询参数。
func QueryInt64(r *http.Request, name string) (int64, error) {
	raw := r.URL.Query().Get(name)
	if raw == "" {
		return 0, nil
	}
	n, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || n < 0 {
		return 0, apperr.InvalidArgument("查询参数 "+name+" 必须为非负整数").WithDetail("value", raw)
	}
	return n, nil
}

// QueryInt 解析可选整型查询参数（int）。
func QueryInt(r *http.Request, name string) (int, error) {
	n, err := QueryInt64(r, name)
	return int(n), err
}
