package semantic

import (
	"testing"

	"github.com/nokia/ntt/ttcn3"
)

func TestConcatAndContextRules(t *testing.T) {
	bad := map[string]string{
		`template bitstring t := '01'B & 'AB'O;`:                            "concat-operand",
		`template bitstring t := '01'B & ? length(1 .. 2) & '1'B;`:          "concat-operand",
		`template RoI t := { 1 } & * & { 3 };`:                              "concat-operand",
		`template charstring t := "A" & ("B", "C") & "D";`:                  "concat-operand",
		`template charstring t := (pattern @nocase "abc") & (pattern "d");`: "concat-operand",
		`template charstring t := '01'B & ? length(1);`:                     "concat-operand",
		`template integer t := -;`:                                          "not-used-symbol-template",
	}
	for decl, code := range bad {
		tree := parse(t, `module M { type record of integer RoI; `+decl+` }`)
		if diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree); !containsCode(diags, code) {
			t.Errorf("%s: expected %s, got %v", decl, code, codes(diags))
		}
	}
	good := []string{
		`template bitstring t := '01'B & ? & '1'B;`,
		`template bitstring t := '01'B & ? length(2) & '1'B;`,
		`template RoI t := { 1 } & * length(1 .. 2) & { 4 };`,
		`template charstring t := "A" & ? & "D";`,
		`template charstring t := (pattern @nocase "abc") & (pattern @nocase "d");`,
	}
	for _, decl := range good {
		tree := parse(t, `module M { type record of integer RoI; `+decl+` }`)
		if diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree); containsCode(diags, "concat-operand") {
			t.Errorf("%s: unexpected concat-operand", decl)
		}
	}
	tree := parse(t, `module M {
		type component C {}
		testcase tc() runs on C { return; }
		control { log(mtc); return; }
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	for _, code := range []string{"testcase-return", "control-part-return", "control-part-component-ref"} {
		if !containsCode(diags, code) {
			t.Errorf("expected %s, got %v", code, codes(diags))
		}
	}
}
