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
		// On the virtual clock computing takes no time; a component that
		// computes for long lets the others run, and time pass to the next
		// timer. A busy wait on another component ends, two spinning
		// components leave the MTC's timer to fire, and a loop over a
		// timer of no length does not hold everyone.
		"busy wait on another": `module M {
			type component C {}
			function short() runs on C { var integer i := 0; while (i < 10) { i := i + 1 } }
			function waiter(C o) runs on C { while (true) { if (not o.running) { break } } setverdict(pass); }
			testcase tc() runs on C system C {
				var C o := C.create, w := C.create;
				o.start(short());
				w.start(waiter(o));
				w.done;
			}
		}`,
		"two spinning": `module M {
			type component C {}
			function f() runs on C { while (true) {} }
			testcase tc() runs on C system C {
				var C a := C.create, b := C.create;
				a.start(f());
				b.start(f());
				timer t := 0.2; t.start; t.timeout;
				setverdict(pass);
				a.stop;
				b.stop;
			}
		}`,
		"zero-length timer loop": `module M {
			type component C { timer z }
			function f() runs on C { while (true) { z.start(0.0); z.timeout; } }
			testcase tc() runs on C system C {
				var C c := C.create;
				c.start(f());
				timer g := 0.5; g.start; g.timeout;
				setverdict(pass);
				c.stop;
			}
		}`,
		// A computation that ends takes no time on the virtual clock: a
		// timer does not fire while it runs, the component's own included,
		// nor while a loop evaluates an alt's guard.
		"a computation takes no time": `module M {
			type component C {}
			function work(integer n) runs on C { var integer i := 0; while (i < n) { i := i + 1 } setverdict(pass) }
			function sleeper() runs on C { timer w := 2.0; w.start; w.timeout; }
			testcase tc() runs on C system C {
				var C c := C.create, s := C.create;
				s.start(sleeper());
				c.start(work(50000));
				timer g := 1.0; g.start;
				var integer i := 0; while (i < 2500) { i := i + 1 }
				if (not g.running) { setverdict(fail, "the MTC's loop took time") }
				alt { [] c.done {} [] g.timeout { setverdict(fail, "the PTC's loop took time") } }
				s.stop;
			}
		}`,
		// A computation that goes on for long takes some time at last,
		// but not all there is to the next timer: a watchdog far off
		// outlasts it, and a testcase limit is not reached.
		"a long computation takes some time": `module M {
			type port P message { inout integer }
			type component C { port P p }
			function work() runs on C { var integer s := 0; for (var integer i := 0; i < 1100000; i := i + 1) { s := s + 1 } setverdict(pass) }
			testcase tc() runs on C system C {
				var C c := C.create;
				c.start(work());
				timer guard := 60.0; guard.start;
				alt { [] c.done {} [] guard.timeout { setverdict(fail, "watchdog") } }
			}
			control { execute(tc(), 100.0) }
		}`,
		// Two components exchanging messages for good, each waiting for
		// the other's, still let the MTC's timer fire.
		"exchanging for good": `module M {
			type port P message { inout integer }
			type component C { port P p }
			function ping() runs on C { var integer v; p.send(0); while (true) { p.receive(integer:?) -> value v; p.send(v + 1) } }
			function pong() runs on C { var integer v; while (true) { p.receive(integer:?) -> value v; p.send(v + 1) } }
			testcase tc() runs on C system C {
				var C a := C.create, b := C.create;
				connect(a:p, b:p);
				a.start(ping()); b.start(pong());
				timer g := 1.0; g.start; g.timeout;
				setverdict(pass);
				all component.stop;
			}
		}`,
		// When time passes for an exchange at one instant, it stops at
		// the timer due first — a woken component's included — so two
		// timers still expire in order.
		"timers in order while exchanging": `module M {
			type port P message { inout integer }
			type component C { port P p }
			function echo() runs on C { var integer v; while (true) { p.receive(integer:?) -> value v; p.send(v) } }
			function x() runs on C {
				timer t1 := 1.5, t2 := 1.8; t1.start; t2.start;
				alt { [] t2.timeout { setverdict(fail, "t2 before t1") } [] t1.timeout { setverdict(pass) } }
			}
			testcase tc() runs on C system C {
				var C e := C.create, d := C.create;
				connect(self:p, e:p); e.start(echo());
				d.start(x());
				var integer v;
				for (var integer i := 0; i < 120000; i := i + 1) { p.send(i); p.receive(integer:?) -> value v; }
				d.done;
				setverdict(pass);
			}
		}`,
		// Two components keeping each other busy do not keep a third
		// from running: its answer comes at once, as on the real clock.
		"a third component while two exchange": `module M {
			type port P message { inout integer, charstring }
			type component C { port P p, q }
			function echo() runs on C { var integer v; while (true) { p.receive(integer:?) -> value v; p.send(v) } }
			function replier() runs on C { q.receive(charstring:"ping"); q.send("pong") }
			testcase tc() runs on C system C {
				var C a := C.create, w := C.create;
				connect(self:p, a:p); connect(self:q, w:q);
				a.start(echo()); w.start(replier());
				timer guard := 5.0; guard.start;
				q.send("ping"); p.send(0);
				var integer v;
				alt {
					[] p.receive(integer:?) -> value v { p.send(v + 1); repeat }
					[] q.receive(charstring:"pong") { if (guard.read > 0.5) { setverdict(fail, "pong at ", guard.read) } else { setverdict(pass) } }
					[] guard.timeout { setverdict(fail, "no pong") }
				}
				a.stop;
			}
		}`,
		"a loop in a guard": `module M {
			type port P message { inout integer }
			type component C { port P p }
			function long() return boolean { var integer i := 0; while (i < 2500) { i := i + 1 } return true }
			function late() runs on C { timer w := 2.0; w.start; w.timeout; p.send(1) }
			testcase tc() runs on C system C {
				var C c := C.create; connect(mtc:p, c:p);
				c.start(late());
				timer t := 1.0; t.start;
				alt { [long()] p.receive(integer:1) { setverdict(fail, "taken after the timer expired") } [] t.timeout { setverdict(pass) } }
				c.stop;
			}
		}`,
		// A loop that ends, however it ends, runs to its end.
		"return in an inner loop": `module M {
			type component C { var integer x := 0 }
			function g() runs on C { while (true) { for (var integer i := 0; i < 2; i := i + 1) { if (x > 5) { return } } x := x + 1 } }
			function f() runs on C { g(); setverdict(pass); }
			testcase tc() runs on C system C {
				var C c := C.create;
				c.start(f());
				timer t := 2.0; t.start;
				alt { [] c.done {} [] t.timeout { setverdict(fail, "the loop did not end") } }
			}
		}`,
		"loop that kills another": `module M {
			type component C { var integer x := 0 }
			function spin() runs on C { while (true) {} }
			function f(C v) runs on C { while (true) { x := x + 1; if (x == 3) { v.kill; break } } setverdict(pass); }
			testcase tc() runs on C system C {
				var C v := C.create, c := C.create;
				v.start(spin());
				c.start(f(v));
				timer t := 2.0; t.start;
				alt { [] v.killed {} [] t.timeout { setverdict(fail, "not killed") } }
				c.done;
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

// TestComputingAfterTheMTCEnds: a PTC that computes when the MTC's
// behaviour ends finishes a finite computation, and the verdict it then
// sets counts, on either clock.
func TestComputingAfterTheMTCEnds(t *testing.T) {
	src := `module M {
		type component C {}
		function f() runs on C { var integer i := 0; while (i < 3000) { i := i + 1 } setverdict(fail, "late") }
		testcase tc() runs on C system C { var C c := C.create; c.start(f()); setverdict(pass); }
	}`
	for _, k := range clocks {
		v, reason, err := interpreter.RunTestcaseWith([]*ttcn3.Tree{parse(t, src)}, "M.tc", k.opts)
		if err != nil || v != runtime.FailVerdict {
			t.Errorf("%s clock: %s (%s) %v, want fail", k.name, v, reason, err)
		}
	}
}
