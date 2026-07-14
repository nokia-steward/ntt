package interpreter_test

import (
	"testing"

	"github.com/nokia/ntt/interpreter"
	"github.com/nokia/ntt/runtime"
	"github.com/nokia/ntt/ttcn3"
)

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
