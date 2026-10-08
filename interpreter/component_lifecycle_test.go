package interpreter_test

import (
	"testing"

	"github.com/nokia/ntt/interpreter"
	"github.com/nokia/ntt/runtime"
	"github.com/nokia/ntt/ttcn3"
)

// TestRestartAfterStop: `stop` on a component created with `alive` only
// suspends it, and it stays reusable (ETSI 21.3.3), so the second body
// runs and sets pass. Mirrors Sem_210303_Stop_test_component_005..010.
//
// Two layers kept it from running. `start` did not clear the component's
// `done` flag, so the following `v_ptc.done` was satisfied by the previous
// run. And the stopped behaviour, unwinding without the scheduler's token,
// was taken for a deadlock by the starter waiting for it, and, finishing,
// finished the new behaviour in its place.
func TestRestartAfterStop(t *testing.T) {
	src := `module M {
		type component C {}
		function f_first() runs on C { timer t := 1.0; t.start; t.timeout; }
		function f_second() runs on C { setverdict(pass); }
		testcase tc() runs on C system C {
			var C v_ptc := C.create("PTC") alive;
			v_ptc.start(f_first());
			v_ptc.stop;
			v_ptc.start(f_second());
			v_ptc.done;
		}
	}`
	for _, k := range clocks {
		v, reason, err := interpreter.RunTestcaseWith([]*ttcn3.Tree{parse(t, src)}, "M.tc", k.opts)
		if err != nil || v != runtime.PassVerdict {
			t.Errorf("%s clock: %s (%s) %v", k.name, v, reason, err)
		}
	}
}

// TestRestartBeginsWithoutDefaults: the defaults a behaviour activated
// end with it (ETSI 21.3.2), so an alive component started again begins
// with none. Mirrors Sem_210302_Start_test_component_009.
func TestRestartBeginsWithoutDefaults(t *testing.T) {
	src := `module M {
		type port P message { inout integer }
		type component C { timer t := 0.5; port P p }
		altstep a() runs on C { [] p.receive(integer:?) { setverdict(fail, "the first behaviour's default fired") } }
		function first() runs on C { activate(a()); }
		function second() runs on C { t.start; alt { [] t.timeout { setverdict(pass) } } }
		testcase tc() runs on C system C {
			var C v_ptc := C.create("PTC") alive;
			connect(self:p, v_ptc:p);
			v_ptc.start(first());
			v_ptc.done;
			p.send(2);
			v_ptc.start(second());
			v_ptc.done;
		}
	}`
	for _, k := range clocks {
		v, reason, err := interpreter.RunTestcaseWith([]*ttcn3.Tree{parse(t, src)}, "M.tc", k.opts)
		if err != nil || v != runtime.PassVerdict {
			t.Errorf("%s clock: %s (%s) %v", k.name, v, reason, err)
		}
	}
}

func TestAliveComponentStopPreservesStateAfterConnectedSend(t *testing.T) {
	tree := parse(t, `module ComponentLifecycleSmoke {
		type port P message { inout charstring; }
		type component C {
			var integer vc_val := 0;
			port P p;
		}

		function first() runs on C {
			vc_val := 1;
			p.send("ready");
			p.receive(charstring:?);
			setverdict(fail, "PTC consumed its own connected send");
		}

		function second() runs on C {
			if (vc_val == 1) {
				setverdict(pass);
			} else {
				setverdict(fail, "state was not preserved", vc_val);
			}
		}

		testcase tc_alive_stop_state() runs on C system C {
			var C v_ptc := C.create("ptc") alive;
			connect(self:p, v_ptc:p);
			v_ptc.start(first());
			p.receive(charstring:?);
			v_ptc.stop;
			v_ptc.start(second());
			v_ptc.done;
		}
	}`)
	v, reason, err := interpreter.RunTestcase([]*ttcn3.Tree{tree}, "ComponentLifecycleSmoke.tc_alive_stop_state")
	if err != nil {
		t.Fatalf("RunTestcase: %v", err)
	}
	if v != runtime.PassVerdict {
		t.Fatalf("verdict = %s (%s), want pass", v, reason)
	}
}

func TestComponentCallRedirectsCompletedOnly(t *testing.T) {
	tree := parse(t, `module ComponentCallRedirectSmoke {
		type component C {}

		function complete() runs on C return integer {
			setverdict(pass);
			return 1;
		}

		function incomplete() runs on C {
			setverdict(pass);
			stop;
		}

		testcase tc_component_call_redirects() runs on C system C {
			var C v_ptc := C.create;
			var integer v_ret := 0;
			var verdicttype v_verdict := none;
			v_ptc.call(complete()) -> value v_ret;
			v_ptc := C.create;
			v_ptc.call(incomplete()) -> verdict v_verdict catch(stop) {}
			if (v_ret == 1 and v_verdict == none) {
				setverdict(pass);
			} else {
				setverdict(fail, "unexpected redirects", v_ret, v_verdict);
			}
		}
	}`)
	v, reason, err := interpreter.RunTestcase([]*ttcn3.Tree{tree}, "ComponentCallRedirectSmoke.tc_component_call_redirects")
	if err != nil {
		t.Fatalf("RunTestcase: %v", err)
	}
	if v != runtime.PassVerdict {
		t.Fatalf("verdict = %s (%s), want pass", v, reason)
	}
}

func TestComponentCallWritesBackOutAndInoutParams(t *testing.T) {
	tree := parse(t, `module ComponentCallWritebackSmoke {
		type component C {}

		function update(out integer p_out, inout integer p_inout) runs on C {
			p_out := 10;
			p_inout := p_inout * 2;
			setverdict(pass);
		}

		testcase tc_component_call_writeback() runs on C system C {
			var C v_ptc := C.create;
			var integer v_out := 0;
			var integer v_inout := 3;
			v_ptc.call(update(v_out, v_inout));
			if (v_out == 10 and v_inout == 6) {
				setverdict(pass);
			} else {
				setverdict(fail, "writeback", v_out, v_inout);
			}
		}
	}`)
	v, reason, err := interpreter.RunTestcase([]*ttcn3.Tree{tree}, "ComponentCallWritebackSmoke.tc_component_call_writeback")
	if err != nil {
		t.Fatalf("RunTestcase: %v", err)
	}
	if v != runtime.PassVerdict {
		t.Fatalf("verdict = %s (%s), want pass", v, reason)
	}
}

func TestBareUnmapClearsOnlyCurrentComponentMappings(t *testing.T) {
	tree := parse(t, `module ComponentBareUnmapSmoke {
		type port P message { inout integer; }
		type component C { port P p1, p2; }

		function checkMapped(boolean p_expected) runs on C {
			if (p1.checkstate("Mapped") == p_expected and p2.checkstate("Mapped") == p_expected) {
				setverdict(pass);
			} else {
				setverdict(fail);
			}
		}

		testcase tc_bare_unmap() runs on C system C {
			var C v_ptc := C.create;
			map(self:p1, system:p1);
			map(self:p2, system:p2);
			map(v_ptc:p1, system:p1);
			map(v_ptc:p2, system:p2);
			unmap;
			checkMapped(false);
			v_ptc.start(checkMapped(true));
			v_ptc.done;
		}
	}`)
	v, reason, err := interpreter.RunTestcase([]*ttcn3.Tree{tree}, "ComponentBareUnmapSmoke.tc_bare_unmap")
	if err != nil {
		t.Fatalf("RunTestcase: %v", err)
	}
	if v != runtime.PassVerdict {
		t.Fatalf("verdict = %s (%s), want pass", v, reason)
	}
}
