package semantic

import (
	"testing"

	"github.com/nokia/ntt/ttcn3"
)

func TestMapAfterDisconnectAccepted(t *testing.T) {
	// 21.1.2: disconnect releases the ports, so mapping them
	// afterwards is legal; each disconnect form is exercised.
	for _, release := range []string{
		"disconnect(mtc:p, v_ptc:p);",
		"disconnect(v_ptc:p, self:p);",
		"disconnect(self:p);",
		"disconnect(v_ptc:p);",
		"disconnect(self:all port);",
		"disconnect(all component:all port);",
		"disconnect;",
		"disconnect();",
	} {
		tree := parse(t, `module M {
			type port P message { inout integer; }
			type component C { port P p; }
			testcase TC() runs on C system C {
				var C v_ptc := C.create;
				connect(mtc:p, v_ptc:p);
				`+release+`
				map(mtc:p, system:p);
			}
		}`)
		diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
		if containsCode(diags, "map-on-connected-port") {
			t.Errorf("%s: did not expect map-on-connected-port, got %v", release, codes(diags))
		}
	}
}

func TestMapOnStillConnectedPortRejected(t *testing.T) {
	// 9.1 Figure 7: disconnecting another port leaves p connected.
	tree := parse(t, `module M {
		type port P message { inout integer; }
		type component C { port P p, q; }
		testcase TC() runs on C system C {
			var C v_ptc := C.create;
			connect(mtc:p, v_ptc:p);
			connect(mtc:q, v_ptc:q);
			disconnect(mtc:q, v_ptc:q);
			map(self:p, system:p);
		}
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	if !containsCode(diags, "map-on-connected-port") {
		t.Fatalf("expected map-on-connected-port, got %v", codes(diags))
	}
}

func TestConnectAfterUnmapAccepted(t *testing.T) {
	tree := parse(t, `module M {
		type port P message { inout integer; }
		type component C { port P p; }
		testcase TC() runs on C system C {
			var C v_ptc := C.create;
			map(self:p, system:p);
			unmap(self:all port);
			connect(mtc:p, v_ptc:p);
		}
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	if containsCode(diags, "connect-on-mapped-port") {
		t.Fatalf("did not expect connect-on-mapped-port, got %v", codes(diags))
	}
}
