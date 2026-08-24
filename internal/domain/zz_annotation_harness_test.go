package domain_test
import("testing";"observatory/internal/domain")
func TestBug19Trigger(t *testing.T){if !domain.CanTransition(domain.EntityJob,domain.JobDead,domain.JobPending){t.Fatal("dead blocked")}}
func TestBug19Control(t *testing.T){if !domain.CanTransition(domain.EntityJob,domain.JobFailed,domain.JobPending){t.Fatal("failed blocked")}}
