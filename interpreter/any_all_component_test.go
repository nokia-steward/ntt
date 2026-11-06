package interpreter_test

import (
	"strings"
	"testing"

	"github.com/nokia/ntt/interpreter"
	"github.com/nokia/ntt/runtime"
	"github.com/nokia/ntt/ttcn3"
)

// any component and all component with alive and killed ask every PTC,
// an inactive one too (ETSI 21.3.5, 21.3.8); with running and all
// component.done only those ever started (21.3.6, 21.3.7). all
// component.kill kills every PTC (21.3.4), all component.stop the
// started ones (21.3.3).
func TestAnyAllComponentStates(t *testing.T) {
	v, reason := runSched(t, "M.tc", `module M {
		type component C {}
		function f_wait() runs on C { timer t := 5.0; t.start; t.timeout }
		function want(boolean b, charstring what) runs on C {
			if (not b) { setverdict(fail, what); stop }
		}
		testcase tc() runs on C {
			var C v_inactive := C.create;
			want(any component.alive and all component.alive, "an inactive PTC is alive");
			want(not any component.running, "an inactive PTC isn't running");
			all component.done;

			var C v_run := C.create alive;
			v_run.start(f_wait());
			want(all component.running, "all component.running ignores the inactive PTC");
			all component.stop;
			want(v_inactive.alive, "all component.stop leaves an inactive PTC alive");

			var C v_other := C.create;
			v_other.kill;
			want(any component.alive and not all component.alive, "one PTC killed");
			all component.kill;
			want(not any component.alive, "all component.kill kills the inactive PTC");
			all component.killed;
			setverdict(pass);
		}
	}`)
	if v != runtime.PassVerdict {
		t.Fatalf("verdict = %s (%s), want pass", v, reason)
	}
}

// any component.done matches a PTC killed before it ever started, or
// stopped, an alive-type one: its behaviour has been killed or stopped
// (21.3.7).
func TestAnyComponentDoneNeverStarted(t *testing.T) {
	for _, end := range []string{"var C v := C.create; v.kill;", "var C v := C.create alive; v.stop;"} {
		v, reason := runSched(t, "M.tc", `module M {
			type component C {}
			testcase tc() runs on C {
				`+end+`
				timer g := 1.0;
				g.start;
				alt {
					[] any component.done { setverdict(pass) }
					[] g.timeout { setverdict(fail, "any component.done didn't match") }
				}
			}
		}`)
		if v != runtime.PassVerdict {
			t.Errorf("%s: verdict = %s (%s), want pass", end, v, reason)
		}
	}
}

// any component and all component are the MTC's (21.3.9, table 21): on
// a PTC each, a query too, is an error.
func TestAnyAllComponentOnAPTC(t *testing.T) {
	for _, op := range []string{"if (any component.alive) {}", "all component.killed", "all component.kill", "all component.stop"} {
		v, reason := runSched(t, "M.tc", `module M {
			type component C {}
			function f() runs on C { `+op+` }
			testcase tc() runs on C {
				var C v := C.create;
				v.start(f());
				v.done;
				if (mtc.alive) { setverdict(pass) }
			}
		}`)
		if v != runtime.ErrorVerdict || !strings.Contains(reason, "MTC only") {
			t.Errorf("%s on a PTC: verdict = %s (%s), want an MTC-only error", op, v, reason)
		}
	}
}

// A PTC that never ran, killed by another one, ends the MTC's wait for it
// when it is killed, not when the killer ends, on either clock.
func TestAnyComponentKilledByAThirdParty(t *testing.T) {
	for _, op := range []string{"any component.killed", "any component.done"} {
		src := parse(t, `module M {
			type component C {}
			function k(C v) runs on C {
				timer y := 0.2; y.start; y.timeout;
				v.kill;
				timer z := 3.0; z.start; z.timeout;
			}
			testcase tc() runs on C {
				var C v_idle := C.create, v_killer := C.create alive;
				timer g := 1.0;
				v_killer.start(k(v_idle));
				g.start;
				`+op+`;
				if (g.running) { setverdict(pass) } else { setverdict(fail, "woken only when the killer ended") }
				v_killer.kill;
			}
		}`)
		for _, k := range clocks {
			v, reason, err := interpreter.RunTestcaseWith([]*ttcn3.Tree{src}, "M.tc", k.opts)
			if err != nil || v != runtime.PassVerdict {
				t.Errorf("%s, %s clock: %s (%s) %v", op, k.name, v, reason, err)
			}
		}
	}
}

// A wait for a PTC that has never run ends when the behaviour another
// component starts on it ends, not when that component ends, on either
// clock.
func TestWaitForAPTCStartedByAThirdParty(t *testing.T) {
	for _, op := range []string{"v.done", "v.killed", "any component.killed"} {
		src := parse(t, `module M {
			type component C {}
			function f_short() runs on C { timer s := 0.1; s.start; s.timeout }
			function f_starter(C v) runs on C {
				timer y := 0.2; y.start; y.timeout;
				v.start(f_short());
				timer z := 3.0; z.start; z.timeout;
			}
			testcase tc() runs on C {
				var C v := C.create, k := C.create alive;
				timer g := 1.0;
				k.start(f_starter(v));
				g.start;
				`+op+`;
				if (g.running) { setverdict(pass) } else { setverdict(fail, "woken only when the starter ended") }
				k.kill;
			}
		}`)
		for _, k := range clocks {
			v, reason, err := interpreter.RunTestcaseWith([]*ttcn3.Tree{src}, "M.tc", k.opts)
			if err != nil || v != runtime.PassVerdict {
				t.Errorf("%s, %s clock: %s (%s) %v", op, k.name, v, reason, err)
			}
		}
	}
}

// A wait that begins while another component is starting a behaviour on
// the PTC, the arguments still being evaluated, waits for that behaviour,
// on either clock.
func TestWaitWhileAPTCIsBeingStarted(t *testing.T) {
	for _, op := range []string{"v.done", "any component.killed"} {
		src := parse(t, `module M {
			type component C {}
			function f_short(integer i) runs on C { timer s := 0.1; s.start; s.timeout }
			function slow() runs on C return integer { timer w := 0.4; w.start; w.timeout; return 1 }
			function f_starter(C v) runs on C {
				v.start(f_short(slow()));
				timer z := 3.0; z.start; z.timeout;
			}
			testcase tc() runs on C {
				var C v := C.create, k := C.create alive;
				k.start(f_starter(v));
				timer d := 0.1; d.start; d.timeout;
				timer g := 2.0; g.start;
				`+op+`;
				if (not g.running) { setverdict(fail, "woken only when the starter ended") }
				else {
					alt {
						[] v.done { setverdict(pass) }
						[else] { setverdict(fail, "the wait ended before v was done") }
					}
				}
				k.kill;
			}
		}`)
		for _, k := range clocks {
			v, reason, err := interpreter.RunTestcaseWith([]*ttcn3.Tree{src}, "M.tc", k.opts)
			if err != nil || v != runtime.PassVerdict {
				t.Errorf("%s, %s clock: %s (%s) %v", op, k.name, v, reason, err)
			}
		}
	}
}
