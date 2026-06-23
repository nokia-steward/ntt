package semantic

import (
	"testing"

	"github.com/nokia/ntt/ttcn3"
)

func TestEnumInitWithIntegerRejected(t *testing.T) {
	tree := parse(t, `module M {
		type enumerated EDays { Monday, Tuesday, Wednesday };
		type component C {}
		testcase tc() runs on C {
			var EDays v_day0 := 0;
		}
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	if !containsCode(diags, "enum-init-with-integer") {
		t.Fatalf("expected enum-init-with-integer, got %v", codes(diags))
	}
}

func TestEnumInitWithEnumeratorAccepted(t *testing.T) {
	tree := parse(t, `module M {
		type enumerated EDays { Monday, Tuesday, Wednesday };
		type component C {}
		testcase tc() runs on C {
			var EDays v_day0 := Monday;
		}
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	if containsCode(diags, "enum-init-with-integer") {
		t.Fatalf("did not expect enum-init-with-integer, got %v", codes(diags))
	}
}

func TestBareEnumComparisonRejected(t *testing.T) {
	tree := parse(t, `module M {
		type enumerated EDays { Monday, Tuesday, Wednesday };
		type component C {}
		testcase tc() runs on C {
			if (Tuesday != Wednesday) {}
		}
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	if !containsCode(diags, "bare-enum-comparison-without-type-context") {
		t.Fatalf("expected bare-enum-comparison-without-type-context, got %v", codes(diags))
	}
}

func TestTypedEnumComparisonAccepted(t *testing.T) {
	tree := parse(t, `module M {
		type enumerated EDays { Monday, Tuesday, Wednesday };
		type component C {}
		testcase tc() runs on C {
			var EDays v_day := Monday;
			if (v_day == Monday) {}
		}
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	if containsCode(diags, "bare-enum-comparison-without-type-context") {
		t.Fatalf("unexpected bare-enum-comparison-without-type-context, got %v", codes(diags))
	}
}

func TestEnumOrdinalCallRejectedOnFixedShape(t *testing.T) {
	tree := parse(t, `module M {
		type enumerated EDays { Monday(-1), Tuesday(1), Friday(5) };
		type component C {}
		testcase tc() runs on C {
			var EDays v := Friday(5);
		}
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	if !containsCode(diags, "enum-ordinal-in-value-ref") {
		t.Fatalf("expected enum-ordinal-in-value-ref, got %v", codes(diags))
	}
}

func TestEnumOrdinalCallAcceptedOnRangedShape(t *testing.T) {
	tree := parse(t, `module M {
		type enumerated EDays { Monday(1), Tuesday, Wednesday, Thursday(10), Friday(11..30) };
		type component C {}
		testcase tc() runs on C {
			var EDays v := Friday(15);
		}
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	if containsCode(diags, "enum-ordinal-in-value-ref") {
		t.Fatalf("did not expect enum-ordinal-in-value-ref, got %v", codes(diags))
	}
}

func TestEnumOrdinalCallAcceptedWhenSharedWithRangedShape(t *testing.T) {
	tree := parse(t, `module M {
		type enumerated E1 { e_monday(2), e_tuesday };
		type enumerated E2 { e_monday(1..8), e_tuesday };
		type component C {}
		testcase tc() runs on C {
			var E2 v := e_monday(2);
		}
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	if containsCode(diags, "enum-ordinal-in-value-ref") {
		t.Fatalf("did not expect enum-ordinal-in-value-ref for shared label, got %v", codes(diags))
	}
}
