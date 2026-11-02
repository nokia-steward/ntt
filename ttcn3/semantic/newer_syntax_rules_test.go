package semantic

import (
	"testing"

	"github.com/nokia/ntt/ttcn3"
)

// TestNewerSyntaxRules: what the parser accepts of variadic parameters
// (5.4.3), automatic types (6.5), embedded fields (6.2.1.4) and lazy or
// fuzzy references (16.1.4 m), the checker rejects where the standard
// does, and lets pass where it does not.
func TestNewerSyntaxRules(t *testing.T) {
	for _, tt := range []struct {
		name, src, code string
	}{
		{"variadic not last", `module M { function f(in integer i..., in integer max) {} }`, "variadic-parameter"},
		{"two variadics", `module M { function f(in integer a..., in integer b...) {} }`, "variadic-parameter"},
		{"variadic out", `module M { function f(out integer x...) {} }`, "variadic-parameter"},
		{"named actual expanded", `module M { function f(in integer args...) {} function g() { f(args := {1, 2}...); } }`, "variadic-parameter"},
		{"variadic ok", `module M { function f(in integer a, in integer r...) {} function g() { f(1); f(1, 2, 3); f(1, -); } }`, ""},
		{"automatic type of ?", `module M { modulepar template mpar := ?; }`, "automatic-type-unknown"},
		{"automatic type of *", `module M { modulepar template mpar := *; }`, "automatic-type-unknown"},
		{"automatic type ok", `module M { const X := 23; const Y := null; }`, ""},
		{"embedded field's field", `module M { type record B { integer b } type record A { B } const A x := { b := 1 } }`, "embedded-field-name"},
		{"embedded field", `module M { type record B { integer b } type record A { B } const A x := { B := { b := 1 } } }`, ""},
		{"lazy reference in a restricted context", `module M {
			type component C { var @lazy integer vc := 1; }
			function f() runs on C return template integer { log(vc); return ?; }
			function g() runs on C { template @fuzzy @deterministic integer t := f(); log(t); }
		}`, "restricted-context-side-effect"},
		{"deterministic lazy reference", `module M {
			type component C { var @lazy @deterministic integer vc := 1; }
			function f() runs on C return template integer { log(vc); return ?; }
			function g() runs on C { template @fuzzy @deterministic integer t := f(); log(t); }
		}`, ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			diags := NewAnalyzer(&ttcn3.DB{Modules: map[string]map[string]bool{}}).Analyze(parse(t, tt.src))
			for _, code := range []string{"variadic-parameter", "automatic-type-unknown", "embedded-field-name", "restricted-context-side-effect", "dash-without-default", "too-many-actual-parameters", "too-few-actual-parameters"} {
				if got := containsCode(diags, code); got != (code == tt.code) {
					t.Errorf("%s reported: %v, want %v (%v)", code, got, code == tt.code, codes(diags))
				}
			}
		})
	}
}
