package semantic

import (
	"testing"

	"github.com/nokia/ntt/ttcn3"
)

func TestIndexRedirectOnPlainReceiveRejected(t *testing.T) {
	// NegSem_220202_ReceiveOperation_017
	tree := parse(t, `module M {
		type port P message { inout integer; }
		type component C { port P p; }
		testcase tc() runs on C {
			var integer v_int;
			p.receive(integer:?) -> @index value v_int;
		}
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	if !containsCode(diags, "index-redirect-without-any-from") {
		t.Fatalf("expected index-redirect-without-any-from, got %v", codes(diags))
	}
}

func TestIndexRedirectOnAnyPortReceiveRejected(t *testing.T) {
	// NegSem_220202_ReceiveOperation_018
	tree := parse(t, `module M {
		type port P message { inout integer; }
		type component C { port P p; }
		testcase tc() runs on C {
			var integer v_int;
			any port.receive(integer:?) -> @index value v_int;
		}
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	if !containsCode(diags, "index-redirect-without-any-from") {
		t.Fatalf("expected index-redirect-without-any-from, got %v", codes(diags))
	}
}

func TestIndexRedirectOnAnyFromAccepted(t *testing.T) {
	tree := parse(t, `module M {
		type port P message { inout integer; }
		type component C { port P p[2]; }
		testcase tc() runs on C {
			var integer v_int;
			any from p.receive(integer:?) -> @index value v_int;
		}
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	if containsCode(diags, "index-redirect-without-any-from") {
		t.Fatalf("unexpected diag on any from form, got %v", codes(diags))
	}
}

func TestValueRedirectWithoutTemplateRejected(t *testing.T) {
	// NegSem_220202_ReceiveOperation_012
	tree := parse(t, `module M {
		type port P message { inout integer; }
		type component C { port P p; }
		testcase tc() runs on C {
			var integer v_val;
			p.receive -> value v_val;
		}
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	if !containsCode(diags, "value-redirect-without-template") {
		t.Fatalf("expected value-redirect-without-template, got %v", codes(diags))
	}
}

func TestValueRedirectWithTemplateAccepted(t *testing.T) {
	tree := parse(t, `module M {
		type port P message { inout integer; }
		type component C { port P p; }
		testcase tc() runs on C {
			var integer v_val;
			p.receive(integer:?) -> value v_val;
		}
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	if containsCode(diags, "value-redirect-without-template") {
		t.Fatalf("unexpected diag, got %v", codes(diags))
	}
}
