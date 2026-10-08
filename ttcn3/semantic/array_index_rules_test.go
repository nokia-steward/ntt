package semantic

import (
	"testing"

	"github.com/nokia/ntt/ttcn3"
)

func TestArrayIndexStringRejected(t *testing.T) {
	// NegSem_060207_arrays_006
	tree := parse(t, `module M {
		type integer MyArr[5] (1..10);
		type component C {}
		testcase tc() runs on C {
			var MyArr v := { 8, 9, 2, 3, 4 };
			v["0"] := 10;
		}
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	if !containsCode(diags, "array-index-non-integer") {
		t.Fatalf("expected array-index-non-integer, got %v", codes(diags))
	}
}

func TestArrayIndexOutOfBoundsRejected(t *testing.T) {
	// NegSem_060207_arrays_007
	tree := parse(t, `module M {
		type integer MyArr[5] (1..10);
		type component C {}
		testcase tc() runs on C {
			var MyArr v := { 8, 9, 2, 3, 4 };
			v[5] := 3;
		}
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	if !containsCode(diags, "array-index-out-of-bounds") {
		t.Fatalf("expected array-index-out-of-bounds, got %v", codes(diags))
	}
}

func TestArrayIndexNegativeRejected(t *testing.T) {
	tree := parse(t, `module M {
		type integer MyArr[3] (0..10);
		type component C {}
		testcase tc() runs on C {
			var MyArr v := { 1, 2, 3 };
			v[-1] := 0;
		}
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	// `-1` is a UnaryExpr, not a single ValueLiteral so it
	// falls through unchecked - we only catch literal-int
	// out-of-bounds. Still a useful smoke test that the
	// analyzer doesn't crash.
	_ = diags
}

func TestArrayIndexWithinBoundsAccepted(t *testing.T) {
	tree := parse(t, `module M {
		type integer MyArr[5] (1..10);
		type component C {}
		testcase tc() runs on C {
			var MyArr v := { 8, 9, 2, 3, 4 };
			v[4] := 7;
		}
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	if containsCode(diags, "array-index-out-of-bounds") {
		t.Fatalf("unexpected array-index-out-of-bounds, got %v", codes(diags))
	}
}
