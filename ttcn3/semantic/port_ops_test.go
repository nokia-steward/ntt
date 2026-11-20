package semantic

import (
	"testing"

	"github.com/nokia/ntt/ttcn3"
)

func TestPortOpReplyDirectionsAccepted(t *testing.T) {
	// 6.2.9: getreply and catch answer our own call, so they use
	// the out list; getcall, reply and raise use the in list.
	tree := parse(t, `module M {
		signature S(in integer p_x) return integer exception(charstring);
		type port P_out procedure { out S; }
		type port P_in procedure { in S; }
		type component C { port P_out po; port P_in pi; }
		testcase TC() runs on C {
			po.call(S:{1}, 1.0) {
				[] po.getreply(S:{-}) {}
				[] po.catch(S, charstring:?) {}
				[] po.catch(timeout) {}
			}
			pi.getcall(S:{?});
			pi.reply(S:{-} value 1);
			pi.raise(S, "e");
		}
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	if containsCode(diags, "port-op-type-not-allowed") {
		t.Fatalf("did not expect port-op-type-not-allowed, got %v", codes(diags))
	}
}

func TestPortOpGetreplyOnInListRejected(t *testing.T) {
	// A getreply on a signature that is only in the in list
	// cannot belong to a call made through this port.
	tree := parse(t, `module M {
		signature S(in integer p_x) return integer;
		type port P_in procedure { in S; }
		type component C { port P_in pi; }
		testcase TC() runs on C {
			pi.getreply(S:{-});
		}
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	if !containsCode(diags, "port-op-type-not-allowed") {
		t.Fatalf("expected port-op-type-not-allowed, got %v", codes(diags))
	}
}
