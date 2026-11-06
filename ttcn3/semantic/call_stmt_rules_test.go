package semantic

import (
	"testing"

	"github.com/nokia/ntt/ttcn3"
)

func TestCallBlockElseClauseRejected(t *testing.T) {
	// NegSyn_220301_CallOperation_001
	tree := parse(t, `module M {
		signature S();
		type port P procedure { inout S }
		type component C { port P p }
		testcase tc() runs on C {
			p.call(S:{}) {
				[] p.getreply(S:?) { setverdict(pass); }
				[else] {}
			}
		}
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	if !containsCode(diags, "call-block-else-clause") {
		t.Fatalf("expected call-block-else-clause, got %v", codes(diags))
	}
}

func TestCallBlockAltstepRejected(t *testing.T) {
	// NegSyn_220301_CallOperation_002
	tree := parse(t, `module M {
		signature S();
		type port P procedure { inout S }
		type component C { port P p }
		altstep a_handleReply() runs on C {
			[] p.getreply {}
		}
		testcase tc() runs on C {
			p.call(S:{}) {
				[] p.getreply(S:?) { setverdict(pass); }
				[] a_handleReply() {}
			}
		}
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	if !containsCode(diags, "call-block-altstep-invocation") {
		t.Fatalf("expected call-block-altstep-invocation, got %v", codes(diags))
	}
}

func TestCallBlockWithGetreplyAccepted(t *testing.T) {
	tree := parse(t, `module M {
		signature S();
		type port P procedure { inout S }
		type component C { port P p }
		testcase tc() runs on C {
			p.call(S:{}) {
				[] p.getreply(S:?) { setverdict(pass); }
			}
		}
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	if containsCode(diags, "call-block-else-clause") ||
		containsCode(diags, "call-block-altstep-invocation") {
		t.Fatalf("unexpected diag on clean call block, got %v", codes(diags))
	}
}
