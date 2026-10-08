package semantic

import (
	"testing"

	"github.com/nokia/ntt/ttcn3"
)

func TestSizeofOnRecordOfRejected(t *testing.T) {
	// NegSem_160102_predefined_functions_062
	tree := parse(t, `module M {
		type record of integer MyROItype;
		type component C {}
		testcase tc() runs on C {
			template MyROItype MyROI := {1, 2, 3};
			var integer v_i;
			v_i := sizeof(MyROI);
		}
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	if !containsCode(diags, "sizeof-on-variable-shape") {
		t.Fatalf("expected sizeof-on-variable-shape, got %v", codes(diags))
	}
}

func TestSizeofOnSetOfRejected(t *testing.T) {
	// NegSem_160102_predefined_functions_063
	tree := parse(t, `module M {
		type set of integer MySOItype;
		type component C {}
		testcase tc() runs on C {
			template MySOItype MySOI := {1, 2, 3};
			var integer v_i;
			v_i := sizeof(MySOI);
		}
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	if !containsCode(diags, "sizeof-on-variable-shape") {
		t.Fatalf("expected sizeof-on-variable-shape, got %v", codes(diags))
	}
}

func TestSizeofOnUnionRejected(t *testing.T) {
	// NegSem_160102_predefined_functions_060
	tree := parse(t, `module M {
		type union MyUnion { integer a, charstring b }
		type component C {}
		testcase tc() runs on C {
			template MyUnion MU := { a := 1 };
			var integer v_i;
			v_i := sizeof(MU);
		}
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	if !containsCode(diags, "sizeof-on-variable-shape") {
		t.Fatalf("expected sizeof-on-variable-shape, got %v", codes(diags))
	}
}

func TestSizeofOnFixedRecordAccepted(t *testing.T) {
	tree := parse(t, `module M {
		type record R { integer a, integer b }
		type component C {}
		testcase tc() runs on C {
			template R t1 := { a := 1, b := 2 };
			var integer v_i;
			v_i := sizeof(t1);
		}
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	if containsCode(diags, "sizeof-on-variable-shape") {
		t.Fatalf("unexpected diag on fixed record, got %v", codes(diags))
	}
}

func TestRegexpNegativeGroupRejected(t *testing.T) {
	// NegSem_160102_predefined_functions_018
	tree := parse(t, `module M {
		type component C {}
		testcase tc() runs on C {
			var charstring v_example := "x";
			var charstring v_i := regexp(v_example, charstring:"?+(x)?+", -1);
		}
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	if !containsCode(diags, "regexp-negative-group-index") {
		t.Fatalf("expected regexp-negative-group-index, got %v", codes(diags))
	}
}

func TestRegexpZeroGroupAccepted(t *testing.T) {
	tree := parse(t, `module M {
		type component C {}
		testcase tc() runs on C {
			var charstring v_example := "x";
			var charstring v_i := regexp(v_example, charstring:"?+(x)?+", 0);
		}
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	if containsCode(diags, "regexp-negative-group-index") {
		t.Fatalf("unexpected diag on zero group index, got %v", codes(diags))
	}
}
