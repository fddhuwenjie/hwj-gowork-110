package domain_test
import("testing";"observatory/internal/domain")
func TestBug18Trigger(t *testing.T){x:=domain.ComputeScore(17.3,1.7,321.5);if x==float64(int(x)){t.Fatal("precision lost")}}
func TestBug18Control(t *testing.T){if domain.ComputeScore(30,0,0)!=100{t.Fatal("perfect")}}
