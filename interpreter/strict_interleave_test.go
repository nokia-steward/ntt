package interpreter_test

import (
	"context"
	"testing"
	"time"

	"github.com/nokia/ntt/interpreter"
	"github.com/nokia/ntt/runtime"
	"github.com/nokia/ntt/ttcn3"
)

// TestStrictComp_ModeledDoneUsesVirtualClock covers a skipped finite-timer
// PTC's `.done` state under the deterministic clock: the MTC's observation
// window (`g.timeout`) advances VIRTUAL time, so completion of the modelled
// body must be measured against the virtual clock. A real-time measure would
// read ~0 (no wall time passed) and the PTC would never be seen as done.
// Mirrors Sem_210307_done_operation_00x.
func TestStrictComp_ModeledDoneUsesVirtualClock(t *testing.T) {
	src := `module M {
		type component C {}
		function f() runs on C { timer t := 1.0; t.start; t.timeout; }
		testcase tc() runs on C system C {
			var C p := C.create;
			timer g := 2.0;
			p.start(f());
			g.start;
			g.timeout;
			if (p.done) { setverdict(pass); }
			else { setverdict(fail, "modelled PTC not done after the virtual observation window"); }
		}
	}`
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	v, reason, err := interpreter.RunTestcaseWith([]*ttcn3.Tree{parse(t, src)}, "M.tc",
		interpreter.TestcaseOptions{Profile: runtime.ProfileStrict, DeterministicClock: true, Context: ctx})
	if err != nil {
		t.Fatalf("RunTestcaseWith: %v", err)
	}
	if v != runtime.PassVerdict {
		t.Fatalf("verdict = %s (%s), want pass (modelled done must use the virtual clock)", v, reason)
	}
}

// TestStrictAlt_AltstepGuardTimerConcludes covers a timer guard nested
// INSIDE an altstep-call alternative (`alt { [] a() }` where `a` has
// `[] t.timeout {}`). The strict block step must look THROUGH the altstep
// call to find t's deadline (nextAltTimerVirtualDeadline recurses into
// altstep guards); without that no deadline is found and the alt blocks
// forever. Mirrors Sem_1101_ValueVars_001 / Sem_160201_invoking_altsteps_004.
// A context bounds the run so a regression surfaces as a non-pass rather
// than a hang.
func TestStrictAlt_AltstepGuardTimerConcludes(t *testing.T) {
	src := `module M {
		type component C { timer t }
		altstep a() runs on C {
			[] t.timeout { setverdict(pass); }
		}
		testcase tc() runs on C system C {
			t.start(0.05);
			alt {
				[] a();
			}
		}
	}`
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	v, reason, err := interpreter.RunTestcaseWith([]*ttcn3.Tree{parse(t, src)}, "M.tc",
		interpreter.TestcaseOptions{Profile: runtime.ProfileStrict, DeterministicClock: true, Context: ctx})
	if err != nil {
		t.Fatalf("RunTestcaseWith: %v", err)
	}
	if v != runtime.PassVerdict {
		t.Fatalf("verdict = %s (%s), want pass via the altstep's nested timer guard", v, reason)
	}
}

// TestStrictInterleave_TakesEachBranchOnce covers the core interleave
// semantics (ETSI ES 201 873-1 §20.4): EVERY alternative is taken exactly
// once, unlike a plain alt which takes only one. Both messages are queued
// (loopback self-send), so a correct interleave consumes both and the
// counter reaches 11; the best-effort "take one branch" model would leave
// it at 1 or 10.
func TestStrictInterleave_TakesEachBranchOnce(t *testing.T) {
	v, reason := runStrict(t, "M.tc", `module M {
		type port P message { inout integer }
		type component C { port P p }
		testcase tc() runs on C system C {
			var integer c := 0;
			p.send(integer:1);
			p.send(integer:2);
			interleave {
				[] p.receive(integer:1) { c := c + 1; }
				[] p.receive(integer:2) { c := c + 10; }
			}
			if (c == 11) { setverdict(pass); }
			else { setverdict(fail, "interleave did not take both branches"); }
		}
	}`)
	if v != runtime.PassVerdict {
		t.Fatalf("verdict = %s (%s), want pass (interleave must take every branch once)", v, reason)
	}
}

// TestStrictInterleave_NoDefaultSuppressesDefaults covers `interleave
// @nodefault`: an activated default must NOT be invoked while the interleave
// blocks, so the timer alternative fires instead of the default's fail.
// Mirrors Sem_2004_InterleaveStatement_013. The deterministic clock lets the
// 3s timer fire instantly.
func TestStrictInterleave_NoDefaultSuppressesDefaults(t *testing.T) {
	src := `module M {
		type port P message { inout integer }
		type component C { port P p }
		altstep a() runs on C {
			[] p.receive(integer:?) { setverdict(fail, "default invoked despite @nodefault"); }
		}
		testcase tc() runs on C system C {
			timer t := 3.0;
			activate(a());
			t.start;
			p.send(integer:1);
			interleave @nodefault {
				[] p.receive(integer:5) { setverdict(fail, "matched a message that was never sent"); }
				[] t.timeout { setverdict(pass); break; }
			}
		}
	}`
	v, reason, err := interpreter.RunTestcaseWith([]*ttcn3.Tree{parse(t, src)}, "M.tc",
		interpreter.TestcaseOptions{Profile: runtime.ProfileStrict, DeterministicClock: true})
	if err != nil {
		t.Fatalf("RunTestcaseWith: %v", err)
	}
	if v != runtime.PassVerdict {
		t.Fatalf("verdict = %s (%s), want pass (@nodefault must suppress the default)", v, reason)
	}
}
