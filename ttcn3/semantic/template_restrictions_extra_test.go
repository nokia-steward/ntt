package semantic

import (
	"testing"

	"github.com/nokia/ntt/ttcn3"
)

func TestDecmatchInOmitTemplateRejected(t *testing.T) {
	// NegSem_1508_TemplateRestrictions_050 shape: decmatch
	// content match inside a template(omit) initializer.
	tree := parse(t, `module M {
		type record MessageType { hexstring payload }
		type record Mymessage { integer field1, bitstring field2 optional }
		type component C {}
		testcase tc() runs on C {
			template (omit) MessageType mw := {
				payload := decmatch Mymessage: { field1 := 10, field2 := omit }
			}
		}
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	if !containsCode(diags, "template-restriction-violation") {
		t.Fatalf("expected template-restriction-violation, got %v", codes(diags))
	}
}

func TestDecmatchInValueTemplateRejected(t *testing.T) {
	// NegSem_1508_TemplateRestrictions_051 shape: decmatch in
	// template(value).
	tree := parse(t, `module M {
		type record MessageType { hexstring payload }
		type record Mymessage { integer field1, bitstring field2 optional }
		type component C {}
		testcase tc() runs on C {
			template (value) MessageType mw := {
				payload := decmatch Mymessage: { field1 := 10, field2 := '1001'B }
			}
		}
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	if !containsCode(diags, "template-restriction-violation") {
		t.Fatalf("expected template-restriction-violation, got %v", codes(diags))
	}
}

func TestDecmatchInPresentTemplateAccepted(t *testing.T) {
	tree := parse(t, `module M {
		type record MessageType { hexstring payload }
		type record Mymessage { integer field1, bitstring field2 optional }
		type component C {}
		testcase tc() runs on C {
			template (present) MessageType mw := {
				payload := decmatch Mymessage: { field1 := 10, field2 := '1001'B }
			}
		}
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	if containsCode(diags, "template-restriction-violation") {
		t.Fatalf("unexpected diag on template(present), got %v", codes(diags))
	}
}

func TestModifiedTemplateListBaseRejected(t *testing.T) {
	tree := parse(t, `module M {
		signature S(in integer p_a, out integer p_b);
		template S base := (
			{ p_a := -, p_b := 4 },
			{ p_a := -, p_b := 5 }
		);
		template S changed modifies base := {
			p_b := 6
		}
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	if !containsCode(diags, "modified-template-list-base") {
		t.Fatalf("expected modified-template-list-base, got %v", codes(diags))
	}
}

func TestModifiedTemplateCompositeBaseAccepted(t *testing.T) {
	tree := parse(t, `module M {
		signature S(in integer p_a, out integer p_b);
		template S base := {
			p_a := -,
			p_b := 4
		}
		template S changed modifies base := {
			p_b := 6
		}
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	if containsCode(diags, "modified-template-list-base") {
		t.Fatalf("unexpected modified-template-list-base, got %v", codes(diags))
	}
}
