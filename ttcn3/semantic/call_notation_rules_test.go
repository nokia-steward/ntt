package semantic

import (
	"testing"

	"github.com/nokia/ntt/ttcn3"
)

func TestCallNotationRules(t *testing.T) {
	cases := []struct{ name, src, code string }{
		{"unknown named parameter", `module M {
			function f(integer p_a, integer p_b := 2) {}
			function g() { f(p_a := 1, p_c := 3); }
		}`, "named-arg-unknown"},
		{"empty template parentheses", `module M { template integer m_t() := 1; }`, "template-empty-parentheses"},
		{"void function as actual parameter", `module M {
			function f_void() {}
			function f(integer p) {}
			function g() { f(f_void()); }
		}`, "void-function-actual-parameter"},
		{"assignment notation on both sides", `module M {
			function g() { if ({ a := 1 } == { a := 1 }) {} }
		}`, "compound-comparison-notation"},
	}
	for _, c := range cases {
		diags := NewAnalyzer(&ttcn3.DB{}).Analyze(parse(t, c.src))
		if !containsCode(diags, c.code) {
			t.Errorf("%s: expected %s, got %v", c.name, c.code, codes(diags))
		}
	}
	ok := `module M {
		type component C {}
		function f_void() runs on C {}
		function f(integer p_a, integer p_b := 2) {}
		template integer m_p(integer p) := p;
		testcase tc() runs on C {
			var C v := C.create;
			v.start(f_void());
			f(p_a := 1, p_b := 3);
			if ({ 1, 2 } == { 1, 2 }) {}
		}
	}`
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(parse(t, ok))
	for _, code := range []string{"named-arg-unknown", "template-empty-parentheses", "void-function-actual-parameter", "compound-comparison-notation"} {
		if containsCode(diags, code) {
			t.Errorf("valid module: unexpected %s", code)
		}
	}
}
