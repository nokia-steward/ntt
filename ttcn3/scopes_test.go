package ttcn3_test

import (
	"testing"

	"github.com/nokia/ntt/ttcn3"
)

// TestScope verifies that scopes are built and populated correctly.
func TestScopes(t *testing.T) {
	tests := []struct {
		input string
		names []string
	}{
		{`{var int x := x}`, []string{"x"}},
		{`{var int x := x; {var int y := x}}`, []string{"x"}},
		{`template int t<type T>(int x) := 1`, []string{"T", "x"}},
		{`function f<type T>(int p) {var int x}`, []string{"T", "p"}},
		{`type record { int x } R<type T>`, []string{"x", "T"}},
		{`type record { record { int y } x } R<type T>`, []string{"x", "T"}},
		{`type record of record { int x } R`, []string{}},
		{`signature S<type T>(int x) return A exception(B)`, []string{"T", "x"}},
		{`type union U<type T>{ int x }`, []string{"T", "x"}},
		{`type enumerated E<type T>{ E1, E2 }`, []string{"T", "E1", "E2"}},
		{`type function F<type T>(int p) runs on C return X`, []string{"T", "p"}},
		{`type port P<type T}> message { inout T; map param(int p)}`, []string{"T"}},
		{`type component C<type T> extends D {var T x}`, []string{"T", "x"}},
		{`for (var int i;true;i:=i+1) {var int x}`, []string{"i"}},
		{`if (var int i := 1; i > 0) {var int x}`, []string{"i"}},
		{`if (i := 1; i > 0) {var int x}`, []string{}},
		{`while (var int i := 1; i < 10) {var int x}`, []string{"i"}},
		{`select (var int i := 1; i) { case (1) {var int x} }`, []string{"i"}},
		{`select union (var U u := {}; u) { case (a) {var int x} }`, []string{"u"}},
		{`
			module M {
				import from foo all;
				friend module bar;
				group G { group G2 {
				var int x;
				}}
				control {};
				type enumerated E<type T> {E1}
			}`, []string{"foo", "x", "control", "E", "E1"}},
	}

	for _, tt := range tests {
		tree := ttcn3.Parse(tt.input)
		scp := ttcn3.NewScope(unwrapFirst(tree.Root), tree)
		if scp == nil {
			t.Errorf("%q: scope is nil", tt.input)
			continue
		}
		if actual := nameSlice(scp); !equal(actual, tt.names) {
			t.Errorf("%q: mismatch:\n\twant=%v\n\t got=%v", tt.input, tt.names, actual)
		}
	}
}
