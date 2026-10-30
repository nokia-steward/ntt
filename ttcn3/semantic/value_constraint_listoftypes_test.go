package semantic

import (
	"testing"

	"github.com/nokia/ntt/ttcn3"
)

// TestValueConstraint_ListOfTypes pins ETSI 6.1.2.2: a subtype
// declared as `type integer T (T1, T2)` inherits the union of
// T1's and T2's value sets. A literal that falls outside both
// is rejected as a value-constraint violation.
func TestValueConstraint_ListOfTypes_InvalidLiteral(t *testing.T) {
	tree := parse(t, `module M {
		type integer Integer1 (0..9);
		type integer Integer2 (20..30);
		type integer Integer_1_2 (Integer1, Integer2);
		type component C {}
		testcase tc() runs on C {
			var Integer_1_2 v_b;
			v_b := 15;
			setverdict(pass);
		}
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	if !containsCode(diags, "value-constraint-violation") {
		t.Fatalf("expected value-constraint-violation, got %v", codes(diags))
	}
}

func TestValueConstraint_ListOfTypes_ValidLiteral(t *testing.T) {
	tree := parse(t, `module M {
		type integer Integer1 (0..9);
		type integer Integer2 (20..30);
		type integer Integer_1_2 (Integer1, Integer2);
		type component C {}
		testcase tc() runs on C {
			var Integer_1_2 v_b := 5;
			setverdict(pass);
		}
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	if containsCode(diags, "value-constraint-violation") {
		t.Fatalf("did not expect value-constraint-violation, got %v", codes(diags))
	}
}

// TestValueConstraint_ListOfTypes_ForwardDeclaration verifies
// the fixpoint resolver handles a constraint that references a
// type declared *later* in the module (declaration order should
// not matter for the inferred value set).
func TestValueConstraint_ListOfTypes_ForwardDeclaration(t *testing.T) {
	tree := parse(t, `module M {
		type integer Integer_1_2 (Integer1, Integer2);
		type integer Integer1 (0..9);
		type integer Integer2 (20..30);
		type component C {}
		testcase tc() runs on C {
			var Integer_1_2 v_b;
			v_b := 15;
			setverdict(pass);
		}
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	if !containsCode(diags, "value-constraint-violation") {
		t.Fatalf("expected value-constraint-violation on forward-declared T1/T2, got %v", codes(diags))
	}
}
