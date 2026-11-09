package semantic

import (
	"testing"

	"github.com/nokia/ntt/ttcn3"
)

const declKindDecls = `
	type port P message { inout integer }
	type component C {}
	type component CP { port P p }
	type record Inner { P p }
	type record R { integer a, Inner b }
	type record D { default d }
	type record of C CL
	type R RA
`

// Some declarations may not be of, nor hold at any level of nesting, a
// component, port, timer or default type (ETSI 5.4.1.1 m, 8.2.1 b, 16.1.5
// b, c, d).
func TestDeclKinds(t *testing.T) {
	for name, c := range map[string]struct{ src, code string }{
		"a component modulepar":          {`modulepar C mp := null;`, "modulepar-kind"},
		"a modulepar holding a port":     {`modulepar R mp;`, "modulepar-kind"},
		"a modulepar of an alias":        {`modulepar RA mp;`, "modulepar-kind"},
		"a modulepar list of components": {`modulepar CL mp;`, "modulepar-kind"},
		"a modulepar of default type":    {`modulepar default mp := null;`, "modulepar-kind"},
		"a grouped modulepar":            {`modulepar { C mp1 := null }`, "modulepar-kind"},
		"a template's component param":   {`template boolean t(C c) := true;`, "template-param-kind"},
		"a control function's port":      {`function control(P p) {}`, "control-function-kind"},
		"an @control function's default": {`function @control f(D d) {}`, "control-function-kind"},
		"a control function's result":    {`function control() return R { return {1, {p := -}} }`, "control-function-kind"},
		"a control function's ports":     {`function control() runs on CP {}`, "control-function-kind"},
	} {
		tree := parse(t, "module M {"+declKindDecls+c.src+"\n}")
		if diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree); !containsCode(diags, c.code) {
			t.Errorf("%s: expected %s, got %v", name, c.code, codes(diags))
		}
	}
}

// What the standard allows stays allowed.
func TestDeclKindsAllowed(t *testing.T) {
	tree := parse(t, `module M {`+declKindDecls+`
		modulepar integer mp1 := 1;
		modulepar record { integer a } mp2 := {1};
		modulepar Unknown mp3;
		template integer t(integer i) := i;
		function control() runs on C {}
		function f(P p, C c, default d) runs on CP return R { return {1, {p := -}} }
	}`)
	for _, d := range NewAnalyzer(&ttcn3.DB{}).Analyze(tree) {
		switch d.Code {
		case "modulepar-kind", "template-param-kind", "control-function-kind":
			t.Errorf("unexpected %s: %s", d.Code, d.Message)
		}
	}
}
