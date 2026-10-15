package interpreter_test

import (
	"context"
	"testing"
	"time"

	"github.com/nokia/ntt/interpreter"
	"github.com/nokia/ntt/runtime"
	"github.com/nokia/ntt/ttcn3"
)

// A `stop` in an altstep body reached as an activated default ends the
// component, so the statements after the alt statement never run - ETSI
// 20.5.1 / 21.3.3, Sem_200501_the_default_mechanism_008. The testcase
// below fails loudly if control returns from the alt.
func TestDefaultMechanism_StopEndsTheComponent(t *testing.T) {
	runPass(t, "M.tc", `module M {
		type port P message { inout integer }
		type component C { port P p }
		altstep a() runs on C {
			[] p.receive(integer:?) {
				setverdict(pass);
				stop;
			}
		}
		testcase tc() runs on C {
			activate(a());
			p.send(integer:5);
			alt {
				[] p.receive(integer:1) { setverdict(fail); }
			}
			setverdict(fail, "component stop expected");
		}
	}`)
}

// The same rule on a PTC: its default stops only that component, so the
// PTC's post-alt code is skipped while the MTC carries on to observe it
// as done and set the verdict itself.
func TestDefaultMechanism_StopOnPTCLeavesTheMTCRunning(t *testing.T) {
	src := `module M {
		type port P message { inout integer }
		type component C { port P p }
		altstep a() runs on C {
			[] p.receive(integer:?) { stop; }
		}
		function f_ptc() runs on C {
			activate(a());
			alt {
				[] p.receive(integer:1) { setverdict(fail); }
			}
			setverdict(fail, "component stop expected");
		}
		testcase tc() runs on C {
			var C v_ptc := C.create;
			connect(self:p, v_ptc:p);
			v_ptc.start(f_ptc());
			p.send(integer:5);
			v_ptc.done;
			setverdict(pass);
		}
	}`
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	v, reason, err := interpreter.RunTestcaseWith([]*ttcn3.Tree{parse(t, src)}, "M.tc",
		interpreter.TestcaseOptions{DeterministicScheduler: true, DeterministicClock: true, Context: ctx})
	if err != nil {
		t.Fatalf("RunTestcaseWith: %v", err)
	}
	if v != runtime.PassVerdict {
		t.Fatalf("verdict = %s (%s), want pass", v, reason)
	}
}

// A component created with the `alive` modifier survives a `stop`: the
// behaviour unwinds but the component stays usable for another start,
// which is what distinguishes stop from kill (ETSI 21.3.3).
func TestDefaultMechanism_StopKeepsAnAliveComponentReusable(t *testing.T) {
	tree := parse(t, `module M {
		type component C {}
		function f_first() runs on C { stop; }
		function f_second() runs on C { setverdict(pass); }
		testcase tc() runs on C {
			var C v_ptc := C.create alive;
			v_ptc.start(f_first());
			v_ptc.done;
			if (v_ptc.alive) { setverdict(pass); }
			else { setverdict(fail, "alive component must survive stop"); }
		}
	}`)
	v, reason, err := interpreter.RunTestcase([]*ttcn3.Tree{tree}, "M.tc")
	if err != nil {
		t.Fatalf("RunTestcase: %v", err)
	}
	if v != runtime.PassVerdict {
		t.Fatalf("verdict = %s (%s), want pass", v, reason)
	}
}
