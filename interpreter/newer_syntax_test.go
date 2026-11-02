package interpreter_test

import (
	"testing"

	"github.com/nokia/ntt/interpreter"
	"github.com/nokia/ntt/runtime"
	"github.com/nokia/ntt/ttcn3"
)

// TestNewerSyntaxRuns: a variadic parameter is the record of the actuals
// from its position on, its default for none or `-`, the record itself in
// assignment notation, and expands `r...`; a variable an if, while or
// select header declares is the statement's own; an embedded field is a
// field named by its type, its value encoded as JSON too.
func TestNewerSyntaxRuns(t *testing.T) {
	runLogged(t, `module M {
		type record of integer RoI;
		type record B { integer b }
		type record A { B, integer x } with { encode "JSON" }
		type component C {}
		function f(in integer i... := {1, 2, 3}) return integer { return lengthof(i); }
		function g(in integer a, in integer r...) return integer {
			var integer s := a;
			for (var integer k := 0; k < lengthof(r); k := k + 1) { s := s + r[k]; }
			return s;
		}
		testcase tc() runs on C {
			var RoI l := {4, 5};
			var integer n := 0;
			if (var integer x := 5; x == 5) { n := n + 1; }
			while (var integer i := 0; i < 0) { }
			select (var integer k := 2; k) { case (2) { n := n + 1; } case else { } }
			var A v := { B := { b := 1 }, x := 2 };
			if (f() == 3 and f(-) == 3 and f(7) == 1 and g(1) == 1 and g(1, 2, 3) == 6 and g(1, l...) == 10 and
				g(a := 1, r := {2, 2}) == 5 and n == 2 and v.B.b == 1 and
				encvalue_unichar(v) == "{""B"":{""b"":1},""x"":2}") {
				setverdict(pass);
			} else {
				setverdict(fail);
			}
		}
	}`, clocks[0].opts)
}

// TestExecuteInAConditionsHeader: a testcase a control part executes in
// an if header's declaration is found with its actual parameters.
func TestExecuteInAConditionsHeader(t *testing.T) {
	src := `module M {
		type component C {}
		testcase tc(integer p) runs on C { if (p == 5) { setverdict(pass); } else { setverdict(fail); } }
		control { if (var verdicttype v := execute(tc(5)); v == pass) { } }
	}`
	v, reason, err := interpreter.RunTestcaseWith([]*ttcn3.Tree{parse(t, src)}, "M.tc", clocks[0].opts)
	if err != nil || v != runtime.PassVerdict {
		t.Fatalf("verdict = %s (%s), %v; want pass", v, reason, err)
	}
}
