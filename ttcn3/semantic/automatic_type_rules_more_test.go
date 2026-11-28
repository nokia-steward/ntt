package semantic

import (
	"testing"

	"github.com/nokia/ntt/ttcn3"
)

// What gives no type for a declaration without one (ETSI 6.5).
func TestAutomaticTypeProblems(t *testing.T) {
	for _, init := range []string{
		`{}`, `red`, `{ null, null }`, `{ a := 1, b := null }`,
		`{ [0] := null, [1] := null }`, `(?, ?)`, `superset(null, null)`,
	} {
		tree := parse(t, `module M {
			type enumerated E { red, green }
			function f() { var template v_auto := `+init+`; }
		}`)
		if diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree); !containsCode(diags, "automatic-type-unknown") {
			t.Errorf("%s: expected automatic-type-unknown, got %v", init, codes(diags))
		}
	}
	for _, init := range []string{
		`{ x := 1, y := omit }`, `{ {}, { 1 }, { 3 } }`, `E:red`, `(1, ?)`, `{ 1, 2 }`,
	} {
		tree := parse(t, `module M {
			type enumerated E { red, green }
			function f() { var template v_auto := `+init+`; }
		}`)
		if diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree); containsCode(diags, "automatic-type-unknown") {
			t.Errorf("%s: unexpected automatic-type-unknown", init)
		}
	}
}

// A field of an automatic record type is optional only when initialised
// with omit (ETSI 6.5).
func TestAutomaticTypeMandatoryField(t *testing.T) {
	for body, reject := range map[string]bool{
		`var v := { a := 1, b := 2 }; v.b := omit;`:    true,
		`var v := { a := 1, b := omit }; v.b := omit;`: false,
		`var v := { a := 1, b := 2 }; v.b := 3;`:       false,
	} {
		tree := parse(t, `module M { function f() { `+body+` } }`)
		if got := containsCode(NewAnalyzer(&ttcn3.DB{}).Analyze(tree), "omit-mandatory-field"); got != reject {
			t.Errorf("%s: omit-mandatory-field reported: %v, want %v", body, got, reject)
		}
	}
}
