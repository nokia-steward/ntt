package semantic

import (
	"testing"

	"github.com/nokia/ntt/ttcn3"
)

func TestStringRangeInfinityRejected(t *testing.T) {
	// NegSem_06010203_Ranges_016 / _017: infinity / -infinity
	// in a charstring range.
	tree := parse(t, `module M {
		type charstring T1 ("a"..infinity);
		type charstring T2 (-infinity.."d");
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	if !containsCode(diags, "range-bound-infinity-on-string") {
		t.Fatalf("expected range-bound-infinity-on-string, got %v", codes(diags))
	}
}

func TestStringRangeLiteralAccepted(t *testing.T) {
	tree := parse(t, `module M {
		type charstring T1 ("a".."z");
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	if containsCode(diags, "range-bound-infinity-on-string") {
		t.Fatalf("did not expect range-bound-infinity-on-string on literal bounds, got %v", codes(diags))
	}
}

// TestRangeBoundBoolRejected pins ETSI 6.1.2.3: boolean literals
// (true / false) are not legal numeric range bounds even when
// the type prefix is integer. The pre-fix behaviour silently
// dropped the constraint (since neither bound was a numeric
// literal) and the type was registered without any restriction,
// making the surrounding fixture pass instead of reject.
func TestRangeBoundBoolRejected(t *testing.T) {
	tree := parse(t, `module M {
		type integer MyBoolRange (false .. true);
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	if !containsCode(diags, "range-bound-bool") {
		t.Fatalf("expected range-bound-bool, got %v", codes(diags))
	}
}

// TestRangeBoundNaNRejected pins the float NaN exclusion from
// 6.1.2.3: `not_a_number` is a valid float literal but is
// explicitly forbidden as a range bound.
func TestRangeBoundNaNRejected(t *testing.T) {
	tree := parse(t, `module M {
		type float MyFloatRange (-infinity .. not_a_number);
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	if !containsCode(diags, "range-bound-nan") {
		t.Fatalf("expected range-bound-nan, got %v", codes(diags))
	}
}

// TestRangeBoundNumericAccepted is the negative control: a
// well-formed range should not produce any range-bound
// diagnostic.
func TestRangeBoundNumericAccepted(t *testing.T) {
	tree := parse(t, `module M {
		type integer Good (1 .. 10);
		type float GoodF (-infinity .. 3.14);
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	for _, d := range diags {
		switch d.Code {
		case "range-bound-bool", "range-bound-nan",
			"range-bound-verdict", "range-bound-string":
			t.Fatalf("false positive: %s", d.Message)
		}
	}
	_ = ttcn3.DB{}
}
