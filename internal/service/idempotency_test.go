package service_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"observatory/internal/model"
)

// TestReadingIdempotency 重复温度读数按幂等键重放，不产生重复记录。
func TestReadingIdempotency(t *testing.T) {
	s := newStack(t)
	ctx := context.Background()

	site, _ := s.svc.Sites.CreateSite(ctx, model.Site{Code: "S6", Name: "站6", Latitude: -80, Longitude: 70, AltitudeM: 4000})
	in, _ := s.svc.Instruments.CreateInstrument(ctx, model.Instrument{
		SiteID: site.ID, Code: "I7", Name: "仪7", Kind: "imager", TempMinMK: 250, TempMaxMK: 350,
	})
	cryo, _ := s.svc.Cryo.RegisterSystem(ctx, in.ID, "制冷机", 300)

	first, replay, err := s.svc.Cryo.AddReading(ctx, cryo.ID, 300, 0, time.Time{}, "idem-rd", "op")
	if err != nil || replay {
		t.Fatalf("首次读数异常: replay=%v err=%v", replay, err)
	}
	second, replay, err := s.svc.Cryo.AddReading(ctx, cryo.ID, 300, 0, time.Time{}, "idem-rd", "op")
	if err != nil {
		t.Fatalf("重放读数失败: %v", err)
	}
	if !replay || second.ID != first.ID {
		t.Fatalf("重放应返回首次读数: replay=%v first=%d second=%d", replay, first.ID, second.ID)
	}
	readings, _ := s.svc.Cryo.ListReadings(ctx, cryo.ID, page100())
	if len(readings) != 1 {
		t.Fatalf("幂等重放不应产生重复读数，实际 %d 条", len(readings))
	}
}

// TestTargetScheduleIdempotency 重复排程按幂等键重放。
func TestTargetScheduleIdempotency(t *testing.T) {
	s := newStack(t)
	ctx := context.Background()

	site, _ := s.svc.Sites.CreateSite(ctx, model.Site{Code: "S7", Name: "站7", Latitude: -80, Longitude: 70, AltitudeM: 4000})
	in, _ := s.svc.Instruments.CreateInstrument(ctx, model.Instrument{
		SiteID: site.ID, Code: "I8", Name: "仪8", Kind: "imager", TempMinMK: 250, TempMaxMK: 350,
	})
	win, _ := s.svc.Windows.Apply(ctx, model.ObservationWindow{
		InstrumentID: in.ID, Title: "W7", StartAt: testEpoch, EndAt: testEpoch.AddDate(0, 0, 1),
	}, "sch")

	first, replay, err := s.svc.Targets.Schedule(ctx, win.ID, model.Target{
		Name: "T", RaDeg: 1, DecDeg: 1, Priority: 1, ExposureGoal: 1, IdempotencyKey: "idem-tgt",
	}, "sch")
	if err != nil || replay {
		t.Fatalf("首次排程异常: replay=%v err=%v", replay, err)
	}
	second, replay, err := s.svc.Targets.Schedule(ctx, win.ID, model.Target{
		Name: "T", RaDeg: 1, DecDeg: 1, Priority: 1, ExposureGoal: 1, IdempotencyKey: "idem-tgt",
	}, "sch")
	if err != nil || !replay || second.ID != first.ID {
		t.Fatalf("重放排程异常: replay=%v err=%v", replay, err)
	}
	targets, _ := s.svc.Targets.ListByWindow(ctx, win.ID, page100())
	if len(targets) != 1 {
		t.Fatalf("幂等重放不应产生重复目标，实际 %d 个", len(targets))
	}
}

// TestArchiveIdempotency 重复归档请求按幂等键重放。
func TestArchiveIdempotency(t *testing.T) {
	s := newStack(t)
	ctx := context.Background()
	st := seedToActiveWindow(t, s)
	b := runBatch(t, s, st, "batch-aidem")

	first, replay, err := s.svc.Archives.Request(ctx, b.ID, strings.Repeat("c", 64), 10, "idem-arch", "op")
	if err != nil || replay {
		t.Fatalf("首次归档异常: replay=%v err=%v", replay, err)
	}
	second, replay, err := s.svc.Archives.Request(ctx, b.ID, strings.Repeat("c", 64), 10, "idem-arch", "op")
	if err != nil || !replay || second.ID != first.ID {
		t.Fatalf("重放归档异常: replay=%v err=%v", replay, err)
	}
}

// TestBatchIdempotency 重复批次创建按幂等键重放。
func TestBatchIdempotency(t *testing.T) {
	s := newStack(t)
	ctx := context.Background()
	st := seedToActiveWindow(t, s)

	first, replay, err := s.svc.Batches.Start(ctx, st.windowID, st.targetID, "s3://raw/i", "idem-batch", nil, "op")
	if err != nil || replay {
		t.Fatalf("首次批次异常: replay=%v err=%v", replay, err)
	}
	second, replay, err := s.svc.Batches.Start(ctx, st.windowID, st.targetID, "s3://raw/i", "idem-batch", nil, "op")
	if err != nil || !replay || second.ID != first.ID {
		t.Fatalf("重放批次异常: replay=%v err=%v", replay, err)
	}
}
