package semantic

import (
	"testing"

	"github.com/nokia/ntt/ttcn3"
)

func TestArraySizeMismatchOnAssign(t *testing.T) {
	tree := parse(t, `module M {
		type integer A[1];
		type component C {}
		testcase tc() runs on C {
			var integer v_int[2] := { 5, 4 };
			var A v_a;
			v_a := v_int;
		}
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	if !containsCode(diags, "array-size-mismatch") {
		t.Fatalf("expected array-size-mismatch, got %v", codes(diags))
	}
}

func TestArraySizeMismatchOnLiteralInit(t *testing.T) {
	tree := parse(t, `module M {
		type integer A[3];
		type component C {}
		testcase tc() runs on C {
			var A v := { 1, 2 };
		}
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	if !containsCode(diags, "array-size-mismatch") {
		t.Fatalf("expected array-size-mismatch, got %v", codes(diags))
	}
}

func TestArrayIndexedLiteralAccepted(t *testing.T) {
	tree := parse(t, `module M {
		type integer A[3];
		type component C {}
		testcase tc() runs on C {
			var A v := { [1] := 1 };
		}
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	if containsCode(diags, "array-size-mismatch") {
		t.Fatalf("unexpected array-size-mismatch on indexed init, got %v", codes(diags))
	}
}

func TestMixedLiteralOverlapIndexed(t *testing.T) {
	tree := parse(t, `module M {
		type integer A[3];
		type component C {}
		testcase tc() runs on C {
			var A v := { 1, [0] := 3 };
		}
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	if !containsCode(diags, "mixed-literal-notation") {
		t.Fatalf("expected mixed-literal-notation, got %v", codes(diags))
	}
}

func TestMixedLiteralNonOverlappingIndexed(t *testing.T) {
	tree := parse(t, `module M {
		type integer A[3];
		type component C {}
		testcase tc() runs on C {
			var A v := { 1, 2, [2] := 3 };
		}
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	if containsCode(diags, "mixed-literal-notation") {
		t.Fatalf("unexpected mixed-literal-notation on non-overlapping indexed, got %v", codes(diags))
	}
}

func TestMixedLiteralOverlapNamedField(t *testing.T) {
	tree := parse(t, `module M {
		type record R { integer f1, integer f2, integer f3 }
		type component C {}
		testcase tc() runs on C {
			var R v := { 1, 2, f1 := 3 };
		}
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	if !containsCode(diags, "mixed-literal-notation") {
		t.Fatalf("expected mixed-literal-notation (named overlap), got %v", codes(diags))
	}
}

func TestMixedLiteralNamedFieldAccepted(t *testing.T) {
	tree := parse(t, `module M {
		type record R { integer f1, charstring f2, float f3 }
		type component C {}
		testcase tc() runs on C {
			var R v := { 5, f3 := 3.14, f2 := "ABCD" };
		}
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	if containsCode(diags, "mixed-literal-notation") {
		t.Fatalf("unexpected mixed-literal-notation on non-overlapping named, got %v", codes(diags))
	}
}

func TestLengthConstraintBareIdentRHS(t *testing.T) {
	tree := parse(t, `module M {
		type charstring CC length (1);
		type component C {}
		testcase tc() runs on C {
			var charstring v_c := "jk";
			var CC v_cc;
			v_cc := v_c;
		}
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	if !containsCode(diags, "length-constraint-violation") {
		t.Fatalf("expected length-constraint-violation, got %v", codes(diags))
	}
}
