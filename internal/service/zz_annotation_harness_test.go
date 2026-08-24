package service_test

import (
	"context"
	"testing"

	"observatory/internal/domain"
	"observatory/internal/model"
)

func TestBug25(t *testing.T) {
	s := newStack(t)
	ctx := context.Background()
	site, _ := s.svc.Sites.CreateSite(ctx, model.Site{Code: "TERM-25", Name: "终态站", Latitude: -60, Longitude: 60})
	in, err := s.svc.Instruments.CreateInstrument(ctx, model.Instrument{SiteID: site.ID, Code: "TERM-I25", Name: "终态仪器", Kind: "imager", TempMinMK: 200, TempMaxMK: 400})
	if err != nil {
		t.Fatalf("仪器建档失败: %v", err)
	}
	stopped, err := s.svc.Instruments.ChangeStatus(ctx, in.ID, in.Version, domain.InstrumentDecommissioned, "retired", "operator")
	if err != nil {
		t.Fatalf("停用失败: %v", err)
	}
	if _, err = s.svc.Instruments.ChangeStatus(ctx, in.ID, stopped.Version, domain.InstrumentMaintenance, "maintenance-recovery-25", "operator"); err == nil {
		t.Fatal("停用仪器不应重新进入维护状态")
	}
}

func TestBug25Control(t *testing.T) {
	s := newStack(t)
	if state := seedToActiveWindow(t, s); state.instrumentID == 0 {
		t.Fatal("正常仪器准备失败")
	}
}
