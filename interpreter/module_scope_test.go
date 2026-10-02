package interpreter_test

import (
	"strings"
	"testing"

	"github.com/nokia/ntt/interpreter"
	"github.com/nokia/ntt/runtime"
	"github.com/nokia/ntt/runtime/tl"
	"github.com/nokia/ntt/ttcn3"
)

// TestModuleOwnNamesFirst: a module's functions resolve its own names
// before another module's like-named ones, whichever module the run
// loaded last and whether or not it imports the other.
func TestModuleOwnNamesFirst(t *testing.T) {
	a := parse(t, `module A {
		type component C {}
		const charstring c_closed := "<closed>";
		function isClosed(charstring line) return boolean { return line == c_closed }
		function own() runs on C return charstring { return c_closed }
	}`)
	b := parse(t, `module B {
		type record Link { integer up }
		const Link c_closed := { up := 0 };
		function linkDown() return boolean { return c_closed.up == 0 }
	}`)
	m := parse(t, `module M {
		import from A all;
		import from B { function linkDown };
		testcase tc() runs on C system C {
			if (isClosed("<closed>") and own() == "<closed>" and linkDown()) { setverdict(pass) } else { setverdict(fail) }
		}
	}`)
	for _, order := range [][]*ttcn3.Tree{{m, a, b}, {m, b, a}} {
		for _, k := range clocks {
			v, reason, err := interpreter.RunTestcaseWith(order, "M.tc", k.opts)
			if err != nil || v != runtime.PassVerdict {
				t.Errorf("%s clock: %s (%s) %v", k.name, v, reason, err)
			}
		}
	}
}

// TestModuleOfAFunctionsVariables: the test log names a function's
// variable by the function's module, not the testcase's.
func TestModuleOfAFunctionsVariables(t *testing.T) {
	a := parse(t, `module A {
		function f() return integer { var integer x := 0; x := 5; return x }
	}`)
	m := parse(t, `module M {
		import from A all;
		type component C {}
		testcase tc() runs on C { if (f() == 5) { setverdict(pass) } }
	}`)
	rec := &tl.Recorder{}
	v, reason, err := interpreter.RunTestcaseWith([]*ttcn3.Tree{m, a}, "M.tc", interpreter.TestcaseOptions{TestLogger: rec})
	if err != nil || v != runtime.PassVerdict {
		t.Fatalf("%s (%s) %v", v, reason, err)
	}
	found := false
	for _, e := range rec.Events {
		if e.Op != "tliVar" {
			continue
		}
		for _, a := range e.Args {
			if a.Name == "name" && len(a.Val.Attrs) >= 2 && a.Val.Attrs[1].Value == "x" {
				found = true
				if a.Val.Attrs[0].Value != "A" {
					t.Errorf("x logged in module %q, want A", a.Val.Attrs[0].Value)
				}
			}
		}
	}
	if !found {
		var ops []string
		for _, e := range rec.Events {
			ops = append(ops, e.Op)
		}
		t.Fatalf("no tliVar for x: %s", strings.Join(ops, " "))
	}
}

// TestLikeNamedComponentTypes: a component type extending Base in one
// module extends that module's Base, though another module has a Base
// of its own; and each component has its own inherited variables.
func TestLikeNamedComponentTypes(t *testing.T) {
	x := parse(t, `module X {
		type component Base { var integer nb := 5 }
		type component C extends Base { var integer n := 1 }
		function xget() runs on C return integer { return n + nb }
	}`)
	m := parse(t, `module M {
		import from X all;
		type component Base { var integer other := 1 }
		function w() runs on C { nb := nb + 1; if (xget() != 7) { setverdict(fail, "xget=", xget()) } }
		testcase tc() runs on C system C {
			var C c := C.create; c.start(w()); c.done;
			var C c2 := C.create; c2.start(w()); c2.done;
			if (nb != 5) { setverdict(fail, "MTC nb=", nb) }
			w();
			setverdict(pass);
		}
	}`)
	for _, order := range [][]*ttcn3.Tree{{m, x}, {x, m}} {
		for _, k := range clocks {
			v, reason, err := interpreter.RunTestcaseWith(order, "M.tc", k.opts)
			if err != nil || v != runtime.PassVerdict {
				t.Errorf("%s clock: %s (%s) %v", k.name, v, reason, err)
			}
		}
	}
}

// TestComponentMembersOfTheDeclaringModule: a component type's member
// initialisers name what the module declaring the type sees, for the MTC
// and a PTC, and for an inherited member.
func TestComponentMembersOfTheDeclaringModule(t *testing.T) {
	lib := parse(t, `module Lib {
		const integer c_n := 1;
		type component Base { var integer v_base := c_n }
		type component Comp extends Base { var integer v_own := c_n + 1 }
	}`)
	m := parse(t, `module M {
		import from Lib all;
		const integer c_n := 100;
		function check() runs on Comp {
			if (v_base == 1 and v_own == 2) { setverdict(pass) } else { setverdict(fail, v_base, " ", v_own) }
		}
		testcase tc() runs on Comp system Comp {
			check();
			var Comp p := Comp.create;
			p.start(check());
			p.done;
		}
	}`)
	for _, order := range [][]*ttcn3.Tree{{m, lib}, {lib, m}} {
		for _, k := range clocks {
			v, reason, err := interpreter.RunTestcaseWith(order, "M.tc", k.opts)
			if err != nil || v != runtime.PassVerdict {
				t.Errorf("%s clock: %s (%s) %v", k.name, v, reason, err)
			}
		}
	}
}

// TestModuleQualifiedReference: `A.c_x` and `A.f()` name module A's
// definitions, whatever like-named ones other modules have (ETSI
// 8.2.3.1).
func TestModuleQualifiedReference(t *testing.T) {
	a := parse(t, `module A {
		const integer c_x := 1;
		function f() return integer { return 10 }
	}`)
	b := parse(t, `module B {
		const integer c_x := 2;
		function f() return integer { return 20 }
	}`)
	m := parse(t, `module M {
		import from A all;
		import from B all;
		type component C {}
		testcase tc() runs on C {
			if (A.c_x == 1 and B.c_x == 2 and A.f() == 10 and B.f() == 20) { setverdict(pass) }
			else { setverdict(fail, A.c_x, " ", B.c_x, " ", A.f(), " ", B.f()) }
		}
	}`)
	for _, order := range [][]*ttcn3.Tree{{m, a, b}, {m, b, a}} {
		for _, k := range clocks {
			v, reason, err := interpreter.RunTestcaseWith(order, "M.tc", k.opts)
			if err != nil || v != runtime.PassVerdict {
				t.Errorf("%s clock: %s (%s) %v", k.name, v, reason, err)
			}
		}
	}
}

// TestFunctionWritesTheComponentsRecord: a function running on a component
// that writes a field of an unbound component record writes the
// component's, not a copy of its own.
func TestFunctionWritesTheComponentsRecord(t *testing.T) {
	src := `module M {
		type record R { integer n, integer m optional }
		type component C { var R cr }
		function fill() runs on C { cr.n := 5; cr.m := omit }
		testcase tc() runs on C {
			fill();
			if (isbound(cr) and cr.n == 5) { setverdict(pass) } else { setverdict(fail, cr) }
		}
	}`
	for _, k := range clocks {
		v, reason, err := interpreter.RunTestcaseWith([]*ttcn3.Tree{parse(t, src)}, "M.tc", k.opts)
		if err != nil || v != runtime.PassVerdict {
			t.Errorf("%s clock: %s (%s) %v", k.name, v, reason, err)
		}
	}
}
