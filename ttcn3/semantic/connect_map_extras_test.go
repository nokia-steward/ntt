package semantic

import (
	"testing"

	"github.com/nokia/ntt/ttcn3"
)

func TestConnectOnSystemPortRejected(t *testing.T) {
	// NegSem_210101_*_019: connect(mtc:p, system:p)
	tree := parse(t, `module M {
		type port P message { inout integer }
		type component C { port P p }
		testcase tc() runs on C system C {
			connect(mtc:p, system:p);
		}
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	if !containsCode(diags, "connect-on-system-port") {
		t.Fatalf("expected connect-on-system-port, got %v", codes(diags))
	}
}

func TestConnectBetweenComponentsAccepted(t *testing.T) {
	tree := parse(t, `module M {
		type port P message { inout integer }
		type component C { port P p }
		testcase tc() runs on C system C {
			var C ptc := C.create;
			connect(mtc:p, ptc:p);
		}
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	if containsCode(diags, "connect-on-system-port") {
		t.Fatalf("unexpected connect-on-system-port, got %v", codes(diags))
	}
}

func TestMapBothComponentsRejected(t *testing.T) {
	// NegSem_210101_*_020: map(mtc:p, v_ptc:p)
	tree := parse(t, `module M {
		type port P message { inout integer }
		type component C { port P p }
		testcase tc() runs on C system C {
			var C v_ptc := C.create;
			map(mtc:p, v_ptc:p);
		}
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	if !containsCode(diags, "map-requires-one-system-port") {
		t.Fatalf("expected map-requires-one-system-port, got %v", codes(diags))
	}
}

func TestMapWithSystemAccepted(t *testing.T) {
	tree := parse(t, `module M {
		type port P message { inout integer }
		type component C { port P p }
		testcase tc() runs on C system C {
			map(mtc:p, system:p);
		}
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	if containsCode(diags, "map-requires-one-system-port") {
		t.Fatalf("unexpected map-requires-one-system-port, got %v", codes(diags))
	}
}
