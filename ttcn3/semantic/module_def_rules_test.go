package semantic

import (
	"testing"

	"github.com/nokia/ntt/ttcn3"
)

// A module parameter is no target of an assignment and is declared once
// (ETSI 8.2.1 a, e); the module definitions part declares no variable
// (8.2.0); the right-hand operand of a shift or rotate is not negative
// (7.1.6, 7.1.7).
func TestModuleDefRules(t *testing.T) {
	for name, c := range map[string]struct{ src, code string }{
		"a modulepar assigned":       {`modulepar integer mp := 1; function f() { mp := 2 }`, "modulepar-assigned"},
		"a modulepar field assigned": {`type record R { integer a } modulepar R mp := {1}; function f() { mp.a := 2 }`, "modulepar-assigned"},
		"a modulepar declared twice": {`modulepar integer mp := 1; modulepar integer mp := 2;`, "duplicate-definition"},
		"in a modulepar group":       {`modulepar { integer mp := 1; integer mp := 2 }`, "duplicate-definition"},
		"a constant declared twice":  {`const integer c := 1; const integer c := 2;`, "duplicate-definition"},
		"a module variable":          {`var integer v := 1;`, "module-variable"},
		"a negative shift":           {`function f() { var bitstring b := '1010'B << -1 }`, "negative-shift"},
		"a negative rotate":          {`function f() { var bitstring b := '1010'B <@ (-2) }`, "negative-shift"},
	} {
		tree := parse(t, "module M {"+c.src+"\n}")
		if diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree); !containsCode(diags, c.code) {
			t.Errorf("%s: expected %s, got %v", name, c.code, codes(diags))
		}
	}
}

// What the standard allows stays allowed: reading a module parameter,
// a local of its name, two names in one declaration, a shift by zero or
// a positive amount.
func TestModuleDefRulesAllowed(t *testing.T) {
	tree := parse(t, `module M {
		type record R { integer mp }
		modulepar integer mp := 1;
		const integer c1 := 1, c2 := 2;
		function f() {
			var integer x := mp + 1;
			var R r := {mp := 2};
			var bitstring b := '1010'B << 0;
			b := b >> 2;
		}
		function g() { var integer mp := 3; mp := 4 }
	}`)
	for _, d := range NewAnalyzer(&ttcn3.DB{}).Analyze(tree) {
		switch d.Code {
		case "modulepar-assigned", "duplicate-definition", "module-variable", "negative-shift":
			t.Errorf("unexpected %s: %s", d.Code, d.Message)
		}
	}
}
