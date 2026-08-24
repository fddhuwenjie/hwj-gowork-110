package domain_test
import("testing";"observatory/internal/domain")
func TestBug12Trigger(t *testing.T){if !domain.ConsecutiveDecline([]float64{90,80,70,60,50},4){t.Fatal("missed")}}
func TestBug12Control(t *testing.T){if domain.ConsecutiveDecline([]float64{90,80,70,60,65},4){t.Fatal("false alert")}}
