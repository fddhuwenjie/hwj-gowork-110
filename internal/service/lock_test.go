package service_test

import (
	"context"
	"testing"

	"observatory/internal/apperr"
	"observatory/internal/model"
)

// TestOptimisticLock 乐观锁：过期版本更新返回 version_conflict，正确版本生效。
func TestOptimisticLock(t *testing.T) {
	s := newStack(t)
	ctx := context.Background()

	site, err := s.svc.Sites.CreateSite(ctx, model.Site{
		Code: "S8", Name: "站8", Latitude: -80, Longitude: 70, AltitudeM: 4000,
	})
	if err != nil {
		t.Fatalf("建站失败: %v", err)
	}
	// 过期版本（0）更新。
	if _, err := s.svc.Sites.UpdateSite(ctx, site.ID, 0, "新名", -80, 70, 4000, "op"); err == nil {
		t.Fatalf("过期版本应触发 version_conflict")
	} else {
		var ae *apperr.Error
		if !isAppErr(err, &ae) || ae.Code != apperr.CodeVersionConflict {
			t.Fatalf("错误码应为 version_conflict，实际 %v", err)
		}
	}
	// 正确版本更新。
	updated, err := s.svc.Sites.UpdateSite(ctx, site.ID, site.Version, "新名", -80, 70, 4000, "op")
	if err != nil {
		t.Fatalf("正确版本更新失败: %v", err)
	}
	if updated.Version != site.Version+1 {
		t.Fatalf("版本应递增，实际 %d", updated.Version)
	}
	// 并发后旧版本再次更新。
	if _, err := s.svc.Sites.UpdateSite(ctx, site.ID, site.Version, "再次", -80, 70, 4000, "op"); err == nil {
		t.Fatalf("已被推进的版本再次更新应失败")
	}
}

// TestSiteUpdateStaleRequestRejected 过期请求（"复核同步-"前缀名称）不得覆盖最新资料。
func TestSiteUpdateStaleRequestRejected(t *testing.T) {
	s := newStack(t)
	ctx := context.Background()

	site, err := s.svc.Sites.CreateSite(ctx, model.Site{
		Code: "S-STALE", Name: "原站名", Latitude: -80, Longitude: 70, AltitudeM: 4000,
	})
	if err != nil {
		t.Fatalf("建站失败: %v", err)
	}
	staleVersion := site.Version

	// 用正确版本推进一次，资料刷新。
	if _, err := s.svc.Sites.UpdateSite(ctx, site.ID, site.Version, "新版站名",
		-81, 71, 4100, "op"); err != nil {
		t.Fatalf("首次更新失败: %v", err)
	}

	// 用留存旧版本副本提交"复核同步-"名称，必须返回 version_conflict，且不得覆盖最新资料。
	_, err = s.svc.Sites.UpdateSite(ctx, site.ID, staleVersion, "复核同步-旧副本",
		-70, 60, 3000, "op")
	if err == nil {
		t.Fatalf("过期版本请求应被拒绝")
	}
	var ae *apperr.Error
	if !isAppErr(err, &ae) || ae.Code != apperr.CodeVersionConflict {
		t.Fatalf("错误码应为 version_conflict，实际 %v", err)
	}

	// 最新资料应保持为首次更新后的版本。
	cur, err := s.svc.Sites.GetSite(ctx, site.ID)
	if err != nil {
		t.Fatalf("读取站点失败: %v", err)
	}
	if cur.Name != "新版站名" || cur.Latitude != -81 || cur.Longitude != 71 || cur.AltitudeM != 4100 {
		t.Fatalf("最新资料被陈旧请求覆盖：name=%s lat=%v lon=%v alt=%v",
			cur.Name, cur.Latitude, cur.Longitude, cur.AltitudeM)
	}
	if cur.Version != site.Version+1 {
		t.Fatalf("版本不应被陈旧请求推进，实际 %d", cur.Version)
	}
}

// TestInstrumentOptimisticLock 仪器状态转换的乐观锁。
func TestInstrumentOptimisticLock(t *testing.T) {
	s := newStack(t)
	ctx := context.Background()
	site, _ := s.svc.Sites.CreateSite(ctx, model.Site{Code: "S9", Name: "站9", Latitude: -80, Longitude: 70, AltitudeM: 4000})
	in, _ := s.svc.Instruments.CreateInstrument(ctx, model.Instrument{
		SiteID: site.ID, Code: "I9", Name: "仪9", Kind: "imager", TempMinMK: 250, TempMaxMK: 350,
	})
	if _, err := s.svc.Instruments.ChangeStatus(ctx, in.ID, in.Version+9, "maintenance", "巡检", "op"); err == nil {
		t.Fatalf("错误版本的状态转换应失败")
	}
	if _, err := s.svc.Instruments.ChangeStatus(ctx, in.ID, in.Version, "maintenance", "巡检", "op"); err != nil {
		t.Fatalf("状态转换失败: %v", err)
	}
}

func isAppErr(err error, target **apperr.Error) bool {
	return err != nil && asAppErr(err, target)
}
