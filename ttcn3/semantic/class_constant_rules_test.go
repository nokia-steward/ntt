package semantic

import (
	"testing"

	"github.com/nokia/ntt/ttcn3"
)

// No constant or module parameter is of class type, or of a structured
// type with a field or element of one (ES 203 790 5.1.1, restriction d);
// a variable is, and a class's own constant is a field of the object.
func TestClassTypedConstants(t *testing.T) {
	for src, want := range map[string]bool{
		`module M { type class K {} const K c := K.create(); }`:                                                    true,
		`module M { type class K {} modulepar K p; }`:                                                              true,
		`module M { type class K {} modulepar { K p } }`:                                                           true,
		`module M { type class K {} type record R { K k } type record of R RL; const RL c := {}; }`:                true,
		`module M { type class K {} type component C {} function f() runs on C { const K c := null; } }`:           true,
		`module M { type class K {} type component C {} function f() runs on C { var K v := null; } }`:             false,
		`module M { type class K { const integer c := 1; } type record R { integer i } const R c := { i := 1 }; }`: false,
		`module M { type class K { const K c := null; public function f() { const K l := null; } } }`:              true,
		`module M { type class K { const K c := null; } }`:                                                         false,
	} {
		diags := NewAnalyzer(&ttcn3.DB{}).Analyze(parse(t, src))
		if got := containsCode(diags, "class-typed-constant"); got != want {
			t.Errorf("%s: class-typed-constant = %v, want %v (%v)", src, got, want, codes(diags))
		}
	}
}
