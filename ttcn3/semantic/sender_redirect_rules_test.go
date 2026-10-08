package semantic

import (
	"testing"

	"github.com/nokia/ntt/ttcn3"
)

func TestSenderAddressRedirectOnConnectedReceiveRejected(t *testing.T) {
	tree := parse(t, `module M {
		type port P message {
			inout integer;
			address integer;
		}
		type component C { port P p; }
		testcase tc() runs on C system C {
			var P.address v_addr;
			connect(self:p, self:p);
			p.receive -> sender v_addr;
		}
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	if !containsCode(diags, "sender-address-on-connected-port") {
		t.Fatalf("expected sender-address-on-connected-port, got %v", codes(diags))
	}
}

func TestSenderAddressRedirectOnConnectedTriggerRejected(t *testing.T) {
	tree := parse(t, `module M {
		type port P message {
			inout integer;
			address integer;
		}
		type component C { port P p; }
		testcase tc() runs on C system C {
			var P.address v_addr;
			connect(self:p, self:p);
			p.trigger -> sender v_addr;
		}
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	if !containsCode(diags, "sender-address-on-connected-port") {
		t.Fatalf("expected sender-address-on-connected-port, got %v", codes(diags))
	}
}

func TestSenderAddressRedirectWithoutConnectAccepted(t *testing.T) {
	tree := parse(t, `module M {
		type port P message {
			inout integer;
			address integer;
		}
		type component C { port P p; }
		testcase tc() runs on C system C {
			var P.address v_addr;
			map(self:p, system:p);
			p.receive -> sender v_addr;
		}
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	if containsCode(diags, "sender-address-on-connected-port") {
		t.Fatalf("unexpected sender-address-on-connected-port, got %v", codes(diags))
	}
}
