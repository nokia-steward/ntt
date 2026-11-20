package semantic

import (
	"testing"

	"github.com/nokia/ntt/ttcn3"
)

func TestAnyFromSinglePortRejected(t *testing.T) {
	tree := parse(t, `module M {
		signature S();
		type port P procedure { inout S }
		type component C { port P p }
		function f() runs on C {
			alt {
				[] any from p.getcall { setverdict(pass); }
			}
		}
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	if !containsCode(diags, "any-from-non-port-array") {
		t.Fatalf("expected any-from-non-port-array, got %v", codes(diags))
	}
}

func TestAnyFromPortArrayAccepted(t *testing.T) {
	tree := parse(t, `module M {
		signature S();
		type port P procedure { inout S }
		const integer c := 4;
		type component C { port P p[c] }
		function f() runs on C {
			alt {
				[] any from p.getcall { setverdict(pass); }
			}
		}
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	if containsCode(diags, "any-from-non-port-array") {
		t.Fatalf("unexpected diag, got %v", codes(diags))
	}
}

func TestAnyFromNonPortVarRejected(t *testing.T) {
	tree := parse(t, `module M {
		signature S();
		type port P procedure { inout S }
		type component C {
			var anytype p;
		}
		function f() runs on C {
			alt {
				[] any from p.getcall { setverdict(pass); }
			}
		}
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	if !containsCode(diags, "any-from-non-port-ref") {
		t.Fatalf("expected any-from-non-port-ref, got %v", codes(diags))
	}
}

func TestValueRedirectForbiddenOnGetcall(t *testing.T) {
	tree := parse(t, `module M {
		signature S();
		type port P procedure { inout S }
		type component C { port P PCO }
		function f() runs on C {
			var integer v;
			alt {
				[] PCO.getcall(S:?) -> value v { setverdict(pass); }
			}
		}
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	if !containsCode(diags, "value-redirect-forbidden-op") {
		t.Fatalf("expected value-redirect-forbidden-op, got %v", codes(diags))
	}
}

func TestBareSelectorPortOpKindMismatch(t *testing.T) {
	tree := parse(t, `module M {
		type port mp message { inout integer }
		type component C { port mp m }
		function f() runs on C {
			alt {
				[] m.getcall { setverdict(fail); }
			}
		}
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	if !containsCode(diags, "port-op-wrong-port-kind") {
		t.Fatalf("expected port-op-wrong-port-kind, got %v", codes(diags))
	}
}

func TestElementValueConstraintArrayRejected(t *testing.T) {
	tree := parse(t, `module M {
		type integer A[5] (1..10);
		type component C {}
		testcase tc() runs on C {
			var A v := { 8, 11, 2, 3, 4 };
		}
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	if !containsCode(diags, "element-value-constraint-violation") {
		t.Fatalf("expected element-value-constraint-violation, got %v", codes(diags))
	}
}
