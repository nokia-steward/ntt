package semantic

import (
	"slices"
	"testing"

	"github.com/nokia/ntt/ttcn3"
)

// The parameters of a signature template are not referenced with dot
// notation (15.6.4), nor an in-line template at all (15.4); an array
// element takes values of the element type (6.2.7).
func TestSignatureParamRefRules(t *testing.T) {
	cases := []struct {
		name, defs, want string
	}{
		{"parameter of a signature template", "template integer t_par := s_sig.par1;", "signature-template-parameter-reference"},
		{"parameter in a function", "function f() { var integer v := valueof(s_sig.par2); }", "signature-template-parameter-reference"},
		{"field of an in-line template", "function f() { var template integer v_t := R:{ f := ? }.f; }", "inline-template-reference"},
		{"element of a value written in place", "function f() { var integer v := { 1, 2 }[0]; }", "inline-template-reference"},
		{"charstring into an integer array element", `function f() { var integer v_a[2]; v_a[1] := "two"; }`, "type-incompatible-assignment"},
		{"charstring into an element of a 2-dimensional array", `function f() { var integer v_a[2][2]; v_a[1][0] := "two"; }`, "type-incompatible-assignment"},

		{"modified signature template", "template S s_mod modifies s_sig := { par1 := 3 };", ""},
		{"field of a record template", "template integer t_f := t_rec.f;", ""},
		{"integer into an integer array element", "function f() { var integer v_a[2]; v_a[1] := 2; }", ""},
		{"row of a 2-dimensional array", "function f() { var integer v_a[2][2]; var integer v_r[2]; v_r := v_a[0]; }", ""},
		{"charstring into a record of element", `function f() { var RC v_l; v_l[0] := "x"; }`, ""},
		{"variant of several encodings", `type charstring Str with { encode "C1"; encode "C2"; variant {"C1", "C2"}."rule" }`, ""},
		{"in-line template of a field", "function f() { var template R v_t := R:{ f := ? }; var template integer v_f := v_t.f; }", ""},
	}
	ruleCodes := []string{"signature-template-parameter-reference", "type-incompatible-assignment", "inline-template-reference"}
	for _, c := range cases {
		tree := parse(t, `module M {
			signature S(in integer par1, in integer par2);
			type record R { integer f }
			type record of charstring RC;
			template S s_sig := { par1 := 1, par2 := 2 };
			template R t_rec := { f := 1 };
			`+c.defs+`
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
