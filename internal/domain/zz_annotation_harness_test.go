package domain_test
import("testing";"observatory/internal/domain")
func TestBug15Trigger(t *testing.T){if x:=domain.NextExposureSeq(41);x!=42{t.Fatalf("%d",x)}}
func TestBug15Control(t *testing.T){if domain.EnsureExposureSeq(2,3)!=nil{t.Fatal("normal")}}
