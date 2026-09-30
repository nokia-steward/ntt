package interpreter_test

import (
	"testing"

	"github.com/nokia/ntt/interpreter"
	"github.com/nokia/ntt/runtime"
	"github.com/nokia/ntt/ttcn3"
)

// TestStartedBodiesRun: a started behaviour runs as its component, on
// either clock (ETSI 21.3.2). Bodies that wait on a timer and use no port,
// and loops that break, used to be modelled on the virtual clock instead:
// their statements, a setverdict among them, never ran.
func TestStartedBodiesRun(t *testing.T) {
	for name, src := range map[string]string{
		"timer only": `module M {
			type component C {}
			function f() runs on C { timer t := 0.2; t.start; t.timeout; setverdict(pass); }
			testcase tc() runs on C system C {
				var C c := C.create;
				c.start(f());
				c.done;
			}
		}`,
		"loop that breaks": `module M {
			type component C {}
			function f() runs on C { var integer i := 0; while (true) { i := i + 1; if (i > 3) { break } } setverdict(pass); }
			testcase tc() runs on C system C {
				var C c := C.create;
				c.start(f());
				timer g := 2.0; g.start;
				alt { [] all component.done {} [] g.timeout { setverdict(fail, "the loop did not end") } }
			}
		}`,
		// A computation that never ends and never waits is modelled: it
		// runs until it is stopped, and does not hold up the others.
		"endless loop": `module M {
			type component C {}
			function f() runs on C { while (true) {} }
			testcase tc() runs on C system C {
				var C c := C.create, d := C.create alive;
				c.start(f());
				d.start(f());
				timer t := 0.1; t.start; t.timeout;
				if (c.running and d.running) { setverdict(pass) } else { setverdict(fail, "not running") }
				c.stop;
				d.stop;
				if (c.running or d.running) { setverdict(fail, "still running after stop") }
			}
		}`,
		// A loop that never ends is found where it is, whatever else the
		// body does: one in a called function, and one whose break is an
		// inner loop's, wait for their stop; the one on a branch not taken
		// is never entered, and what comes before one runs.
		"endless loop in a callee": `module M {
			type component C { var integer x := 0 }
			function spin() runs on C { while (true) { x := x + 1 } }
			function f() runs on C { spin(); }
			testcase tc() runs on C system C {
				var C c := C.create;
				c.start(f());
				timer t := 0.1; t.start; t.timeout;
				if (c.running) { setverdict(pass) }
				c.stop;
			}
		}`,
		"inner break": `module M {
			type component C { var integer x := 0 }
			function f() runs on C { while (true) { for (var integer i := 0; i < 2; i := i + 1) { if (i == 1) { break } } x := x + 1 } }
			testcase tc() runs on C system C {
				var C c := C.create;
				c.start(f());
				timer t := 0.1; t.start; t.timeout;
				if (c.running) { setverdict(pass) }
				c.stop;
			}
		}`,
		"branch not taken": `module M {
			type port P message { inout integer }
			type component C { port P p; var integer x := 0 }
			function f(integer mode) runs on C { if (mode == 1) { while (true) { x := x + 1 } } p.send(42); }
			testcase tc() runs on C system C {
				var C c := C.create;
				connect(self:p, c:p);
				c.start(f(0));
				timer g := 2.0; g.start;
				alt { [] p.receive(integer:42) { setverdict(pass) } [] g.timeout { setverdict(fail, "no message") } }
			}
		}`,
		"work before the loop": `module M {
			type port P message { inout integer }
			type component C { port P p; var integer x := 0 }
			function f() runs on C { p.send(42); while (true) { x := x + 1 } }
			testcase tc() runs on C system C {
				var C c := C.create;
				connect(self:p, c:p);
				c.start(f());
				timer g := 2.0; g.start;
				alt { [] p.receive(integer:42) { setverdict(pass) } [] g.timeout { setverdict(fail, "no message") } }
				c.stop;
			}
		}`,
		// Two PTCs exchanging messages for good when the MTC ends: each
		// gets turns, then they are stopped.
		"ping-pong at the end": `module M {
			type port P message { inout integer }
			type component C { port P p }
			function ping() runs on C { var integer v; p.send(0); while (true) { p.receive(integer:?) -> value v; p.send(v + 1) } }
			function pong() runs on C { var integer v; while (true) { p.receive(integer:?) -> value v; p.send(v + 1) } }
			testcase tc() runs on C system C {
				var C a := C.create, b := C.create;
				connect(a:p, b:p);
				a.start(ping());
				b.start(pong());
				setverdict(pass);
			}
		}`,
		// A PTC started just before the MTC's behaviour ends gets its turn
		// before the PTCs still running are stopped.
		"started at the end": `module M {
			type component C {}
			function f() runs on C { setverdict(pass); }
			testcase tc() runs on C system C {
				var C c := C.create;
				c.start(f());
			}
		}`,
	} {
		for _, k := range clocks {
			v, reason, err := interpreter.RunTestcaseWith([]*ttcn3.Tree{parse(t, src)}, "M.tc", k.opts)
			if err != nil || v != runtime.PassVerdict {
				t.Errorf("%s, %s clock: %s (%s) %v", name, k.name, v, reason, err)
			}
		}
	}
}

// TestStartWithAnObjectReferenceIsAnError: an object lives in the
// component that created it; a behaviour started on another component
// cannot be given a reference to it (ETSI ES 203 790 5.1.2.2).
func TestStartWithAnObjectReferenceIsAnError(t *testing.T) {
	src := `module M "TTCN-3:2018 Object-Oriented" {
		type component C {}
		type class K { var integer x; public function setX(integer v) { this.x := v } }
		type record of K KL;
		function f(K k) runs on C { k.setX(1); }
		function g(KL l) runs on C { l[0].setX(1); }
		testcase tc() runs on C {
			var K k := K.create(7);
			var C c := C.create;
			c.start(f(k));
			setverdict(pass);
		}
		testcase tc_part() runs on C {
			var K k := K.create(7);
			var KL l := { k };
			var C c := C.create;
			c.start(g(l));
			setverdict(pass);
		}
	}`
	for _, tc := range []string{"M.tc", "M.tc_part"} {
		for _, k := range clocks {
			v, reason, _ := interpreter.RunTestcaseWith([]*ttcn3.Tree{parse(t, src)}, tc, k.opts)
			if v != runtime.ErrorVerdict {
				t.Errorf("%s, %s clock: %s (%s), want error", tc, k.name, v, reason)
			}
		}
	}
}
