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
		`var template(value) integer t := ?;`:                           "template-restriction-violation",
		`var template integer t := 1; if (match(t, 1)) {}`:              "match-value-is-template",
		`var RoI v := { 1 }; for (var charstring e in v) {}`:            "range-loop-element-type",
		`var template integer t := ?; var integer v := t;`:              "template-to-value",
		`var template integer t := 1; var integer v; v := t;`:           "template-to-value",
		`var integer v; v := ?;`:                                        "template-to-value",
		`var template integer t := 1; var integer v := t + 1;`:          "template-operand",
		`var R v_r := { 1 }; var R2 v_2 := v_r;`:                        "type-incompatible-value",
		`var R v_r := { 1 }; var S v_s := v_r;`:                         "type-incompatible-value",
		`var RoI v_r := { 1 }; var SoI v_s; v_s := v_r;`:                "type-incompatible-assignment",
		`var integer x := 1; select (x) { case ("two") {} }`:            "select-case-type",
		`var integer x := 1; select union (x) { case (a) {} }`:          "select-union-header",
		`timer T := 1.0; var integer v; v := T.read;`:                   "type-incompatible-assignment",
		`var integer v := int2float(1);`:                                "type-incompatible-value",
		`var template integer v_t := "abc";`:                            "type-incompatible-value",
	}
	for body, code := range bad {
		tree := parse(t, `module M { type record R { integer a } type record R2 { boolean a } type record R3 { integer b } type set S { integer a } type record of integer RoI; type set of integer SoI; function f(in integer p_i) {} function g() { `+body+` } }`)
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
			var charstring v_back := v_u;
			var template integer t_i := ?;
			t_i := 5;
			var integer v_val := valueof(t_i);
			var charstring v_c := "x";
			v_u := v_c;
			f(v_m, v_c);
			if (v_u == v_c and v_i > 2) {}
		}
	}`
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(parse(t, good))
	for _, code := range []string{"type-incompatible-value", "type-incompatible-assignment", "type-incompatible-comparison", "type-boolean-operand", "type-incompatible-actual", "template-to-value", "template-operand"} {
		if containsCode(diags, code) {
			t.Errorf("valid module: unexpected %s %v", code, codes(diags))
		}
	}
}
