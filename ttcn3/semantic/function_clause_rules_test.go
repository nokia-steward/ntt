package semantic

import (
	"testing"

	"github.com/nokia/ntt/ttcn3"
)

func TestFunctionSystemWithoutRunsOnAccepted(t *testing.T) {
	// 6.3.3 item 4: a function may have a system clause and no runs
	// on clause; a testcase running on CB without a system clause
	// has a CB system, whose ports cover those of CA.
	tree := parse(t, `module M {
		type port P message { inout integer; }
		type component CA { port P p; }
		type component CB { port P p; var integer v_x := 1; }
		type component CX { port P q; }
		function f_checkSystem() system CA {}
		function f_unknownSystem() { f_checkSystem(); }
		testcase TC() runs on CB { f_checkSystem(); }
		testcase TC2() runs on CB system CX { f_unknownSystem(); }
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	if containsCode(diags, "call-system-incompatible") {
		t.Fatalf("did not expect call-system-incompatible, got %v", codes(diags))
	}
}

func TestCallSystemIncompatibleRejected(t *testing.T) {
	// 16.1.0: the system clause of f_systemA names A, whose port p_a
	// is missing from the system type S of the calling testcase.
	tree := parse(t, `module M {
		type port P message { inout integer }
		type component A { port P p_a; }
		type component C {}
		type component S {}
		function f_systemA() system A {}
		testcase TC() runs on C system S { f_systemA(); }
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	if !containsCode(diags, "call-system-incompatible") {
		t.Fatalf("expected call-system-incompatible, got %v", codes(diags))
	}
}

func TestControlCallsFunctionWithSystemStillRejected(t *testing.T) {
	// NegSem_1601_toplevel_007: the control part calls a function
	// with a system clause (16.1 restriction c).
	tree := parse(t, `module M {
		type component GeneralComp {}
		function f_test(integer p_integer := 0) system GeneralComp return integer {
			return p_integer + 1;
		}
		control { f_test(1); }
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	if !containsCode(diags, "control-calls-function-with-mtc-or-system") {
		t.Fatalf("expected control-calls-function-with-mtc-or-system, got %v", codes(diags))
	}
}
