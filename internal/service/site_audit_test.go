package service_test

import (
	"context"
	"testing"

	"observatory/internal/model"
)

// TestUpdateSiteRollsBackOnAuditFailure 审计写入失败时站点更新必须整体回滚：
// 数据库拒绝审计 INSERT → 调用返回错误，且重新读取站点名称与版本应保持原值，
// 不得出现"无审计凭据的业务状态"。
func TestUpdateSiteRollsBackOnAuditFailure(t *testing.T) {
	s := newStack(t)
	ctx := context.Background()

	site, err := s.svc.Sites.CreateSite(ctx, model.Site{
		Code: "S-AUD", Name: "原站名", Latitude: -80, Longitude: 70, AltitudeM: 4000,
	})
	if err != nil {
		t.Fatalf("建站失败: %v", err)
	}
	origName := site.Name
	origVersion := site.Version

	// 破坏 audit_log 表，使后续审计 INSERT 必然失败。
	if _, err := s.db.SQL.ExecContext(ctx, `DROP TABLE audit_log`); err != nil {
		t.Fatalf("破坏审计表失败: %v", err)
	}

	// 更新应因审计写入失败而整体回滚。
	if _, err := s.svc.Sites.UpdateSite(ctx, site.ID, site.Version, "新站名", -81, 71, 4100, "op"); err == nil {
		t.Fatalf("审计写入失败时站点更新应返回错误")
	}

	// 重新读取：名称与版本必须保持原值，不得出现无审计凭据的变更。
	after, err := s.svc.Sites.GetSite(ctx, site.ID)
	if err != nil {
		t.Fatalf("回读站点失败: %v", err)
	}
	if after.Name != origName {
		t.Fatalf("回滚后站点名称应保持 %q，实际 %q", origName, after.Name)
	}
	if after.Version != origVersion {
		t.Fatalf("回滚后站点版本应保持 %d，实际 %d", origVersion, after.Version)
	}
}
