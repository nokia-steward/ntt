package semantic

import (
	"testing"

	"github.com/nokia/ntt/ttcn3"
)

func TestUnionAssignSharedAlternativeAccepted(t *testing.T) {
	// 6.3.2.4: only the selected alternative must exist in the
	// target; U1 -> U2 and back through the shared `i` are legal.
	tree := parse(t, `module M {
		type component C {}
		type union U1 { integer i }
		type union U2 { integer i, boolean b }
		type union UC { integer i }
		type union UA { integer i, charstring d }
		type union US { @default UC d, boolean b }
		testcase TC() runs on C {
			var U1 v_u1 := { i := 1 };
			var U2 v_u2 := v_u1;
			v_u1 := v_u2;
			var US v_s := { d := { i := 1 } };
			var UA v_a := v_s;
		}
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	if containsCode(diags, "union-assign-name-mismatch") {
		t.Fatalf("did not expect union-assign-name-mismatch, got %v", codes(diags))
	}
}

func TestUnionAssignNoSharedAlternativeRejected(t *testing.T) {
	// NegSem_060302_structured_types_010: no alternative of
	// ModifiedUnion is named in UnionType.
	tree := parse(t, `module M {
		type component C {}
		type union UnionType { integer a, boolean c }
		type union ModifiedUnion { integer aa, boolean cc }
		testcase TC() runs on C {
			var ModifiedUnion v_union1 := { aa := 1 };
			var UnionType v_union2;
			v_union2 := v_union1;
		}
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	if !containsCode(diags, "union-assign-name-mismatch") {
		t.Fatalf("expected union-assign-name-mismatch, got %v", codes(diags))
	}
}
