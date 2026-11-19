package semantic

import (
	"testing"

	"github.com/nokia/ntt/ttcn3"
)

func TestTypeRules(t *testing.T) {
	bad := map[string]string{
		`var integer v := 1.0;`:                                         "type-incompatible-value",
		`var v_int := 1; v_int := 1.0;`:                                 "type-incompatible-assignment",
		`var integer v_i := 1; var float v_f := 2.0; if (v_i < v_f) {}`: "type-incompatible-comparison",
		`var boolean b := true; var integer i := 1; if (b or i) {}`:     "type-boolean-operand",
		`var charstring v_s := "1"; f(v_s);`:                            "type-incompatible-actual",
		`var integer x := 1; select (x) { case ("two") {} }`:            "select-case-type",
		`var integer x := 1; select union (x) { case (a) {} }`:          "select-union-header",
		`timer T := 1.0; var integer v; v := T.read;`:                   "type-incompatible-assignment",
		`var integer v := int2float(1);`:                                "type-incompatible-value",
	}
	for body, code := range bad {
		tree := parse(t, `module M { function f(in integer p_i) {} function g() { `+body+` } }`)
		if diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree); !containsCode(diags, code) {
			t.Errorf("%s: expected %s, got %v", body, code, codes(diags))
		}
	}
	good := `module M {
		type integer MyInt (0 .. 10);
		function f(in integer p_i, universal charstring p_u) {}
		function g() {
			var MyInt v_m := 5;
			var integer v_i := v_m;
			var universal charstring v_u := "abc";
			var charstring v_c := "x";
			v_u := v_c;
			f(v_m, v_c);
			if (v_u == v_c and v_i > 2) {}
		}
	}`
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(parse(t, good))
	for _, code := range []string{"type-incompatible-value", "type-incompatible-assignment", "type-incompatible-comparison", "type-boolean-operand", "type-incompatible-actual"} {
		if containsCode(diags, code) {
			t.Errorf("valid module: unexpected %s %v", code, codes(diags))
		}
	}
}
