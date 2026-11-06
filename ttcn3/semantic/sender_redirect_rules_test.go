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

func TestSenderRedirectOfConnectedPeerTypeAccepted(t *testing.T) {
	// The sender of a message is the connected PTC, of type CC; the
	// redirect target need not have the runs-on type CS.
	tree := parse(t, `module M {
		type port P message { inout integer }
		type component CS { port P p; }
		type component CC { port P p; }
		function f() runs on CC { p.send(1); }
		testcase TC() runs on CS {
			var CC v_ptc := CC.create;
			var CC v_sender;
			connect(mtc:p, v_ptc:p);
			v_ptc.start(f());
			p.receive(integer:?) -> sender v_sender;
		}
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	if containsCode(diags, "sender-redirect-component-mismatch") {
		t.Fatalf("unexpected sender-redirect-component-mismatch, got %v", codes(diags))
	}
}

func TestSenderRedirectWithoutKnownPeersAccepted(t *testing.T) {
	// No connect names the components that send on p.
	tree := parse(t, `module M {
		type port P message { inout integer }
		type component C { port P p; }
		type component D { port P p; var integer v_extra; }
		testcase TC() runs on C {
			var D v_sender;
			p.receive(integer:?) -> sender v_sender;
		}
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	if containsCode(diags, "sender-redirect-component-mismatch") {
		t.Fatalf("unexpected sender-redirect-component-mismatch, got %v", codes(diags))
	}
}

func TestSenderRedirectIncompatibleWithConnectedPeerRejected(t *testing.T) {
	// NegSem_220302_getcall_operation_019: the PTC's p is connected
	// to the mtc, a GeneralComp, which an AltComp variable cannot hold.
	tree := parse(t, `module M {
		signature S();
		type port P procedure { inout S }
		type component GeneralComp { port P p; }
		type component AltComp { var integer vc_int; }
		function f() runs on GeneralComp {
			var AltComp v_compRef := null;
			p.getcall(S:{}) -> sender v_compRef;
		}
		testcase TC() runs on GeneralComp system GeneralComp {
			var GeneralComp v_ptc := GeneralComp.create;
			connect(self:p, v_ptc:p);
			v_ptc.start(f());
			p.call(S:{}, nowait);
		}
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	if !containsCode(diags, "sender-redirect-component-mismatch") {
		t.Fatalf("expected sender-redirect-component-mismatch, got %v", codes(diags))
	}
}
