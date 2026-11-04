package semantic

import (
	"testing"

	"github.com/nokia/ntt/ttcn3"
)

// TestUnionAltRefNullVariableAccepted: comparing a variable that was
// assigned null references no alternative (6.2.14).
func TestUnionAltRefNullVariableAccepted(t *testing.T) {
	tree := parse(t, `module M {
		type component C {
			timer T := 5.0;
		}
		testcase TC() runs on C {
			var timer v_timer := T;
			v_timer := null;
			if (v_timer == null) {
				setverdict(pass);
			}
		}
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	if containsCode(diags, "union-alt-ref-bad-parent") {
		t.Fatalf("unexpected union-alt-ref-bad-parent, got %v", codes(diags))
	}
}

// TestUnionAltRefNullAlternativeRejected: NegSem_150605_..._006 still
// rejects referencing an alternative whose value is null.
func TestUnionAltRefNullAlternativeRejected(t *testing.T) {
	tree := parse(t, `module M {
		type integer address;
		type union U {
			integer u1,
			address u2
		}
		type component C {}
		testcase TC() runs on C {
			var template integer m;
			var template U t;
			t.u1 := 1;
			t.u2 := null;
			m := t.u2;
		}
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	if !containsCode(diags, "union-alt-ref-bad-parent") {
		t.Fatalf("expected union-alt-ref-bad-parent, got %v", codes(diags))
	}
}
