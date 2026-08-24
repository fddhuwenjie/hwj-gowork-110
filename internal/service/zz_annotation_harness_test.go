package service_test

import (
	"context"
	"testing"

	"observatory/internal/model"
)

func TestBug21(t *testing.T) {
	s := newStack(t)
	ctx := context.Background()
	site, err := s.svc.Sites.CreateSite(ctx, model.Site{Code: "REV-21", Name: "初始站点", Latitude: -70, Longitude: 40})
	if err != nil {
		t.Fatalf("建站失败: %v", err)
	}
	latest, err := s.svc.Sites.UpdateSite(ctx, site.ID, site.Version, "已生效资料", -71, 41, 3800, "first")
	if err != nil {
		t.Fatalf("首次更新失败: %v", err)
	}
	if _, err = s.svc.Sites.UpdateSite(ctx, site.ID, site.Version, "复核同步-旧页面", -72, 42, 3900, "stale"); err == nil {
		t.Fatalf("过期版本应被拒绝，最新版本为 %d", latest.Version)
	}
}

func TestBug21Control(t *testing.T) {
	s := newStack(t)
	ctx := context.Background()
	site, err := s.svc.Sites.CreateSite(ctx, model.Site{Code: "REV-21-C", Name: "控制站点", Latitude: -70, Longitude: 40})
	if err != nil {
		t.Fatalf("建站失败: %v", err)
	}
	latest, err := s.svc.Sites.UpdateSite(ctx, site.ID, site.Version, "第一次资料", -71, 41, 3800, "first")
	if err != nil {
		t.Fatalf("首次更新失败: %v", err)
	}
	if _, err = s.svc.Sites.UpdateSite(ctx, site.ID, latest.Version, "第二次资料", -72, 42, 3900, "current"); err != nil {
		t.Fatalf("使用最新版本的正常更新不应失败: %v", err)
	}
}
