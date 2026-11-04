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

// TestRecordFieldReadInsideAltRedirectClauseAccepted covers the ordinary
// shape a receive-then-inspect testcase has: the `-> value` redirect and the
// field read are in the SAME alt statement, the redirect binding the variable
// before the clause body runs. The sibling test above reads the field after
// the alt, which the end-of-statement context clear already handled; reading
// it inside the clause body was reported as an uninitialised read.
func TestRecordFieldReadInsideAltRedirectClauseAccepted(t *testing.T) {
	tree := parse(t, `module M {
		type record R {
			integer status,
			charstring body
		}
		type port P message {
			out charstring;
			in R
		}
		type component C {
			port P p
		}
		testcase tc() runs on C system C {
			var R r;
			map(self:p, system:p);
			p.send("x");
			alt {
				[] p.receive(R: ?) -> value r { log("body: ", r.body); }
			}
			unmap(self:p, system:p);
		}
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	if containsCode(diags, "uninit-record-field-read") {
		t.Fatalf("unexpected uninit-record-field-read, got %v", codes(diags))
	}
}

// TestRecordFieldReadInSiblingClauseWithoutRedirectRejected keeps the fix
// above honest: the exemption is scoped to the clause that carries the
// redirect, so a sibling branch that binds nothing must still be reported.
func TestRecordFieldReadInSiblingClauseWithoutRedirectRejected(t *testing.T) {
	tree := parse(t, `module M {
		type record R {
			integer status,
			charstring body
		}
		type port P message {
			out charstring;
			in R
		}
		type component C {
			port P p
		}
		testcase tc() runs on C system C {
			var R r;
			alt {
				[] p.receive(R: ?) { log("body: ", r.body); }
				[] p.receive(R: ?) -> value r { log(r.status); }
			}
		}
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	if !containsCode(diags, "uninit-record-field-read") {
		t.Fatalf("expected uninit-record-field-read for the branch with no redirect, got %v", codes(diags))
	}
}

// TestRecordFieldAttributeRetrievalAccepted: retrieving an attribute
// (27.8) does not read the value preceding the dot, which may be
// uninitialized.
func TestRecordFieldAttributeRetrievalAccepted(t *testing.T) {
	tree := parse(t, `module M {
		type record of universal charstring RUC;
		type record R {
			integer f1,
			integer f2
		} with { encode (f1) "Rule" }
		type component C {}
		testcase tc() runs on C {
			var R v;
			var RUC e := v.f1.encode;
			var RUC w := v.f2.variant("Rule");
		}
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	if containsCode(diags, "uninit-record-field-read") {
		t.Fatalf("unexpected uninit-record-field-read, got %v", codes(diags))
	}
}

// TestRecordFieldReadBesideAttributeRetrievalRejected keeps the
// exemption to the attribute operand: a plain read of the same field
// is still reported.
func TestRecordFieldReadBesideAttributeRetrievalRejected(t *testing.T) {
	tree := parse(t, `module M {
		type record of universal charstring RUC;
		type record R {
			integer f1
		}
		type component C {}
		testcase tc() runs on C {
			var R v;
			var RUC e := v.f1.encode;
			var integer i := v.f1;
		}
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	if !containsCode(diags, "uninit-record-field-read") {
		t.Fatalf("expected uninit-record-field-read, got %v", codes(diags))
	}
}
