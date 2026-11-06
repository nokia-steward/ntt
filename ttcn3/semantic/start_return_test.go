package semantic

import (
	"testing"

	"github.com/nokia/ntt/ttcn3"
)

func TestStartFuncReturnsPortRejected(t *testing.T) {
	// NegSem_210310_call_test_component_operation_012
	tree := parse(t, `module M {
		type port P message { inout integer }
		type component C {}
		function f() runs on C return P { return null; }
		testcase tc() runs on C system C {
			var C v_ptc := C.create;
			v_ptc.call(f());
		}
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	if !containsCode(diags, "start-forbidden-return-kind") {
		t.Fatalf("expected start-forbidden-return-kind, got %v", codes(diags))
	}
}

func TestStartFuncReturnsTimerRejected(t *testing.T) {
	// NegSem_210310_call_test_component_operation_014
	tree := parse(t, `module M {
		type component C {}
		function f() runs on C return timer { return null; }
		testcase tc() runs on C system C {
			var C v_ptc := C.create;
			v_ptc.call(f());
		}
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	if !containsCode(diags, "start-forbidden-return-kind") {
		t.Fatalf("expected start-forbidden-return-kind, got %v", codes(diags))
	}
}

func TestStartFuncReturnsIntegerAccepted(t *testing.T) {
	tree := parse(t, `module M {
		type component C {}
		function f() runs on C return integer { return 42; }
		testcase tc() runs on C system C {
			var C v_ptc := C.create;
			v_ptc.call(f());
		}
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	if containsCode(diags, "start-forbidden-return-kind") {
		t.Fatalf("unexpected diag, got %v", codes(diags))
	}
}
