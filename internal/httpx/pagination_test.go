package httpx_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"observatory/internal/clock"
	"observatory/internal/config"
	"observatory/internal/httpx"
	"observatory/internal/logging"
	"observatory/internal/model"
	"observatory/internal/service"
	"observatory/internal/store/sqlite"
)

// newHTTPServer 基于真实临时 SQLite 文件构建测试 HTTP 服务。
func newHTTPServer(t *testing.T) (*httptest.Server, *service.Services) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "http.db")
	db, err := sqlite.Open(dbPath)
	if err != nil {
		t.Fatalf("打开数据库失败: %v", err)
	}
	if err := db.Migrate(context.Background()); err != nil {
		t.Fatalf("迁移失败: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	svc := service.New(db, clock.Real{}, 3)
	srv := httpx.NewServer(config.Config{Port: 0}, svc, logging.New(io.Discard))
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	return ts, svc
}

type listResp struct {
	Items      []json.RawMessage `json:"items"`
	NextCursor string            `json:"next_cursor"`
}

// TestStablePagination 稳定分页：25 个站点按 id 升序分 3 页取完，不重不漏。
func TestStablePagination(t *testing.T) {
	ts, svc := newHTTPServer(t)
	ctx := context.Background()
	for i := 0; i < 25; i++ {
		if _, err := svc.Sites.CreateSite(ctx, model.Site{
			Code: fmt.Sprintf("SITE-%02d", i), Name: "站", Latitude: -80, Longitude: 70, AltitudeM: 4000,
		}); err != nil {
			t.Fatalf("建站失败: %v", err)
		}
	}

	seen := map[int64]bool{}
	cursor := ""
	pages := 0
	for {
		url := ts.URL + "/api/v1/sites?limit=10"
		if cursor != "" {
			url += "&cursor=" + cursor
		}
		resp, err := http.Get(url)
		if err != nil {
			t.Fatalf("请求失败: %v", err)
		}
		var body listResp
		if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
			t.Fatalf("解码失败: %v", err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("状态码异常: %d", resp.StatusCode)
		}
		pages++
		prevID := int64(0)
		for _, raw := range body.Items {
			var site model.Site
			if err := json.Unmarshal(raw, &site); err != nil {
				t.Fatalf("解析站点失败: %v", err)
			}
			if seen[site.ID] {
				t.Fatalf("分页出现重复 id=%d", site.ID)
			}
			if site.ID <= prevID {
				t.Fatalf("分页顺序不稳定: id=%d 出现在 %d 之后", site.ID, prevID)
			}
			prevID = site.ID
			seen[site.ID] = true
		}
		if body.NextCursor == "" {
			break
		}
		cursor = body.NextCursor
		if pages > 5 {
			t.Fatalf("分页未收敛")
		}
	}
	if len(seen) != 25 {
		t.Fatalf("应取到 25 个站点，实际 %d", len(seen))
	}
}

// TestHealthz 健康检查。
func TestHealthz(t *testing.T) {
	ts, _ := newHTTPServer(t)
	resp, err := http.Get(ts.URL + "/healthz")
	if err != nil {
		t.Fatalf("请求失败: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("健康检查失败: %d", resp.StatusCode)
	}
	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("解码失败: %v", err)
	}
	if body["status"] != "ok" {
		t.Fatalf("健康检查响应异常: %v", body)
	}
}

// TestUnifiedError 统一错误结构：404 与 409 均返回 {"error":{...}}。
func TestUnifiedError(t *testing.T) {
	ts, svc := newHTTPServer(t)

	resp, err := http.Get(ts.URL + "/api/v1/sites/999")
	if err != nil {
		t.Fatalf("请求失败: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("应为 404，实际 %d", resp.StatusCode)
	}
	var body map[string]map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("解码失败: %v", err)
	}
	if body["error"]["code"] != "not_found" {
		t.Fatalf("错误码应为 not_found，实际 %v", body["error"]["code"])
	}

	// 乐观锁失配 → 409 version_conflict。
	site, _ := svc.Sites.CreateSite(context.Background(), model.Site{
		Code: "LOCK", Name: "锁", Latitude: -80, Longitude: 70, AltitudeM: 4000,
	})
	req, _ := http.NewRequest("PATCH", ts.URL+fmt.Sprintf("/api/v1/sites/%d", site.ID),
		strings.NewReader(`{"name":"x","latitude":1,"longitude":1,"altitude_m":1,"version":99}`))
	req.Header.Set("Content-Type", "application/json")
	resp2, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("请求失败: %v", err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusConflict {
		t.Fatalf("应为 409，实际 %d", resp2.StatusCode)
	}
	var body2 map[string]map[string]any
	if err := json.NewDecoder(resp2.Body).Decode(&body2); err != nil {
		t.Fatalf("解码失败: %v", err)
	}
	if body2["error"]["code"] != "version_conflict" {
		t.Fatalf("错误码应为 version_conflict，实际 %v", body2["error"]["code"])
	}
}
