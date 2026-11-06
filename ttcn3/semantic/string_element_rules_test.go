package semantic

import (
	"testing"

	"github.com/nokia/ntt/ttcn3"
)

func TestStringElementNonSingleCharRejected(t *testing.T) {
	// NegSyn_06010101_AccessStringElements_001 shape: assigning
	// a 2-char string to a single string slot.
	tree := parse(t, `module M {
		type component C {}
		testcase tc() runs on C {
			var universal charstring v_b := "AbCdE";
			v_b[1] := "FF";
		}
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	if !containsCode(diags, "string-element-not-single-char") {
		t.Fatalf("expected string-element-not-single-char, got %v", codes(diags))
	}
}

func TestStringElementSingleCharAccepted(t *testing.T) {
	tree := parse(t, `module M {
		type component C {}
		testcase tc() runs on C {
			var charstring v_b := "AbCdE";
			v_b[1] := "Z";
		}
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	if containsCode(diags, "string-element-not-single-char") {
		t.Fatalf("unexpected string-element-not-single-char, got %v", codes(diags))
	}
}

func TestStringElementUniversalSingleAccepted(t *testing.T) {
	tree := parse(t, `module M {
		type component C {}
		testcase tc() runs on C {
			var universal charstring v_b := "";
			v_b[0] := "A";
		}
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	if containsCode(diags, "string-element-not-single-char") {
		t.Fatalf("unexpected string-element-not-single-char, got %v", codes(diags))
	}
}

func TestStringElementEmptyValueRejected(t *testing.T) {
	tree := parse(t, `module M {
		type component C {}
		testcase tc() runs on C {
			var charstring v_b := "ab";
			v_b[0] := "";
		}
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	if !containsCode(diags, "string-element-not-single-char") {
		t.Fatalf("expected string-element-not-single-char on empty assign, got %v", codes(diags))
	}
}
