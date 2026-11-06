package semantic

import (
	"testing"

	"github.com/nokia/ntt/ttcn3"
)

func TestPortRefNotInRunsOnRejected(t *testing.T) {
	// NegSem_210102_disconnect_and_unmap_operations_008
	tree := parse(t, `module M {
		type port P message { inout integer }
		type component C { port P p }
		type component CEx extends C { port P p2 }
		function f_disconnect() runs on C {
			disconnect(self:p, self:p2);
		}
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	if !containsCode(diags, "port-ref-not-in-component") {
		t.Fatalf("expected port-ref-not-in-component, got %v", codes(diags))
	}
}

func TestPortRefOnExtendedComponentAccepted(t *testing.T) {
	tree := parse(t, `module M {
		type port P message { inout integer }
		type component C { port P p }
		type component CEx extends C { port P p2 }
		function f_disconnect() runs on CEx {
			disconnect(self:p, self:p2);
		}
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	if containsCode(diags, "port-ref-not-in-component") {
		t.Fatalf("unexpected diag, got %v", codes(diags))
	}
}

func TestPortRefInheritedPortAccepted(t *testing.T) {
	tree := parse(t, `module M {
		type port P message { inout integer }
		type component C { port P p }
		type component CEx extends C {}
		function f_disconnect() runs on CEx {
			disconnect(self:p, self:p);
		}
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	if containsCode(diags, "port-ref-not-in-component") {
		t.Fatalf("unexpected diag on inherited port, got %v", codes(diags))
	}
}
