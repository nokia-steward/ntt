package semantic

import (
	"testing"

	"github.com/nokia/ntt/ttcn3"
)

func TestRepeatedPortStartAccepted(t *testing.T) {
	// 22.5.2: `p.start` on a port, a component-body timer and
	// `all port.start` restart them; no component is started.
	tree := parse(t, `module M {
		type port P message { inout integer }
		type component C { port P p1, p2; timer T := 1.0; }
		testcase TC() runs on C {
			p2.start;
			T.start;
			p2.halt;
			p2.start;
			T.start;
			all port.start;
			all port.halt;
			all port.start;
		}
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	if containsCode(diags, "start-without-sync") {
		t.Fatalf("did not expect start-without-sync, got %v", codes(diags))
	}
}

func TestRepeatedComponentStartRejected(t *testing.T) {
	// NegSem_210302_Start_test_component_004
	tree := parse(t, `module M {
		type component GeneralComp {}
		function f() runs on GeneralComp {}
		testcase TC() runs on GeneralComp {
			var GeneralComp v_ptc := GeneralComp.create("PTC") alive;
			v_ptc.start(f());
			v_ptc.start(f());
		}
	}`)
	for _, db := range []*ttcn3.DB{{}, nil} {
		diags := NewAnalyzer(db).Analyze(tree)
		if !containsCode(diags, "start-without-sync") {
			t.Fatalf("expected start-without-sync (db %v), got %v", db, codes(diags))
		}
	}
}
