package semantic

import (
	"testing"

	"github.com/nokia/ntt/ttcn3"
)

func TestIndexRedirectFloatTargetRejected(t *testing.T) {
	// NegSem_210305_alive_operation_003: `@index value` target is a
	// float, which can never hold an index value.
	tree := parse(t, `module M {
		type component GeneralComp {}
		function f() runs on GeneralComp {}
		testcase tc() runs on GeneralComp system GeneralComp {
			var boolean v_isAlive;
			var GeneralComp v_ptc[4];
			var float v_index;
			v_isAlive := any from v_ptc.alive -> @index value v_index;
		}
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	if !containsCode(diags, "index-redirect-target-not-integer") {
		t.Fatalf("expected index-redirect-target-not-integer, got %v", codes(diags))
	}
}

func TestIndexRedirectIntegerTargetAccepted(t *testing.T) {
	tree := parse(t, `module M {
		type component GeneralComp {}
		function f() runs on GeneralComp {}
		testcase tc() runs on GeneralComp system GeneralComp {
			var boolean v_isAlive;
			var GeneralComp v_ptc[4];
			var integer v_index;
			v_isAlive := any from v_ptc.alive -> @index value v_index;
		}
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	if containsCode(diags, "index-redirect-target-not-integer") {
		t.Fatalf("unexpected index-redirect-target-not-integer on integer target, got %v", codes(diags))
	}
}

func TestIndexRedirectIntegerArrayTargetAccepted(t *testing.T) {
	// Multi-dimensional source: a record-of / array integer target is
	// valid and must not be flagged by the scalar type rule.
	tree := parse(t, `module M {
		type component GeneralComp {}
		function f() runs on GeneralComp {}
		testcase tc() runs on GeneralComp system GeneralComp {
			var boolean v_isAlive;
			var GeneralComp v_ptc[3][3];
			var integer v_index[2];
			v_isAlive := any from v_ptc.alive -> @index value v_index;
		}
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	if containsCode(diags, "index-redirect-target-not-integer") {
		t.Fatalf("unexpected index-redirect-target-not-integer on integer-array target, got %v", codes(diags))
	}
}
