package semantic

import (
	"testing"

	"github.com/nokia/ntt/ttcn3"
)

func TestRecordFieldReadAfterReceiveValueRedirectAccepted(t *testing.T) {
	tree := parse(t, `module M {
		type record R {
			integer i
		}
		type port P message {
			inout R
		}
		type component C {
			port P p;
			var integer id;
		}
		function f() runs on C {
			var R msg;
			p.receive(R: ?) -> value msg;
			id := msg.i;
		}
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	if containsCode(diags, "uninit-record-field-read") {
		t.Fatalf("unexpected uninit-record-field-read, got %v", codes(diags))
	}
}

func TestRecordFieldReadAfterAltReceiveValueRedirectAccepted(t *testing.T) {
	tree := parse(t, `module M {
		type record R {
			integer i
		}
		type port P message {
			inout R
		}
		type component C {
			port P p;
			var integer id;
		}
		function f() runs on C {
			var R msg;
			alt {
				[] p.receive(R: ?) -> value msg { }
			}
			id := msg.i;
		}
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	if containsCode(diags, "uninit-record-field-read") {
		t.Fatalf("unexpected uninit-record-field-read, got %v", codes(diags))
	}
}

func TestRecordFieldReadBeforeInitialisationRejected(t *testing.T) {
	tree := parse(t, `module M {
		type record R {
			integer i
		}
		type component C {
			var integer id;
		}
		function f() runs on C {
			var R msg;
			id := msg.i;
		}
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	if !containsCode(diags, "uninit-record-field-read") {
		t.Fatalf("expected uninit-record-field-read, got %v", codes(diags))
	}
}
