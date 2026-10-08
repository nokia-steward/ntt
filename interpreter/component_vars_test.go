package interpreter_test

import (
	"testing"

	"github.com/nokia/ntt/interpreter"
	"github.com/nokia/ntt/runtime"
	"github.com/nokia/ntt/ttcn3"
)

// TestComponentVariables: a component's variables, constants and timers
// are its own (ETSI ES 201 873-1 6.2.10.4), and a function that runs on it
// — however it is reached — reads and writes the component's, on either
// clock.
func TestComponentVariables(t *testing.T) {
	for name, src := range map[string]string{
		// A function called from the behaviour a PTC was started with
		// sees the PTC's variables, not another component's.
		"through a called function": `module M {
			type record R { integer i }
			type component C { var charstring cs := "abc"; var integer ci := 0; var R cr := { i := 0 } }
			function g() runs on C return integer { cs[0] := "X"; ci := ci + 5; cr.i := 7; return ci }
			function h() runs on C {
				ci := 3;
				var integer seen := g();
				if (seen == 8 and ci == 8 and cs == "Xbc" and cr.i == 7) { setverdict(pass) } else { setverdict(fail, seen, ci, cs, cr) }
			}
			testcase tc() runs on C system C {
				var C c := C.create;
				c.start(h());
				c.done;
				if (ci != 0 or cs != "abc" or cr.i != 0) { setverdict(fail, "the MTC's changed: ", ci, cs, cr) }
				h();
			}
		}`,
		// Two components running the same functions keep their own.
		"two components": `module M {
			type component C { var integer n := 0; timer t := 0.1 }
			function bump(integer k) runs on C { n := n + k }
			function f(integer k) runs on C {
				bump(k); t.start; t.timeout; bump(k);
				if (n == 2 * k) { setverdict(pass) } else { setverdict(fail, "n=", n, " k=", k) }
			}
			testcase tc() runs on C system C {
				var C a := C.create, b := C.create;
				a.start(f(1)); b.start(f(10));
				all component.done;
				if (n != 0) { setverdict(fail, "MTC n=", n) }
			}
		}`,
		// A component variable is initialised when the component is: from
		// a constant, in the MTC too.
		"initialised from a constant": `module M {
			type record R { integer a, integer b }
			const R c_R := { a := 1, b := 2 };
			type component C { var R cr := c_R; var integer n := 3 }
			function get() runs on C return integer { return cr.a + n }
			testcase tc() runs on C system C {
				var C c := C.create;
				if (cr.a == 1 and get() == 4) { setverdict(pass) } else { setverdict(fail, cr, n) }
			}
		}`,
		// A component has the variables of the type it extends as its
		// own too (6.2.10.2).
		"inherited": `module M {
			type component B { var integer n := 1 }
			type component C extends B { var integer m := 2 }
			function g() runs on C { n := n + 10; m := m + 20 }
			function f() runs on C { g(); if (n == 11 and m == 22) { setverdict(pass) } else { setverdict(fail, "ptc n=", n, " m=", m) } }
			testcase tc() runs on C system C {
				var C c := C.create; c.start(f()); c.done;
				if (n != 1 or m != 2) { setverdict(fail, "MTC's changed: n=", n, " m=", m) }
				g();
				if (n != 11 or m != 22) { setverdict(fail, "MTC n=", n, " m=", m) }
			}
		}`,
		// A like-named component type of another module has its own
		// members, not the ones it extends there.
		"a like-named type elsewhere": `module X {
			type component B1 { var integer c_v := 999 }
			type component C extends B1 { var integer w := 0 }
		}
		module M {
			const integer c_v := 1;
			type component C { var integer w := 0 }
			function rd() runs on C return integer { return c_v }
			function ptc() runs on C { if (rd() != 1) { setverdict(fail, "PTC rd=", rd()) } }
			testcase tc() runs on C system C {
				var C c := C.create; c.start(ptc()); c.done;
				if (c_v == 1 and rd() == 1) { setverdict(pass) } else { setverdict(fail, "c_v=", c_v, " rd=", rd()) }
			}
		}`,
		// What the starter has in scope, a function running on the
		// started component does not see.
		"not the starter's names": `module B {
			const integer c_k := 1;
			type component C { var integer n := 0 }
			function nested() runs on C return integer { return c_k }
			function top() runs on C { if (nested() == 1) { setverdict(pass) } else { setverdict(fail, "nested=", nested()) } }
		}
		module M {
			import from B { type C; function top }
			testcase tc() runs on C system C {
				var integer c_k := 100;
				var C c := C.create; c.start(top()); c.done;
			}
		}`,
		// An initialiser calling a function that runs on the component
		// uses the component's own variables, in the MTC and in a PTC.
		"initialised by a function": `module M {
			type component D { var integer x := 3; var integer y := dbl() }
			function dbl() runs on D return integer { x := x + 1; return x * 2 }
			function ptcD() runs on D { if (y != 8 or x != 4) { setverdict(fail, "PTC y=", y, " x=", x) } }
			testcase tc() runs on D system D {
				if (y != 8 or x != 4) { setverdict(fail, "MTC y=", y, " x=", x) }
				x := 100;
				var D c := D.create; c.start(ptcD()); c.done;
				if (x == 100) { setverdict(pass) } else { setverdict(fail, "MTC x=", x) }
			}
		}`,
		// An initialiser that waits waits as the component it
		// initialises, not as the one that started it.
		"an initialiser that waits": `module M {
			type component MT {}
			type component C { timer t := 0.5; var integer a := slow() }
			function slow() runs on C return integer { t.start; t.timeout; return 7 }
			function ptc() runs on C { if (a == 7) { setverdict(pass) } else { setverdict(fail, "a=", a) } }
			testcase tc() runs on MT system MT { var C c := C.create; c.start(ptc()); c.done }
		}`,
		// __SCOPE__ names the function, or in an initialiser the
		// component type (ETSI D.5); and the module's attributes apply in
		// a started behaviour.
		"scope and attributes": `module M {
			type component C { var charstring s := __SCOPE__ }
			function ptc() runs on C {
				var integer x := 1;
				if (s == "C" and __SCOPE__ == "ptc" and x.encode == {"XML"}) { setverdict(pass) } else { setverdict(fail, s, __SCOPE__, x.encode) }
			}
			testcase tc() runs on C system C { var C c := C.create; c.start(ptc()); c.done }
		} with { encode "XML" }`,
		// An activated default runs on the component that activated it.
		"in a default": `module M {
			type port P message { inout integer }
			type component C { port P p; var integer seen := 0 }
			altstep count() runs on C { [] p.receive(integer:?) { seen := seen + 1; repeat } }
			function f() runs on C {
				var default d := activate(count());
				connect(self:p, self:p);
				p.send(1); p.send(2);
				timer w := 0.2; w.start;
				alt { [] w.timeout {} }
				if (seen == 2) { setverdict(pass) } else { setverdict(fail, "seen=", seen) }
			}
			testcase tc() runs on C system C {
				var C c := C.create;
				c.start(f());
				c.done;
				if (seen != 0) { setverdict(fail, "MTC seen=", seen) }
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
