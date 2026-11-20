package semantic

import (
	"testing"

	"github.com/nokia/ntt/ttcn3"
)

func TestBareComponentKilledBlockRejected(t *testing.T) {
	tree := parse(t, `module M {
		type component C {}
		testcase tc() runs on C system C {
			alt {
				[] any component killed { setverdict(pass); }
			}
		}
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	if !containsCode(diags, "component-test-op-missing-receiver") {
		t.Fatalf("expected component-test-op-missing-receiver, got %v", codes(diags))
	}
}

func TestAnyComponentKilledDottedAccepted(t *testing.T) {
	tree := parse(t, `module M {
		type component C {}
		function f() runs on C {}
		testcase tc() runs on C system C {
			var C ptc := C.create alive;
			ptc.start(f());
			ptc.kill;
			alt {
				[] any component.killed { setverdict(pass); }
			}
		}
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	if containsCode(diags, "component-test-op-missing-receiver") {
		t.Fatalf("unexpected component-test-op-missing-receiver, got %v", codes(diags))
	}
}
