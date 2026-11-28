package semantic

import (
	"testing"

	"github.com/nokia/ntt/ttcn3"
)

func TestValueConstraintResolveBareIdentRejected(t *testing.T) {
	// NegSem_060301_non_structured_types_001
	tree := parse(t, `module M {
		type integer ConstrainedInt(1..10);
		type component C {}
		testcase tc() runs on C {
			var integer v_int := 15;
			var ConstrainedInt v_c;
			v_c := v_int;
		}
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	if !containsCode(diags, "value-constraint-violation") {
		t.Fatalf("expected value-constraint-violation, got %v", codes(diags))
	}
}

func TestValueConstraintInRangeBareIdentAccepted(t *testing.T) {
	tree := parse(t, `module M {
		type integer ConstrainedInt(1..10);
		type component C {}
		testcase tc() runs on C {
			var integer v_int := 5;
			var ConstrainedInt v_c;
			v_c := v_int;
		}
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	if containsCode(diags, "value-constraint-violation") {
		t.Fatalf("unexpected diag, got %v", codes(diags))
	}
}

func TestValueConstraintReassignedIdentNotResolved(t *testing.T) {
	// Once v_int is reassigned its literal init is no longer
	// reliable; we silently skip rather than risk a false positive.
	tree := parse(t, `module M {
		type integer ConstrainedInt(1..10);
		type component C {}
		function f() return integer { return 5; }
		testcase tc() runs on C {
			var integer v_int := 15;
			v_int := f();
			var ConstrainedInt v_c;
			v_c := v_int;
		}
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	if containsCode(diags, "value-constraint-violation") {
		t.Fatalf("unexpected diag, got %v", codes(diags))
	}
}
