package semantic

import (
	"slices"
	"testing"

	"github.com/nokia/ntt/ttcn3"
)

// Templates and enumerated values passed to predefined functions
// (C.1.30, C.2.2, C.4.2, C.5).
func TestPredefinedTemplateArgRules(t *testing.T) {
	cases := []struct {
		name, stmt, want string
	}{
		{"encoding a template of AnyValue", "var universal charstring v := encvalue_unichar(m_any);", "encode-template-not-specific"},
		{"encoding a template with a range field", "var bitstring v := encvalue(m_range);", "encode-template-not-specific"},
		{"sizeof of a template with AnyValueOrNone", "var integer v := sizeof(mw_star);", "sizeof-template-size-varies"},
		{"sizeof of a template with ifpresent", "var integer v := sizeof(mw_ifp);", "sizeof-template-size-varies"},
		{"substr of a template with a value list", "var template RoI v := substr(t_list, 0, 1);", "substr-forbidden-matching-mechanism"},
		{"substr returning AnyElement", "var template RoI v := substr(t_any, 0, 2);", "substr-returns-any-element"},
		{"enum2int of a bare label", "var integer v := enum2int(e_b);", "enum2int-untyped"},

		{"encoding a specific template", "var bitstring v := encvalue(m_rec);", ""},
		{"sizeof of a template of present fields", "var integer v := sizeof(mw_fixed);", ""},
		{"substr not returning AnyElement", "var template RoI v := substr(t_any, 2, 1);", ""},
		{"enum2int of a variable", "var E v_e := e_b; var integer v := enum2int(v_e);", ""},
	}
	ruleCodes := []string{"encode-template-not-specific", "sizeof-template-size-varies", "substr-forbidden-matching-mechanism", "substr-returns-any-element", "enum2int-untyped"}
	for _, c := range cases {
		tree := parse(t, `module M {
			type enumerated E { e_a, e_b }
			type record of integer RoI;
			type record R { integer f1, integer f2 }
			type set S { integer f1, bitstring f2 optional, charstring f3 optional }
			template integer m_any := ?;
			template R m_range := { f1 := 1, f2 := (1 .. 5) };
			template R m_rec := { f1 := 1, f2 := 2 };
			template S mw_star := { f1 := 1, f2 := '1'B, f3 := * };
			template S mw_ifp := { f1 := 1, f2 := '00'B ifpresent, f3 := omit };
			template S mw_fixed := { f1 := 1, f2 := ?, f3 := omit };
			template RoI t_list := { 1, (2, 3), 4 };
			template RoI t_any := { 1, ?, 3 };
			type component C {}
			testcase TC() runs on C {
				`+c.stmt+`
			}
		}`)
		got := codes(NewAnalyzer(&ttcn3.DB{}).Analyze(tree))
		var hit []string
		for _, code := range got {
			if slices.Contains(ruleCodes, code) {
				hit = append(hit, code)
			}
		}
		switch {
		case c.want == "" && len(hit) > 0:
			t.Errorf("%s: unexpected %v", c.name, hit)
		case c.want != "" && !slices.Contains(got, c.want):
			t.Errorf("%s: expected %s, got %v", c.name, c.want, got)
		}
	}
}
