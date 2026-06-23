package semantic

import (
	"testing"

	"github.com/nokia/ntt/ttcn3"
)

func TestSubsetOnRecordOfFieldRejected(t *testing.T) {
	// ETSI B.1.2.6: subset(...) is only legal on a `set of`
	// target. A record-of field with a subset template should
	// be flagged. NegSem_B010207_subset_001.
	tree := parse(t, `module M {
		type record MessageType {
			record of integer field1
		}
		type component C {}
		testcase tc() runs on C {
			template MessageType mw := { field1 := subset(1, 2) };
		}
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	if !containsCode(diags, "subset-on-non-setof") {
		t.Fatalf("expected subset-on-non-setof, got %v", codes(diags))
	}
}

func TestSubsetOnSetFieldRejected(t *testing.T) {
	// NegSem_B010207_subset_002: subset on a `set` (not
	// `set of`) field is also illegal.
	tree := parse(t, `module M {
		type set SetType {
			integer a optional,
			integer b optional
		}
		type record MessageType {
			SetType field1
		}
		type component C {}
		testcase tc() runs on C {
			template MessageType mw := { field1 := subset(1, 2) };
		}
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	if !containsCode(diags, "subset-on-non-setof") {
		t.Fatalf("expected subset-on-non-setof on set field, got %v", codes(diags))
	}
}

func TestSubsetOnSetOfFieldAccepted(t *testing.T) {
	tree := parse(t, `module M {
		type set of integer SoI;
		type record MessageType {
			SoI field1
		}
		type component C {}
		testcase tc() runs on C {
			template MessageType mw := { field1 := subset(1, 2) };
		}
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	if containsCode(diags, "subset-on-non-setof") {
		t.Fatalf("did not expect subset-on-non-setof on set-of field, got %v", codes(diags))
	}
}

func TestSubsetDirectOnRecordOfRejected(t *testing.T) {
	// Direct template assignment: `template T := subset(...)`
	// where T is a record-of subtype.
	tree := parse(t, `module M {
		type record of integer RoI;
		type component C {}
		testcase tc() runs on C {
			template RoI mw := subset(1, 2);
		}
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	if !containsCode(diags, "subset-on-non-setof") {
		t.Fatalf("expected subset-on-non-setof on record-of template, got %v", codes(diags))
	}
}

func TestSubsetDirectOnSubtypedSetOfAccepted(t *testing.T) {
	// `type SoI MessageType;` aliases a set-of; subset on the
	// alias should still resolve to set-of via the typeCategory
	// fixpoint.
	tree := parse(t, `module M {
		type set of integer SoI;
		type SoI MessageType;
		type component C {}
		testcase tc() runs on C {
			template MessageType mw := subset(1, 2);
		}
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	if containsCode(diags, "subset-on-non-setof") {
		t.Fatalf("did not expect subset-on-non-setof on aliased set-of, got %v", codes(diags))
	}
}

func TestSupersetOnRecordOfRejected(t *testing.T) {
	tree := parse(t, `module M {
		type record of integer RoI;
		type component C {}
		testcase tc() runs on C {
			template RoI mw := superset(1, 2);
		}
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	if !containsCode(diags, "superset-on-non-setof") {
		t.Fatalf("expected superset-on-non-setof, got %v", codes(diags))
	}
}
