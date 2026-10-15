package semantic

import (
	"testing"

	"github.com/nokia/ntt/ttcn3"
)

func TestVarOfOmitRejected(t *testing.T) {
	// NegSyn_B010208_omit_value_001: omit on a plain value
	// variable.
	tree := parse(t, `module M {
		type integer My_Int;
		type component C {}
		testcase tc() runs on C {
			var My_Int v_int := omit;
		}
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	if !containsCode(diags, "omit-on-non-template") {
		t.Fatalf("expected omit-on-non-template, got %v", codes(diags))
	}
}

func TestVarTemplateOfOmitAccepted(t *testing.T) {
	// `var template(omit) integer v := omit;` is allowed; the
	// template restriction tells the parser the var stores a
	// template-typed value, which can be omit as a whole.
	tree := parse(t, `module M {
		type component C {}
		testcase tc() runs on C {
			var template(omit) integer v := omit;
		}
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	if containsCode(diags, "omit-on-non-template") {
		t.Fatalf("did not expect omit-on-non-template on template-typed var, got %v", codes(diags))
	}
}

func TestOmitOnMandatoryFieldRejected(t *testing.T) {
	// NegSem_B010208_omit_value_001: field1 is mandatory.
	tree := parse(t, `module M {
		type record MessageType {
			integer field1,
			charstring field2 optional
		}
		type component C {}
		testcase tc() runs on C {
			template MessageType mw := { field1 := omit, field2 := * };
		}
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	if !containsCode(diags, "omit-on-mandatory-field") {
		t.Fatalf("expected omit-on-mandatory-field, got %v", codes(diags))
	}
}

func TestOmitOnNestedMandatoryFieldRejected(t *testing.T) {
	// NegSem_B010208_omit_value_002: `field6 := {a:=1,b:=2,c:=omit}`
	// where `c` is mandatory in the inner RecordType.
	tree := parse(t, `module M {
		type record RecordType {
			integer a optional,
			integer b optional,
			boolean c
		}
		type record MessageType {
			RecordType field6 optional
		}
		type component C {}
		testcase tc() runs on C {
			template MessageType mw := { field6 := { a := 1, b := 2, c := omit } };
		}
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	if !containsCode(diags, "omit-on-mandatory-field") {
		t.Fatalf("expected omit-on-mandatory-field on nested mandatory field, got %v", codes(diags))
	}
}

func TestAnyOrNoneOnMandatoryFieldRejected(t *testing.T) {
	// NegSem_B010204_any_value_or_none_001: `*` is illegal on
	// a non-optional record field.
	tree := parse(t, `module M {
		type record MessageType {
			integer field1,
			charstring field2 optional
		}
		type component C {}
		testcase tc() runs on C {
			template MessageType mw := { field1 := *, field2 := * };
		}
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	if !containsCode(diags, "any-or-none-on-mandatory-field") {
		t.Fatalf("expected any-or-none-on-mandatory-field, got %v", codes(diags))
	}
}

func TestAnyOrNoneOnUnionAlternativeRejected(t *testing.T) {
	// NegSem_B010204_any_value_or_none_002: `*` on a union
	// alternative is illegal because union alternatives are
	// never declared optional.
	tree := parse(t, `module M {
		type union UnionType {
			integer a,
			boolean c
		}
		type record MessageType { UnionType field7 }
		type component C {}
		testcase tc() runs on C {
			template MessageType mw := { field7 := { a := * } };
		}
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	if !containsCode(diags, "any-or-none-on-mandatory-field") {
		t.Fatalf("expected any-or-none-on-mandatory-field on union alt, got %v", codes(diags))
	}
}

func TestOmitOnOptionalFieldAccepted(t *testing.T) {
	tree := parse(t, `module M {
		type record MessageType {
			integer field1,
			charstring field2 optional
		}
		type component C {}
		testcase tc() runs on C {
			template MessageType mw := { field1 := 1, field2 := omit };
		}
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	if containsCode(diags, "omit-on-mandatory-field") {
		t.Fatalf("did not expect omit-on-mandatory-field on optional field, got %v", codes(diags))
	}
}
