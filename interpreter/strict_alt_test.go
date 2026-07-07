package interpreter_test

import (
	"context"
	"testing"
	"time"

	"github.com/nokia/ntt/interpreter"
	"github.com/nokia/ntt/runtime"
	"github.com/nokia/ntt/ttcn3"
)

func runStrict(t *testing.T, qname, src string) (runtime.Verdict, string) {
	t.Helper()
	v, reason, err := interpreter.RunTestcaseWith([]*ttcn3.Tree{parse(t, src)}, qname,
		interpreter.TestcaseOptions{Profile: runtime.ProfileStrict})
	if err != nil {
		t.Fatalf("RunTestcaseWith(%s): %v", qname, err)
	}
	return v, reason
}

// TestStrictAlt_ConnectedPeerReceive covers the ProfileStrict snapshot
// alt evaluator + connection-topology routing: a connected peer sends,
// and the MTC's alt receives it on the connected port. Under the strict
// profile a send on a connected port must reach the PEER's queue (not
// the sender's own), and the alt must match by real guard evaluation —
// not the verdict-preferring heuristic. Mirrors
// Sem_2002_TheAltStatement_016.
func TestStrictAlt_ConnectedPeerReceive(t *testing.T) {
	v, reason := runStrict(t, "M.tc", `module M {
		type port P message { inout charstring }
		type component C { port P p }
		function fsend() runs on C { p.send("ping"); }
		testcase tc() runs on C system C {
			var C peer := C.create;
			connect(self:p, peer:p);
			peer.start(fsend());
			alt {
				[] p.receive(charstring:"ping") { setverdict(pass); }
				[] p.receive { setverdict(fail, "wrong message"); }
			}
		}
	}`)
	if v != runtime.PassVerdict {
		t.Fatalf("verdict = %s (%s), want pass (connected send must reach the peer's queue)", v, reason)
	}
}

// TestStrictAlt_TimerGuardFires covers honest snapshot blocking: the
// receive guard never matches (nothing is queued), so the alt blocks and
// the timer guard fires for real — instead of the approximate model
// fabricating a verdict. Uses a short real timer so the test is fast.
func TestStrictAlt_TimerGuardFires(t *testing.T) {
	start := time.Now()
	v, reason := runStrict(t, "M.tc", `module M {
		type port P message { inout charstring }
		type component C { port P p }
		testcase tc() runs on C system C {
			timer t := 0.05;
			t.start;
			alt {
				[] p.receive(charstring:"never") { setverdict(fail, "unexpected message"); }
				[] t.timeout { setverdict(pass); }
			}
		}
	}`)
	if v != runtime.PassVerdict {
		t.Fatalf("verdict = %s (%s), want pass via the timer guard", v, reason)
	}
	// The timer really elapses (~50ms); the approximate heuristic would
	// return in ~0ms by faking the branch.
	if elapsed := time.Since(start); elapsed < 40*time.Millisecond {
		t.Fatalf("alt returned in %v; the strict timer guard should have waited ~50ms", elapsed)
	}
}

// TestStrictAlt_ContextCancelsBlockedAlt covers cancellable execution: a
// strict alt with only a receive guard that never matches blocks
// forever; a deadline context must stop the run so the goroutine
// terminates (no leak / hang) rather than spinning on the backstop.
func TestStrictAlt_ContextCancelsBlockedAlt(t *testing.T) {
	src := `module M {
		type port P message { inout charstring }
		type component C { port P p }
		testcase tc() runs on C system C {
			alt {
				[] p.receive(charstring:"never") { setverdict(pass); }
			}
		}
	}`
	tree := parse(t, src)
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	done := make(chan struct{})
	go func() {
		_, _, _ = interpreter.RunTestcaseWith([]*ttcn3.Tree{tree}, "M.tc",
			interpreter.TestcaseOptions{Profile: runtime.ProfileStrict, Context: ctx})
		close(done)
	}()

	select {
	case <-done:
		// good: the blocked alt was cancelled and the run returned.
	case <-time.After(3 * time.Second):
		t.Fatal("blocked strict alt was not cancelled by context (leak/hang)")
	}
}
