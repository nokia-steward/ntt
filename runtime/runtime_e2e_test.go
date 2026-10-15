package runtime_test

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/nokia/ntt/runtime/alt"
	"github.com/nokia/ntt/runtime/component"
	"github.com/nokia/ntt/runtime/port"
	"github.com/nokia/ntt/runtime/timer"
)

// TestE2E_PingPong wires the M2 runtime packages together to simulate a
// minimal TTCN-3 testcase: an MTC creates one PTC, connects two ports,
// the PTC echoes whatever it receives, the MTC sends a ping, awaits the
// pong via an alt with a timer guard, and asserts the verdict.
//
// This is the smoke test that proves the runtime building blocks all
// click together. The interpreter rewire that the plan calls for in M2
// will use exactly this orchestration pattern.
func TestE2E_PingPong(t *testing.T) {
	reg := component.NewRegistry()
	mtc := reg.New("mtc", component.MTC)
	ptc := reg.New("ptc", component.PTC)

	mtcPort := port.New("mtcP", port.Message, 0)
	ptcPort := port.New("ptcP", port.Message, 0)
	port.Connect(mtcPort, ptcPort)

	ptc.Start(func(c *component.Component) error {
		// Echo loop with a kill check at the top.
		for {
			select {
			case <-c.Killed():
				return component.ErrKilled
			default:
			}
			v := alt.Run([]alt.Branch{{
				Port: ptcPort,
				Body: func(x interface{}) {
					if env, ok := x.(*port.Envelope); ok {
						ptcPort.Send(env.Payload.(string) + "-pong")
					}
				},
			}}, c.Killed())
			if v.BranchIndex == -1 {
				return component.ErrKilled
			}
			return nil
		}
	})

	mtcPort.Send("ping")

	guard := timer.New("guard")
	guard.Start(500 * time.Millisecond)

	var pong string
	v := alt.Run([]alt.Branch{
		{
			Port: mtcPort,
			Body: func(x interface{}) {
				if env, ok := x.(*port.Envelope); ok {
					pong = env.Payload.(string)
				}
			},
		},
		{Timer: guard},
	}, nil)

	if v.BranchIndex != 0 {
		t.Fatalf("expected port branch, got branch %d", v.BranchIndex)
	}
	if pong != "ping-pong" {
		t.Fatalf("got %q, want %q", pong, "ping-pong")
	}

	guard.Stop()
	ptc.Kill()
	select {
	case <-ptc.Done():
	case <-time.After(time.Second):
		t.Fatal("ptc did not finish")
	}

	if !mtc.Alive() {
		t.Error("mtc should still be Alive (we never started a body for it)")
	}
}

// TestE2E_TimerGuard demonstrates the standard "deadline-bound receive"
// idiom: alt waits for a port message, but a timer ensures the test
// fails fast if no peer ever sends one. Mirrors the canonical TTCN-3
// `T.timeout { setverdict(fail) }` pattern.
func TestE2E_TimerGuard(t *testing.T) {
	mtcPort := port.New("p", port.Message, 0)
	guard := timer.New("guard")
	guard.Start(20 * time.Millisecond)

	var timedOut int32
	v := alt.Run([]alt.Branch{
		{Port: mtcPort},
		{Timer: guard, Body: func(interface{}) { atomic.StoreInt32(&timedOut, 1) }},
	}, nil)
	if v.BranchIndex != 1 {
		t.Fatalf("expected timer branch, got %d", v.BranchIndex)
	}
	if atomic.LoadInt32(&timedOut) == 0 {
		t.Error("timer body did not fire")
	}
}
