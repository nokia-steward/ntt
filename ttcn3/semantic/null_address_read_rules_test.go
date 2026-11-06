package semantic

import (
	"testing"

	"github.com/nokia/ntt/ttcn3"
)

func TestNullAddressRecordFieldReadRejected(t *testing.T) {
	tree := parse(t, `module M {
		type integer address;
		type record R {
			address field1,
			integer field2 optional
		}
		type component C {}
		testcase tc() runs on C {
			var R r := { field1 := null, field2 := - };
			var integer v := r.field1;
		}
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	if !containsCode(diags, "null-address-read") {
		t.Fatalf("expected null-address-read, got %v", codes(diags))
	}
}

func TestNullAddressSetElementReadRejected(t *testing.T) {
	tree := parse(t, `module M {
		type integer address;
		type set of address AddressSet;
		type component C {}
		testcase tc() runs on C {
			var AddressSet s := { [0] := null };
			var template integer v := s[0];
		}
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	if !containsCode(diags, "null-address-read") {
		t.Fatalf("expected null-address-read, got %v", codes(diags))
	}
}

func TestNonNullAddressFieldReadAccepted(t *testing.T) {
	tree := parse(t, `module M {
		type integer address;
		type record R {
			address field1,
			integer field2 optional
		}
		type component C {}
		testcase tc() runs on C {
			var R r := { field1 := 1, field2 := - };
			var integer v := r.field1;
		}
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	if containsCode(diags, "null-address-read") {
		t.Fatalf("unexpected null-address-read, got %v", codes(diags))
	}
}
