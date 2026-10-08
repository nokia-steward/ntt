package semantic

import (
	"testing"

	"github.com/nokia/ntt/ttcn3"
)

func TestTemplateRangeReversedIntegerRejected(t *testing.T) {
	// NegSem_B010205_value_range_002: integer range (2..0).
	tree := parse(t, `module M {
		type record MessageType { integer field1 }
		type component C {}
		testcase tc() runs on C {
			template MessageType mw := { field1 := (2..0) };
		}
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	if !containsCode(diags, "template-range-reversed") {
		t.Fatalf("expected template-range-reversed, got %v", codes(diags))
	}
}

func TestTemplateRangeReversedStringRejected(t *testing.T) {
	// NegSem_B010205_value_range_003: charstring range
	// ("fff".."aaa").
	tree := parse(t, `module M {
		type record MessageType { charstring field2 }
		type component C {}
		testcase tc() runs on C {
			template MessageType mw := { field2 := ("fff".."aaa") };
		}
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	if !containsCode(diags, "template-range-reversed") {
		t.Fatalf("expected template-range-reversed on charstring, got %v", codes(diags))
	}
}

func TestTemplateRangeOnEnumRejected(t *testing.T) {
	// NegSem_B010205_value_range_001: range on enumerated type
	// (e_black..e_white).
	tree := parse(t, `module M {
		type enumerated EnumeratedType { e_black, e_white, e_green }
		type record MessageType { EnumeratedType field2 }
		type component C {}
		testcase tc() runs on C {
			template MessageType mw := { field2 := (e_black..e_white) };
		}
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	if !containsCode(diags, "template-range-on-enum") {
		t.Fatalf("expected template-range-on-enum, got %v", codes(diags))
	}
}

func TestTemplateRangeForwardAccepted(t *testing.T) {
	tree := parse(t, `module M {
		type record MessageType { integer field1, charstring field2 }
		type component C {}
		testcase tc() runs on C {
			template MessageType mw := { field1 := (0..10), field2 := ("aaa".."zzz") };
		}
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	if containsCode(diags, "template-range-reversed") || containsCode(diags, "template-range-on-enum") {
		t.Fatalf("did not expect template-range diagnostics on forward range, got %v", codes(diags))
	}
}
