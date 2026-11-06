package semantic

import (
	"testing"

	"github.com/nokia/ntt/ttcn3"
)

func analyzeCodes(t *testing.T, src string) []string {
	t.Helper()
	return codes(NewAnalyzer(&ttcn3.DB{}).Analyze(parse(t, src)))
}

func hasCode(cs []string, code string) bool {
	for _, c := range cs {
		if c == code {
			return true
		}
	}
	return false
}

// A V5 module takes enumerators of integer expressions, an integer named
// of a label's own, and labels compared alone; V4 rejects all three.
func TestEnumV5AcceptsWhatV4Rejects(t *testing.T) {
	body := `{
		type enumerated E { A, B(1..9, 10+1), C }
		type component Comp {}
		testcase tc() runs on Comp {
			const E e := A(0);
			var boolean b := A == C;
		}
	}`
	v4 := analyzeCodes(t, "module M "+body)
	for _, code := range []string{"enum-user-value-not-literal", "enum-ordinal-in-value-ref"} {
		if !hasCode(v4, code) {
			t.Errorf("V4: expected %s, got %v", code, v4)
		}
	}
	if v5 := analyzeCodes(t, `module M language "TTCN-3:5" `+body); len(v5) != 0 {
		t.Errorf("V5: expected no diagnostics, got %v", v5)
	}
}

func TestEnumV5Rules(t *testing.T) {
	tests := []struct {
		name, decls, body, code string
	}{
		{"no integer left", `type enumerated E { A(0..infinity), B }`, ``, "enum-no-integer-left"},
		{"integer not of the label", `type enumerated E { A, B(1..10) }`, `const E e := A(1);`, "enum-value-not-of-label"},
		{"label of several integers", `type enumerated E { A, B(1..10) }`, `const E e := B;`, "enum-value-ambiguous"},
		{"label of several types", `type enumerated E1 { Sat, Sun } type enumerated E2 { Mon, Sun }`, `const e := Sun;`, "enum-label-ambiguous"},
		{"compared labels of several types", `type enumerated E1 { Sat, Sun } type enumerated E2 { Sat, Sun }`, `var boolean b := Sat < Sun;`, "enum-label-ambiguous"},
		{"hidden by a module definition", `type enumerated E { Sat, Sun } const integer Sun := 1;`, ``, "enum-label-hidden"},
		{"hidden by a local definition", `type enumerated E { Sat, Sun }`, `const integer Sun := 1; var E e;`, "enum-label-hidden"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src := `module M language "TTCN-3:5" { ` + tt.decls + ` control { ` + tt.body + ` } }`
			if cs := analyzeCodes(t, src); !hasCode(cs, tt.code) {
				t.Errorf("expected %s, got %v", tt.code, cs)
			}
			// V4 reads these differently: none is V4's.
			if cs := analyzeCodes(t, "module M "+src[len(`module M language "TTCN-3:5" `):]); hasCode(cs, tt.code) {
				t.Errorf("V4: unexpected %s", tt.code)
			}
		})
	}
}

// What the V5 rules cannot tell they leave alone: a name of several types
// in the module, a field's or a parameter's `:=` in a notation, two labels
// compared that are of one type together, a local out of scope where the
// type is, and an integer at the limits of int64.
func TestEnumV5RulesNoFalsePositives(t *testing.T) {
	tests := []struct{ name, src string }{
		{"parameter of another type", `type enumerated E1 { A(1..3), B } type enumerated E2 { A(7), C }
			function a() { var E1 x := B; }
			function b(inout E2 x) { x := A(7); }`},
		{"field of another type", `type enumerated E1 { A(1..3), B } type enumerated E2 { A(7), C }
			type record R { E2 x }
			function a() { var E1 x := B; }
			function b() { var R r := { x := A(7) }; }`},
		{"labels of one type together", `type enumerated E1 { A, B } type enumerated E2 { A }
			function f() return boolean { return A == B; }`},
		{"local in a sibling block", `type enumerated E { A, B }
			function f(boolean c) { if (c) { var integer A := 1; } else { var E e := B; } }`},
		{"integer at the limit of int64", `type enumerated E { A(0..9223372036854775807), B }`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if cs := analyzeCodes(t, `module M language "TTCN-3:5" { `+tt.src+` }`); len(cs) != 0 {
				t.Errorf("expected no diagnostics, got %v", cs)
			}
		})
	}
}
