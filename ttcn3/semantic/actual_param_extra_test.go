package semantic

import (
	"fmt"
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

func TestPortParamReceivesSynonymPortRefAccepted(t *testing.T) {
	// 6.3.6: a port of a synonym type P2 may be passed to a P1
	// parameter.
	tree := parse(t, `module M {
		type port P1 message { inout integer }
		type P1 P2;
		type component C { port P2 p }
		function f_use(P1 p_port) {}
		testcase tc() runs on C {
			f_use(p);
		}
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	if containsCode(diags, "port-parameter-non-port-arg") {
		t.Fatalf("unexpected diag, got %v", codes(diags))
	}
}

func TestSynonymPortParamReceivesNonPortValueRejected(t *testing.T) {
	tree := parse(t, `module M {
		type port P1 message { inout integer }
		type P1 P2;
		type component C {}
		function f_use(P2 p_port) {}
		testcase tc() runs on C {
			var integer v_val := 5;
			f_use(v_val);
		}
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	if !containsCode(diags, "port-parameter-non-port-arg") {
		t.Fatalf("expected port-parameter-non-port-arg, got %v", codes(diags))
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

// A param redirect binds its variables, in list and in assignment
// notation (22.3.2); a variable it does not name stays unbound.
func TestRedirectParamBindsLocalVar(t *testing.T) {
	src := `module M {
		signature S(in integer p_x, in integer p_y) noblock;
		type port P procedure { inout S }
		type component C { port P p }
		function f_test(integer p_val) {}
		function f_server() runs on C {
			var integer v_x, v_y, v_z;
			p.getcall(S:{?, ?}) -> param(v_x, -);
			p.getcall(S:{?, ?}) -> param(v_y := p_y);
			f_test(v_x);
			f_test(v_y);
			%s
		}
	}`
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(parse(t, fmt.Sprintf(src, "")))
	if containsCode(diags, "uninitialised-arg-to-in-inout") {
		t.Fatalf("unexpected diag on param-redirect-bound var, got %v", codes(diags))
	}
	diags = NewAnalyzer(&ttcn3.DB{}).Analyze(parse(t, fmt.Sprintf(src, "f_test(v_z);")))
	if !containsCode(diags, "uninitialised-arg-to-in-inout") {
		t.Fatalf("expected uninitialised-arg-to-in-inout, got %v", codes(diags))
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
