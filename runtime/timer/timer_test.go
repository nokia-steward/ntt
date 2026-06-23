package timer_test

import (
	"testing"
	"time"

	"github.com/nokia/ntt/runtime/timer"
)

func TestTimer_Lifecycle(t *testing.T) {
	tm := timer.New("T1")
	if tm.State() != timer.Idle {
		t.Fatalf("initial state = %v, want Idle", tm.State())
	}
	tm.Start(10 * time.Millisecond)
	if !tm.Running() {
		t.Fatal("expected Running after Start")
	}
	select {
	case <-tm.Timeout():
		// ok
	case <-time.After(200 * time.Millisecond):
		t.Fatal("timeout not fired")
	}
	if tm.State() != timer.Expired {
		t.Fatalf("state = %v, want Expired", tm.State())
	}
	if !tm.Drain() {
		t.Fatal("Drain should have returned true")
	}
	if tm.State() != timer.Idle {
		t.Fatalf("state after drain = %v, want Idle", tm.State())
	}
}

func TestTimer_Stop(t *testing.T) {
	tm := timer.New("T")
	tm.Start(100 * time.Millisecond)
	time.Sleep(5 * time.Millisecond)
	tm.Stop()
	if tm.Running() {
		t.Fatal("Running should be false after Stop")
	}
	select {
	case <-tm.Timeout():
		t.Fatal("Stop should suppress timeout")
	case <-time.After(150 * time.Millisecond):
	}
}

func TestTimer_Restart(t *testing.T) {
	tm := timer.New("T")
	tm.Start(100 * time.Millisecond)
	time.Sleep(20 * time.Millisecond)
	tm.Start(10 * time.Millisecond) // restart with shorter duration
	select {
	case <-tm.Timeout():
	case <-time.After(150 * time.Millisecond):
		t.Fatal("restarted timer should fire on the new shorter duration")
	}
}

func TestTimer_Read(t *testing.T) {
	tm := timer.New("T")
	tm.Start(50 * time.Millisecond)
	time.Sleep(10 * time.Millisecond)
	if d := tm.Read(); d < 5*time.Millisecond {
		t.Errorf("Read = %v, want >= 5ms", d)
	}
}
