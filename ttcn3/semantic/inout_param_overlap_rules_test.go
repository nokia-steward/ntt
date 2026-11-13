package semantic

import (
	"testing"

	"github.com/nokia/ntt/ttcn3"
)

func TestInoutOverlapParentAndChildRejected(t *testing.T) {
	tree := parse(t, `module M {
		type record of integer RoI;
		type component C {}
		function f(inout RoI p_roi, inout integer p_elem) {}
		testcase tc() runs on C {
			var RoI v_roi := { 0, 1, 2 };
			f(v_roi, v_roi[1]);
		}
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	if !containsCode(diags, "inout-param-overlap") {
		t.Fatalf("expected inout-param-overlap, got %v", codes(diags))
	}
}

func TestInoutOverlapDistinctFieldsRejected(t *testing.T) {
	tree := parse(t, `module M {
		type union U { integer option1, charstring option2 }
		type component C {}
		function f(inout integer p_val, inout charstring p_char) {}
		testcase tc() runs on C {
			var U v_val := { option1 := 1 };
			f(v_val.option1, v_val.option2);
		}
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	if !containsCode(diags, "inout-param-overlap") {
		t.Fatalf("expected inout-param-overlap, got %v", codes(diags))
	}
}

func TestInoutOverlapSiblingIndicesAccepted(t *testing.T) {
	tree := parse(t, `module M {
		type record of integer RoI;
		type component C {}
		function f_swap(inout integer p_a, inout integer p_b) {}
		testcase tc() runs on C {
			var RoI v_roi := { 0, 1, 2, 3, 4, 5 };
			f_swap(v_roi[0], v_roi[5]);
		}
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	if containsCode(diags, "inout-param-overlap") {
		t.Fatalf("did not expect inout-param-overlap for sibling indices, got %v", codes(diags))
	}
}

func TestInoutOverlapDistinctRootsAccepted(t *testing.T) {
	tree := parse(t, `module M {
		type record of integer RoI;
		type component C {}
		function f(inout RoI p_a, inout RoI p_b) {}
		testcase tc() runs on C {
			var RoI v1 := { 0 };
			var RoI v2 := { 0 };
			f(v1, v2);
		}
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	if containsCode(diags, "inout-param-overlap") {
		t.Fatalf("did not expect inout-param-overlap for distinct roots, got %v", codes(diags))
	}
}
