package domain_test
import("testing";"time";"observatory/internal/domain")
func TestBug16Trigger(t *testing.T){a:=time.Unix(1,123);b:=a.Add(time.Hour);if !domain.ValidityCovers(a,b,a,b){t.Fatal("not covered")}}
func TestBug16Control(t *testing.T){a:=time.Unix(1,123);b:=a.Add(time.Hour);if domain.ValidityCovers(a,b,a.Add(-1),b){t.Fatal("overflow")}}
