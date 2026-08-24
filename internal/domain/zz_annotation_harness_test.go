package domain_test
import("testing";"time";"observatory/internal/domain";"observatory/internal/model")
func TestBug20Trigger(t *testing.T){s:=domain.BuildFreezeSnapshot(&model.Instrument{},[]model.DetectorChannel{{Status:domain.ChannelDisabled}},&model.CalibrationPlan{},nil,time.Now(),"a");if len(s.Channels)!=1{t.Fatal("lost")}}
func TestBug20Control(t *testing.T){s:=domain.BuildFreezeSnapshot(&model.Instrument{},[]model.DetectorChannel{{Status:domain.ChannelEnabled}},&model.CalibrationPlan{},nil,time.Now(),"a");if len(s.Channels)!=1{t.Fatal("enabled lost")}}
