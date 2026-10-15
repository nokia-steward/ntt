package interpreter_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/nokia/ntt/interpreter"
	"github.com/nokia/ntt/runtime"
	"github.com/nokia/ntt/ttcn3"
)

// runSched runs src under the discrete-event scheduler with a real-time
// safety net, the configuration the conformance harness uses.
func runSched(t *testing.T, qname, src string) (runtime.Verdict, string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	v, reason, err := interpreter.RunTestcaseWith([]*ttcn3.Tree{parse(t, src)}, qname,
		interpreter.TestcaseOptions{DeterministicScheduler: true, DeterministicClock: true, Context: ctx})
	if err != nil {
		t.Fatalf("RunTestcaseWith(%s): %v", qname, err)
	}
	return v, reason
}

// An alt that can never match, with no timer guard and no other component
// to send anything, is a terminal deadlock: the test system failed, so the
// verdict is `error` rather than the alt quietly concluding as though
// nothing had matched.
const deadlockSrc = `module M {
	type port P message { inout integer }
	type component C { port P p }
	testcase tc() runs on C {
		alt {
			[] p.receive(integer:1) { setverdict(pass); }
		}
	}
}`

func TestDeadlock_ReportedAsError(t *testing.T) {
	v, reason := runSched(t, "M.tc", deadlockSrc)
	if v != runtime.ErrorVerdict {
		t.Fatalf("verdict = %s (%s), want error", v, reason)
	}
	if !strings.Contains(reason, "deadlock") {
		t.Fatalf("reason = %q, want it to name the deadlock", reason)
	}
}

// The deadlock inference rests on the loopback model having no outside. An
// external port driver breaks that - a real peer may still send - so the
// deadlock must NOT be reported there, and the participants conclude as
// they always did.
//
// Asserted as "not an error" rather than against a particular verdict,
// because what a concluded alt leaves behind is a separate question from
// the one this test asks.
func TestDeadlock_NotReportedWhenAnExternalDriverIsBound(t *testing.T) {
	prev := runtime.SetPortDriverProvider(func(typeName, instName string) runtime.PortDriver {
		return fakeServerDriver{}
	})
	t.Cleanup(func() { runtime.SetPortDriverProvider(prev) })

	v, reason := runSched(t, "M.tc", deadlockSrc)
	if v == runtime.ErrorVerdict {
		t.Fatalf("verdict = %s (%s), want anything but error: with a driver bound a late message is "+
			"still possible, so quiescence does not prove a deadlock", v, reason)
	}
}

// A deadlock outranks a verdict the body had already set. `error` is the
// top of the aggregation order and this is the intended reading: a verdict
// reached before the test system broke is not worth reporting.
func TestDeadlock_OutranksAnEarlierPass(t *testing.T) {
	v, reason := runSched(t, "M.tc", `module M {
		type port P message { inout integer }
		type component C { port P p }
		testcase tc() runs on C {
			setverdict(pass);
			alt {
				[] p.receive(integer:1) { setverdict(pass); }
			}
		}
	}`)
	if v != runtime.ErrorVerdict {
		t.Fatalf("verdict = %s (%s), want error to override the earlier pass", v, reason)
	}
}

// A timer guard gives the scheduler a finite deadline, so the clock
// advances and the timer fires. That is not a deadlock, and nothing here
// should report one.
func TestDeadlock_NotReportedWhenATimerCanStillFire(t *testing.T) {
	v, reason := runSched(t, "M.tc", `module M {
		type port P message { inout integer }
		type component C { port P p }
		testcase tc() runs on C {
			timer t := 1.0; t.start;
			alt {
				[] p.receive(integer:1) { setverdict(fail, "nothing was ever sent"); }
				[] t.timeout { setverdict(pass); }
			}
		}
	}`)
	if v != runtime.PassVerdict {
		t.Fatalf("verdict = %s (%s), want pass: a running timer is a finite deadline, not a deadlock", v, reason)
	}
}
