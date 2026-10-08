package semantic

import (
	"testing"

	"github.com/nokia/ntt/ttcn3"
)

func TestPortParamReceivesNonPortValueRejected(t *testing.T) {
	// NegSem_050402_actual_parameters_099
	tree := parse(t, `module M {
		type port IntPort message { inout integer }
		type component C { port IntPort p }
		function f_test(IntPort p_port) {}
		testcase tc() runs on C {
			var integer v_val := 5;
			f_test(v_val);
		}
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	if !containsCode(diags, "port-parameter-non-port-arg") {
		t.Fatalf("expected port-parameter-non-port-arg, got %v", codes(diags))
	}
}

func TestPortParamReceivesPortRefAccepted(t *testing.T) {
	tree := parse(t, `module M {
		type port IntPort message { inout integer }
		type component C { port IntPort p }
		function f_test(IntPort p_port) runs on C {}
		testcase tc() runs on C {
			f_test(p);
		}
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	if containsCode(diags, "port-parameter-non-port-arg") {
		t.Fatalf("unexpected diag, got %v", codes(diags))
	}
}

func TestUnboundVarToInRejected(t *testing.T) {
	// NegSem_050402_actual_parameters_119
	tree := parse(t, `module M {
		type record R { integer field1, integer field2 optional }
		type component C {}
		function f_test(R p_val) {}
		testcase tc() runs on C {
			var R v_rec;
			f_test(v_rec);
		}
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	if !containsCode(diags, "uninitialised-arg-to-in-inout") {
		t.Fatalf("expected uninitialised-arg-to-in-inout, got %v", codes(diags))
	}
}

func TestPartiallyInitialisedVarAccepted(t *testing.T) {
	// Sem_050402_actual_parameters_191
	tree := parse(t, `module M {
		type record R { integer field1, integer field2 optional }
		type component C {}
		function f_test(R p_val) {}
		testcase tc() runs on C {
			var R v_rec;
			v_rec.field1 := 1;
			f_test(v_rec);
		}
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	if containsCode(diags, "uninitialised-arg-to-in-inout") {
		t.Fatalf("unexpected diag, got %v", codes(diags))
	}
}

func TestRedirectValueBindsLocalVar(t *testing.T) {
	tree := parse(t, `module M {
		type port P message { inout integer }
		type component C { port P p }
		function f_test(integer p_val) {}
		function f_receiver() runs on C {
			var integer v_val;
			alt {
				[] p.receive(integer:?) -> value v_val {}
			}
			f_test(v_val);
		}
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	if containsCode(diags, "uninitialised-arg-to-in-inout") {
		t.Fatalf("unexpected diag on redirect-bound var, got %v", codes(diags))
	}
}

func TestStringElementToInoutRejected(t *testing.T) {
	// NegSem_050402_actual_parameters_097
	tree := parse(t, `module M {
		type component C {}
		function f_test(inout charstring p_val) {}
		testcase tc() runs on C {
			var charstring v_val := "test";
			f_test(v_val[0]);
		}
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	if !containsCode(diags, "string-element-to-out-inout") {
		t.Fatalf("expected string-element-to-out-inout, got %v", codes(diags))
	}
}

func TestPlainStringVarToInoutAccepted(t *testing.T) {
	tree := parse(t, `module M {
		type component C {}
		function f_test(inout charstring p_val) {}
		testcase tc() runs on C {
			var charstring v_val := "test";
			f_test(v_val);
		}
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	if containsCode(diags, "string-element-to-out-inout") {
		t.Fatalf("unexpected diag, got %v", codes(diags))
	}
}
