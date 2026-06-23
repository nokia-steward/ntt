package semantic

import (
	"testing"

	"github.com/nokia/ntt/ttcn3"
)

func TestArrayCustomLowerBoundRejected(t *testing.T) {
	// NegSem_060207_arrays_022
	tree := parse(t, `module M {
		type component C {}
		testcase tc() runs on C {
			var integer v_arr[2..5] := { 2, 3, 4, 5 };
			var boolean v_bool := v_arr[0] == 0;
		}
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	if !containsCode(diags, "array-index-out-of-bounds") {
		t.Fatalf("expected array-index-out-of-bounds, got %v", codes(diags))
	}
}

func TestArrayCustomUpperBoundRejected(t *testing.T) {
	// NegSem_060207_arrays_024
	tree := parse(t, `module M {
		type component C {}
		testcase tc() runs on C {
			var integer v_arr[2..5] := { 2, 3, 4, 5 };
			var integer v_int := v_arr[6];
		}
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	if !containsCode(diags, "array-index-out-of-bounds") {
		t.Fatalf("expected array-index-out-of-bounds, got %v", codes(diags))
	}
}

func TestArrayCustomRangeWithinBoundsAccepted(t *testing.T) {
	tree := parse(t, `module M {
		type component C {}
		testcase tc() runs on C {
			var integer v_arr[2..5] := { 2, 3, 4, 5 };
			var integer v_int := v_arr[3];
		}
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	if containsCode(diags, "array-index-out-of-bounds") {
		t.Fatalf("unexpected diag, got %v", codes(diags))
	}
}

func TestRecordOfLengthBoundRejected(t *testing.T) {
	// NegSem_060203_records_and_sets_of_single_types_011
	tree := parse(t, `module M {
		type component C {}
		type record length (0..3) of integer RoI;
		testcase tc() runs on C {
			var RoI v_rec := { 0, 1, 2 };
			v_rec[3] := 3;
		}
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	if !containsCode(diags, "array-index-out-of-bounds") {
		t.Fatalf("expected array-index-out-of-bounds, got %v", codes(diags))
	}
}

func TestSetOfLengthBoundRejected(t *testing.T) {
	// NegSem_060203_records_and_sets_of_single_types_012
	tree := parse(t, `module M {
		type component C {}
		type set length (0..3) of integer SoI;
		testcase tc() runs on C {
			var SoI v_set := { 0, 1, 2 };
			v_set[3] := 3;
		}
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	if !containsCode(diags, "array-index-out-of-bounds") {
		t.Fatalf("expected array-index-out-of-bounds, got %v", codes(diags))
	}
}

func TestRecordOfWithinBoundsAccepted(t *testing.T) {
	tree := parse(t, `module M {
		type component C {}
		type record length (0..3) of integer RoI;
		testcase tc() runs on C {
			var RoI v_rec := { 0, 1, 2 };
			v_rec[2] := 9;
		}
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	if containsCode(diags, "array-index-out-of-bounds") {
		t.Fatalf("unexpected diag, got %v", codes(diags))
	}
}
