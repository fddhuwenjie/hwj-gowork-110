package domain_test
import("testing";"observatory/internal/domain")
func TestBug14Trigger(t *testing.T){if e:=domain.EnsureDifferentReviewer("alice"," alice ");e!=nil{t.Fatal(e)}}
func TestBug14Control(t *testing.T){if domain.EnsureDifferentReviewer("alice","alice")==nil{t.Fatal("same accepted")}}
