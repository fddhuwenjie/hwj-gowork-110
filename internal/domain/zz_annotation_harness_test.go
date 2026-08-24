package domain_test
import("testing";"time";"observatory/internal/domain")
func TestBug11Trigger(t *testing.T){x:=time.Unix(1,0);if e:=domain.EnsureRecordBefore(x,x);e!=nil{t.Fatal(e)}}
func TestBug11Control(t *testing.T){x:=time.Unix(1,0);if domain.EnsureRecordBefore(x.Add(time.Second),x)==nil{t.Fatal("late accepted")}}
