package domain_test
import("testing";"math";"observatory/internal/domain")
func TestBug17Trigger(t *testing.T){if domain.TempInRange(math.NaN(),250,350){t.Fatal("nan accepted")}}
func TestBug17Control(t *testing.T){if !domain.TempInRange(300,250,350){t.Fatal("normal")}}
