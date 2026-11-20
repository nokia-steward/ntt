package semantic

import (
	"testing"

	"github.com/nokia/ntt/ttcn3"
)

func TestStartRunsOnStructurallyCompatibleAccepted(t *testing.T) {
	// 6.3.3 item 2: CExt holds identical copies of every CFlat
	// definition, so a function running on CFlat may start on it.
	tree := parse(t, `module M {
		type component CBase { var integer vc_i := 10; timer tc_t := 1.0; }
		type component CExt extends CBase { var float vc_f := 1.0; }
		type component CFlat {
			var integer vc_i := 10;
			timer tc_t := 1.0;
			var float vc_f := 1.0;
		}
		function f_check() runs on CFlat {}
		testcase TC() runs on CBase {
			var CExt v_ptc := CExt.create;
			v_ptc.start(f_check());
		}
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	if containsCode(diags, "start-runs-on-incompatible") {
		t.Fatalf("did not expect start-runs-on-incompatible, got %v", codes(diags))
	}
}

func TestStartRunsOnDifferentInitialValueRejected(t *testing.T) {
	// 6.3.3 c: same name and type but a different initial value is
	// not an identical definition.
	tree := parse(t, `module M {
		type component C { var integer vc_i := 1; }
		type component D { var integer vc_i := 2; }
		function f() runs on D {}
		testcase TC() runs on C {
			var C v_ptc := C.create;
			v_ptc.start(f());
		}
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	if !containsCode(diags, "start-runs-on-incompatible") {
		t.Fatalf("expected start-runs-on-incompatible, got %v", codes(diags))
	}
}

func TestStartRunsOnMissingDefinitionRejected(t *testing.T) {
	// NegSem_210302_Start_test_component_007: DifferentComp's
	// variable is absent from GeneralComp.
	tree := parse(t, `module M {
		type component GeneralComp { var charstring vc_str := "abc"; }
		type component DifferentComp { var integer vc_int := 0; }
		function f() runs on DifferentComp {}
		testcase TC() runs on GeneralComp {
			var GeneralComp v_ptc := GeneralComp.create alive;
			v_ptc.start(f());
		}
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	if !containsCode(diags, "start-runs-on-incompatible") {
		t.Fatalf("expected start-runs-on-incompatible, got %v", codes(diags))
	}
}
