package semantic

import (
	"testing"

	"github.com/nokia/ntt/ttcn3"
)

// The argument of activate is an altstep invocation or an ObjectReference,
// a function instance among them (ETSI ES 201 873-1 20.5.2, A.1): a
// function returning a default re-activates that default, any other
// function is no argument of activate.
func TestActivateTakesAFunctionReturningADefault(t *testing.T) {
	tree := parse(t, `module M {
		type component C { var default vc_d }
		altstep a() runs on C { [] any timer.timeout {} }
		function f_get() runs on C return default { return vc_d }
		testcase tc() runs on C {
			vc_d := activate(a());
			deactivate(vc_d, true);
			activate(f_get());
		}
	}`)
	if diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree); containsCode(diags, "activate-non-altstep") {
		t.Fatalf("activate(f_get()) rejected: %v", codes(diags))
	}
	tree = parse(t, `module M {
		type component C {}
		function f() runs on C return integer { return 1 }
		testcase tc() runs on C { activate(f()) }
	}`)
	if diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree); !containsCode(diags, "activate-non-altstep") {
		t.Fatalf("expected activate-non-altstep, got %v", codes(diags))
	}
}
