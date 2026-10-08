package semantic

import (
	"testing"

	"github.com/nokia/ntt/ttcn3"
)

func TestFromClauseNullVarRejected(t *testing.T) {
	// NegSem_220202_ReceiveOperation_015
	tree := parse(t, `module M {
		type port P message { inout integer; address integer; }
		type component C { port P p }
		testcase tc() runs on C system C {
			var C v_comp := null;
			alt {
				[] p.receive from v_comp {}
				[] p.receive {}
			}
		}
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	if !containsCode(diags, "from-clause-null-address") {
		t.Fatalf("expected from-clause-null-address, got %v", codes(diags))
	}
}

func TestFromClauseReassignedAccepted(t *testing.T) {
	tree := parse(t, `module M {
		type port P message { inout integer }
		type component C { port P p }
		testcase tc() runs on C system C {
			var C v_comp := null;
			v_comp := mtc;
			alt {
				[] p.receive from v_comp {}
				[] p.receive {}
			}
		}
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	if containsCode(diags, "from-clause-null-address") {
		t.Fatalf("unexpected diag, got %v", codes(diags))
	}
}

func TestFromClauseLiteralRefAccepted(t *testing.T) {
	tree := parse(t, `module M {
		type port P message { inout integer }
		type component C { port P p }
		testcase tc() runs on C system C {
			alt {
				[] p.receive from mtc {}
			}
		}
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	if containsCode(diags, "from-clause-null-address") {
		t.Fatalf("unexpected diag, got %v", codes(diags))
	}
}
